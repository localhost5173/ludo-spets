package ludo

import (
	"fmt"
	"log"
	"runtime"
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
		} else {
			m.Update(dt)
			vid.Render()
			m.Render(dt)
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

// RunGame launches the given core+game and shows a "TIME LEFT: mm:ss" notification
// on top of the Ludo window. When countdown hits zero, window is paused automatically.
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

	// Timer management goroutine
	go func() {
		remaining := durationSeconds
		for {
			if remaining <= 0 {
				// Pause the game and notify the Fyne UI
				state.MenuActive = true
				timerChan <- -1 // Signal timeout to Fyne UI
				
				// Wait for resume signal and new duration
				<-resumeChan     // Wait for resume signal
				newDuration := <-timerChan // Get new timer duration
				remaining = newDuration
				state.MenuActive = false // Resume the game
				
				// Bring the Ludo window to the foreground
				vid.Window.Show()
				vid.Window.Focus()
			} else {
				select {
				case newDuration := <-timerChan:
					if newDuration > 0 {
						remaining = newDuration
					}
				case <-time.After(time.Second):
					mins := remaining / 60
					secs := remaining % 60
					ntf.Display(ntf.Info, fmt.Sprintf("TIME LEFT: %02d:%02d", mins, secs), 1.0)
					remaining--
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
