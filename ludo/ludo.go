package ludo

import (
	"fmt"
	"log"
	"runtime"
	"sync"
	"time"

	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/libretro/ludo/audio"
	"github.com/libretro/ludo/core"
	"github.com/libretro/ludo/history"
	"github.com/libretro/ludo/input"
	"github.com/libretro/ludo/menu"
	ntf "github.com/libretro/ludo/notifications"
	"github.com/libretro/ludo/playlists"
	"github.com/libretro/ludo/savefiles"
	"github.com/libretro/ludo/scanner"
	"github.com/libretro/ludo/settings"
	"github.com/libretro/ludo/state"
	"github.com/libretro/ludo/video"
)

// init locks the OS thread for GLFW.
func init() {
	runtime.LockOSThread()
}

var frame = 0

// timerOverlay holds the remaining time and a mutex for thread safety.
type timerOverlay struct {
	remaining int
	mu        sync.RWMutex
	visible   bool
}

var globalTimerOverlay = &timerOverlay{}

// drawTimerOverlay draws the timer in the top-right corner, small, always visible, transparent background.
func drawTimerOverlay(vid *video.Video) {
	globalTimerOverlay.mu.RLock()
	remaining := globalTimerOverlay.remaining
	visible := globalTimerOverlay.visible
	globalTimerOverlay.mu.RUnlock()

	if !visible || remaining < 0 {
		return
	}

	w, _ := vid.GetFramebufferSize()
	ratio := float32(w) / 1920
	padding := 32 * ratio
	bgW := 120 * ratio // much smaller width
	bgH := 36 * ratio  // much smaller height
	x := float32(w) - bgW - padding
	y := padding

	// Transparent background
	vid.DrawRect(x, y, bgW, bgH, 6*ratio, video.Color{R: 0, G: 0, B: 0, A: 0.45})

	// Timer text (red, much smaller)
	mins := remaining / 60
	secs := remaining % 60
	timerStr := fmt.Sprintf("%02d:%02d", mins, secs)
	vid.Font.SetColor(video.Color{R: 1, G: 0, B: 0, A: 1})
	vid.Font.Printf(x+18*ratio, y+7*ratio, 0.32*ratio, timerStr)
}

func runLoop(vid *video.Video, m *menu.Menu) {
	var currTime time.Time
	prevTime := time.Now()

	for !vid.Window.ShouldClose() {
		currTime = time.Now()
		dt := float32(currTime.Sub(prevTime)) / 1e9

		// Core polling/rendering
		glfw.PollEvents()
		m.ProcessHotkeys()
		ntf.Process(dt)
		vid.ResizeViewport()
		m.UpdatePalette()
		input.Poll()

		if !state.MenuActive {
			if state.CoreRunning {
				state.Core.Run()
				if state.Core.FrameTimeCallback != nil {
					state.Core.FrameTimeCallback.Callback(state.Core.FrameTimeCallback.Reference)
				}
				if state.Core.AudioCallback != nil {
					state.Core.AudioCallback.Callback()
				}
			}
			vid.Render()
			frame++
			if frame%600 == 0 {
				savefiles.SaveSRAM()
				}
			// Draw timer overlay on top of game
			drawTimerOverlay(vid)
		} else {
			m.Update(dt)
			vid.Render()
			m.Render(dt)
			// Draw timer overlay on top of menu
			drawTimerOverlay(vid)
		}

		// Always draw notifications on top
		m.RenderNotifications()

		if state.FastForward {
			glfw.SwapInterval(0)
		} else {
			glfw.SwapInterval(1)
		}
		vid.Window.SwapBuffers()
		prevTime = currTime
	}
}

