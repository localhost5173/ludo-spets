package main

import (
	"fmt"
	"path/filepath"
	"runtime"
	"time"

	"github.com/libretro/ludo/webui"
)

func main() {
	// Determine the appropriate cores directory based on architecture
	var coresDir string
	switch runtime.GOARCH {
	case "amd64", "386":
		coresDir = "cores/x86"
		fmt.Println("Detected x86 architecture, using cores from:", coresDir)
	case "arm64", "arm":
		coresDir = "cores/arm64"
		fmt.Println("Detected ARM architecture, using cores from:", coresDir)
	default:
		// Default to a generic cores directory if architecture is unknown
		coresDir = "cores"
		fmt.Printf("Using default cores directory for architecture: %s\n", runtime.GOARCH)
	}

	// ==============================
	// 1) Prepare your game‐core map
	// ==============================
	games := map[string]string{
		"Nova":            "games/nova.nes",
		"Super Adventure": "games/nova.nes",
		"Pixel Quest":     "games/nova.nes",
		"Retro Hero":      "games/nova.nes",
		"Classic Journey": "games/nova.nes",
		// Add more games here
	}

	// Build core paths using the determined cores directory
	cores := map[string]string{
		"Nova":            filepath.Join(coresDir, "nestopia_libretro.so"),
		"Super Adventure": filepath.Join(coresDir, "nestopia_libretro.so"),
		"Pixel Quest":     filepath.Join(coresDir, "nestopia_libretro.so"),
		"Retro Hero":      filepath.Join(coresDir, "nestopia_libretro.so"),
		"Classic Journey": filepath.Join(coresDir, "nestopia_libretro.so"),
		// Add corresponding cores here
	}

	// Add paths to game pictures
	gamePictures := map[string]string{
		"Nova":            "assets/spets/games/nova.png",
		"Super Adventure": "assets/spets/games/mario.png",
		"Pixel Quest":     "assets/spets/games/nova.png",
		"Retro Hero":      "assets/spets/games/nova.png",
		"Classic Journey": "assets/spets/games/nova.png",
		// Add corresponding picture paths here
	}

	// Channels for communication between Web UI and game logic
	timerChan := make(chan int, 10)
	resumeChan := make(chan bool, 10)

	// Create the web server
	server := webui.NewServer(games, cores, gamePictures, timerChan, resumeChan)

	// Monitor timer events and game loading using a ticker
	ticker := time.NewTicker(100 * time.Millisecond)
	go func() {
		defer ticker.Stop()
		for range ticker.C {
			select {
			case signal := <-timerChan:
				if signal == -1 {
					// Timer expired, tell the server to handle it
					fmt.Println("Main: Received timer expired signal (-1)")
					server.HandleTimeout()
				} else if signal == -999 {
					// Special signal indicating game is loaded
					fmt.Println("Main: Received game loaded signal")
					server.OnGameLoaded()
				} else if signal == -2 {
					// Signal to prepare for timeout (10 seconds remaining)
					fmt.Println("Main: Received prepare timeout signal (-2)")
					server.PrepareTimeout()
				}
			default:
				// No signal, continue
			}
		}
	}()

	// Start the web server on port 8080 - this will also launch the browser
	if err := server.Start(":8080"); err != nil {
		fmt.Printf("Failed to start web server: %v\n", err)
	}
}
