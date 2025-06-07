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
	// Load settings, init, etc.
	err := settings.Load()
	if err != nil {
		log.Println("[Settings]: Loading failed:", err)
		log.Println("[Settings]: Using default settings")
	}

	// Initialize GLFW since we need it for the game window
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
	
	audio.Init()
	m := menu.Init(vid)
	core.Init(vid)
	input.Init(vid)

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

	// Timer management goroutine
	go func() {
		remaining := durationSeconds
		for {
			if remaining <= 0 {
				// Pause the game and notify the Fyne UI
				state.MenuActive = true
				globalTimerOverlay.mu.Lock()
				globalTimerOverlay.remaining = 0
				globalTimerOverlay.mu.Unlock()
				timerChan <- -1 // Signal timeout to Fyne UI

				// Wait for resume signal and new duration
				<-resumeChan     // Wait for resume signal
				newDuration := <-timerChan // Get new timer duration
				remaining = newDuration
				state.MenuActive = false // Resume the game

				// Update overlay
				globalTimerOverlay.mu.Lock()
				globalTimerOverlay.remaining = remaining
				globalTimerOverlay.mu.Unlock()

				// Bring the Ludo window to the foreground
				vid.Window.Show()
				vid.Window.Focus()
			} else {
				select {
				case newDuration := <-timerChan:
					if newDuration > 0 {
						remaining = newDuration
						globalTimerOverlay.mu.Lock()
						globalTimerOverlay.remaining = remaining
						globalTimerOverlay.mu.Unlock()
					}
				case <-time.After(time.Second):
					remaining--
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
	runLoop(vid, m)
	core.Unload()
	return nil
}