// RunGame launches the given core+game and shows a "TIME LEFT: mm:ss" overlay
// in the top-right corner of the Ludo window. When countdown hits zero, window is paused automatically.
func RunGame(corePath, gamePath string, durationSeconds int, timerChan chan int, resumeChan chan bool) error {
	// Ensure we're running on a locked OS thread
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// Load settings, init, etc.
	err := settings.Load()
	if err != nil {
		log.Println("[Settings]: Loading failed:", err)
		log.Println("[Settings]: Using default settings")
	}

	// Initialize GLFW since we need it for the game window
	// GLFW must be initialized from the same thread that will later process its events
	if err := glfw.Init(); err != nil {
		return fmt.Errorf("failed to initialize glfw: %w", err)
	}
	defer glfw.Terminate()

	state.DB, err = scanner.LoadDB(settings.Current.DatabaseDirectory)
	if err != nil {
		log.Println("Can't load game database:", err)
	}

	playlists.Load()
	history.Load()

	// Force window to be visible and focused
	vid := video.Init(true) // Force fullscreen to ensure visibility
	
	// Ensure window is shown and focused
	vid.Window.Show()
	vid.Window.Focus()
	
	// Initialize audio after video to ensure proper context
	audio.Init()
	m := menu.Init(vid)
	core.Init(vid)
	input.Init(vid)

	// Load core and game with improved error handling
	if err := core.Load(corePath); err != nil {
		return fmt.Errorf("failed to load core: %w", err)
	}
	
	if err := core.LoadGame(gamePath); err != nil {
		ntf.DisplayAndLog(ntf.Error, "Menu", err.Error())
		return fmt.Errorf("failed to load game: %w", err)
	}
	
	// Start the game immediately - don't go to quick menu
	state.MenuActive = false
	state.CoreRunning = true

	// Initialize timer overlay
	globalTimerOverlay.mu.Lock()
	globalTimerOverlay.remaining = durationSeconds
	globalTimerOverlay.visible = true
	globalTimerOverlay.mu.Unlock()

	// Wait a moment for everything to initialize properly
	time.Sleep(200 * time.Millisecond)
	
	// Force window to front and ensure it's visible
	vid.Window.Show()
	vid.Window.Focus()
	
	// Signal that the game is loaded and ready
	log.Println("Game fully loaded, sending confirmation signal")
	timerChan <- -999 // Special signal for game loaded

	// Create a done channel to signal when timer goroutine should exit
	doneChan := make(chan struct{})
	
	// Timer management goroutine with cancellation support
	go func() {
		remaining := durationSeconds
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		prepareTimeoutSent := false
		
		for {
			select {
			case <-doneChan:
				// Exit the goroutine when signaled
				return
				
			case <-ticker.C:
				remaining--
				globalTimerOverlay.mu.Lock()
				globalTimerOverlay.remaining = remaining
				globalTimerOverlay.mu.Unlock()
				
				// Send prepare timeout signal 10 seconds before actual timeout
				if remaining == 10 && !prepareTimeoutSent {
					log.Println("Sending prepare timeout signal (10 seconds remaining)")
					timerChan <- -2 // -2 is the prepare timeout signal
					prepareTimeoutSent = true
				}
				
				if remaining <= 0 {
					 // When time runs out, send game window information along with timeout signal
					// This will allow the web UI to position itself over the game window
					
					// Get window position and size
					var xpos, ypos, width, height int
					xpos, ypos = vid.Window.GetPos()
					width, height = vid.Window.GetSize()
					
					log.Printf("Game timeout! Pausing game at window position: %d,%d %dx%d", xpos, ypos, width, height)
					
					// Signal timeout to UI with window information
					timerChan <- -1 // -1 is the timeout signal
					
					// Send window information through the same channel
					// Pack window info as: x, y, width, height
					timerChan <- xpos
					timerChan <- ypos
					timerChan <- width
					timerChan <- height
					
					// Pause the game but keep window visible
					state.MenuActive = true
					log.Println("Game paused, waiting for resume signal...")
					
					// Wait for resume signal
					resumeReceived := <-resumeChan
					log.Printf("Resume signal received: %v", resumeReceived)
					
					// Get new duration
					newDuration := <-timerChan
					log.Printf("New timer duration received: %d seconds", newDuration)
					
					remaining = newDuration
					prepareTimeoutSent = false // Reset for next cycle
					
					// Update overlay
					globalTimerOverlay.mu.Lock()
					globalTimerOverlay.remaining = remaining
					globalTimerOverlay.mu.Unlock()
					
					// Resume game and ensure focus
					log.Println("Resuming game...")
					state.MenuActive = false
					
					// Force window focus
					vid.Window.Show()
					vid.Window.Focus()
					
					log.Printf("Game resumed with %d seconds remaining", remaining)
				}
				
			case newDuration := <-timerChan:
				// Handle additional time being added (not during timeout)
				if newDuration > 0 {
					log.Printf("Timer updated: adding %d seconds", newDuration)
					remaining += newDuration  // Add to existing time instead of replacing
					globalTimerOverlay.mu.Lock()
					globalTimerOverlay.remaining = remaining
					globalTimerOverlay.mu.Unlock()
				}
			}
		}
	}()

	// Add a small delay to ensure everything is initialized
	time.Sleep(100 * time.Millisecond)
	
	// Force window to front again
	vid.Window.Show()
	vid.Window.Focus()
	
	log.Println("Starting game loop...")
	
	// Run the game loop with error handling
	defer func() {
		// Signal timer goroutine to exit
		close(doneChan)
		
		// Ensure core is unloaded properly
		core.Unload()
		
		// Handle any panic
		if r := recover(); r != nil {
			log.Printf("Recovered from panic in game loop: %v", r)
		}
	}()
	
	runLoop(vid, m)
	return nil
}
