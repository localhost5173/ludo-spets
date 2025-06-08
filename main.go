package main

import (
	"fmt"
	"time"

	"github.com/libretro/ludo/webui"
)

func main() {
	// ==============================
	// 1) Prepare your game‐core map
	// ==============================
	games := map[string]string{
		"Nova":            "/home/simon/Dev/ludo-spets/games/nova.nes",
		"Super Adventure": "/home/simon/Dev/ludo-spets/games/nova.nes",
		"Pixel Quest":     "/home/simon/Dev/ludo-spets/games/nova.nes",
		"Retro Hero":      "/home/simon/Dev/ludo-spets/games/nova.nes",
		"Classic Journey": "/home/simon/Dev/ludo-spets/games/nova.nes",
		// Add more games here
	}
	cores := map[string]string{
		"Nova":            "/home/simon/Dev/ludo-spets/cores/nestopia_libretro.so",
		"Super Adventure": "/home/simon/Dev/ludo-spets/cores/nestopia_libretro.so",
		"Pixel Quest":     "/home/simon/Dev/ludo-spets/cores/nestopia_libretro.so",
		"Retro Hero":      "/home/simon/Dev/ludo-spets/cores/nestopia_libretro.so",
		"Classic Journey": "/home/simon/Dev/ludo-spets/cores/nestopia_libretro.so",
		// Add corresponding cores here
	}
	// Add paths to game pictures
	gamePictures := map[string]string{
		"Nova":            "/home/simon/Dev/ludo-spets/assets/spets/games/nova.png",
		"Super Adventure": "/home/simon/Dev/ludo-spets/assets/spets/games/mario.png",
		"Pixel Quest":     "/home/simon/Dev/ludo-spets/assets/spets/games/nova.png",
		"Retro Hero":      "/home/simon/Dev/ludo-spets/assets/spets/games/nova.png",
		"Classic Journey": "/home/simon/Dev/ludo-spets/assets/spets/games/nova.png",
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
