package main

import (
	"fmt"
	"time"

	"github.com/libretro/ludo/ui-wrapper"
)

func main() {
	// ==============================
	// 1) Prepare your game‐core map
	// ==============================
	games := map[string]string{
		"Nova": "/home/simon/Dev/ludo-spets/games/nova.nes",
	}
	cores := map[string]string{
		"Nova": "/home/simon/Dev/ludo-spets/cores/nestopia_libretro.so",
	}

	// Channels for communication between Fyne UI and game logic
	timerChan := make(chan int, 10)
	resumeChan := make(chan bool, 10)

	// Create the UI instance
	appUI := ui.NewUI(games, cores, timerChan, resumeChan)

	// Monitor timer events using a ticker
	ticker := time.NewTicker(100 * time.Millisecond)
	go func() {
		defer ticker.Stop()
		for range ticker.C {
			select {
			case signal := <-timerChan:
				if signal == -1 {
					// Timer expired, tell the UI to handle it
					fmt.Println("Main: Received timer expired signal (-1)")
					appUI.HandleTimeout()
				}
			default:
				// No signal, continue
			}
		}
	}()

	// Run the UI
	appUI.Run()
}