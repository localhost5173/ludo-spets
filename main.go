package main

import (
	"fmt"
	"image/color"
	"runtime"
	"sync"
	"time"

	"github.com/libretro/ludo/ludo"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// arcadeTheme — just your custom colors (unchanged).
type arcadeTheme struct{}

func (t arcadeTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameBackground:
		return color.NRGBA{R: 0x0a, G: 0x0a, B: 0x0a, A: 0xff}
	case theme.ColorNameForeground:
		return color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	case theme.ColorNamePrimary:
		return color.NRGBA{R: 0xff, G: 0x00, B: 0x7f, A: 0xff}
	case theme.ColorNameFocus:
		return color.NRGBA{R: 0x00, G: 0xff, B: 0xff, A: 0xff}
	case theme.ColorNameHover:
		return color.NRGBA{R: 0x40, G: 0x40, B: 0x40, A: 0xff}
	case theme.ColorNameSelection:
		return color.NRGBA{R: 0x00, G: 0xff, B: 0xff, A: 0x40}
	case theme.ColorNameButton:
		return color.NRGBA{R: 0x1a, G: 0x1a, B: 0x1a, A: 0xff}
	default:
		return theme.DefaultTheme().Color(name, variant)
	}
}

func (t arcadeTheme) Font(style fyne.TextStyle) fyne.Resource   { return theme.DefaultTheme().Font(style) }
func (t arcadeTheme) Icon(name fyne.ThemeIconName) fyne.Resource { return theme.DefaultTheme().Icon(name) }
func (t arcadeTheme) Size(name fyne.ThemeSizeName) float32       { return theme.DefaultTheme().Size(name) }

func main() {
	a := app.New()
	a.Settings().SetTheme(&arcadeTheme{})

	w := a.NewWindow("SPETS ARCADE")
	w.SetFullScreen(true)
	w.CenterOnScreen()

	// ==============================
	// 1) Prepare your game‐core map
	// ==============================
	games := map[string]string{
		"Super Mario Bros": "/home/simon/Dev/ludo-spets/games/smb.nes",
		"Nova":             "/home/simon/Dev/ludo-spets/games/nova.nes",
		"Contra":           "/home/simon/Dev/ludo-spets/games/contra.nes",
	}
	cores := map[string]string{
		"Super Mario Bros": "/home/simon/Dev/ludo-spets/cores/nestopia_libretro.so",
		"Nova":             "/home/simon/Dev/ludo-spets/cores/nestopia_libretro.so",
		"Contra":           "/home/simon/Dev/ludo-spets/cores/nestopia_libretro.so",
	}

	// Build an ordered slice of game names (keys) for navigation:
	keys := make([]string, 0, len(games))
	for k := range games {
		keys = append(keys, k)
	}

	selectedIdx := 0

	// ====================================
	// 2) Create all UI widgets up front
	// ====================================
	status := canvas.NewText("◄ ► SELECT GAME    ENTER TO CONTINUE", color.White)
	status.TextSize = 16
	status.Alignment = fyne.TextAlignCenter

	list := widget.NewList(
		func() int { return len(keys) },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			o.(*widget.Label).SetText(keys[i])
		},
	)
	list.Select(selectedIdx)

	// Slider for minutes
	timeSlider := widget.NewSlider(1, 60)
	timeSlider.Step = 1
	timeSlider.SetValue(5)

	priceLabel := canvas.NewText("TOTAL COST: $2.50", color.White)
	priceLabel.TextSize = 18
	priceLabel.Alignment = fyne.TextAlignCenter

	paymentPrompt := canvas.NewText("PRESS 'X' TO INSERT COIN", color.NRGBA{R: 0xff, G: 0xff, B: 0x00, A: 0xff})
	paymentPrompt.TextSize = 24
	paymentPrompt.Alignment = fyne.TextAlignCenter

	// Channels for communication between Fyne UI and game logic
	timerChan := make(chan int, 10)
	resumeChan := make(chan bool, 10)

	var stateMutex sync.Mutex

	// State machine: 0=select game, 1=time select, 2=payment, 3=extend time
	const (
		stateSelectGame = 0
		stateTimeSelect = 1
		statePayment    = 2
		stateExtendTime = 3
	)
	state := stateSelectGame

	const pricePerMinute = 0.5
	updatePrice := func() {
		mins := int(timeSlider.Value)
		price := float64(mins) * pricePerMinute
		priceLabel.Text = fmt.Sprintf("TOTAL COST: $%.2f", price)
		priceLabel.Refresh()
	}
	updatePrice()

	// Put everything in a single VBox
	content := container.NewVBox(
		status,
		list,
		container.NewVBox(timeSlider, priceLabel),
		paymentPrompt,
	)

	// Hide slider, price, payment prompt initially:
	timeSlider.Hide()
	priceLabel.Hide()
	paymentPrompt.Hide()

	w.SetContent(content)

	// Monitor timer events using a ticker
	ticker := time.NewTicker(100 * time.Millisecond)
	go func() {
		defer ticker.Stop()
		for range ticker.C {
			select {
			case signal := <-timerChan:
				if signal == -1 {
					// Timer expired, show window on main thread
					stateMutex.Lock()
					state = stateExtendTime
					stateMutex.Unlock()

					w.Show()
					w.RequestFocus()
					status.Text = "TIME OUT! SELECT TIME THEN PRESS X TO PAY"
					status.Refresh()
					timeSlider.Show()
					priceLabel.Show()
					paymentPrompt.Show()
				}
			default:
				// No signal, continue
			}
		}
	}()

	// ===========================
	// 3) Handle key events
	// ===========================
	w.Canvas().SetOnTypedKey(func(key *fyne.KeyEvent) {
		stateMutex.Lock()
		currentState := state
		stateMutex.Unlock()

		switch currentState {
		// ─── State 0: SELECT GAME ───
		case stateSelectGame:
			switch key.Name {
			case fyne.KeyDown:
				if selectedIdx < len(keys)-1 {
					selectedIdx++
					list.Select(selectedIdx)
				}
			case fyne.KeyUp:
				if selectedIdx > 0 {
					selectedIdx--
					list.Select(selectedIdx)
				}
			case fyne.KeyReturn, fyne.KeyEnter:
				stateMutex.Lock()
				state = stateTimeSelect
				stateMutex.Unlock()

				status.Text = "SELECT TIME THEN ENTER"
				status.Refresh()
				list.Hide()
				timeSlider.Show()
				priceLabel.Show()
			}

		// ─── State 1: TIME SELECT ───
		case stateTimeSelect:
			switch key.Name {
			case fyne.KeyRight:
				if timeSlider.Value < timeSlider.Max {
					timeSlider.SetValue(timeSlider.Value + 1)
					updatePrice()
				}
			case fyne.KeyLeft:
				if timeSlider.Value > timeSlider.Min {
					timeSlider.SetValue(timeSlider.Value - 1)
					updatePrice()
				}
			case fyne.KeyReturn, fyne.KeyEnter:
				stateMutex.Lock()
				state = statePayment
				stateMutex.Unlock()

				status.Text = "PRESS X TO PAY"
				status.Refresh()
				timeSlider.Hide()
				priceLabel.Hide()
				paymentPrompt.Show()
			}

		// ─── State 2: PAYMENT ───
		case statePayment:
			if key.Name == fyne.KeyX {
				fmt.Println("X key pressed - starting game launch")
				status.Text = "LAUNCHING GAME..."
				status.Refresh()

				// Pull out the chosen game & core
				gameName := keys[selectedIdx]
				corePath := cores[gameName]
				gamePath := games[gameName]
				mins := int(timeSlider.Value)
				durationSecs := mins * 2

				fmt.Printf("Launching: %s with core: %s for %d seconds\n", gamePath, corePath, durationSecs)

				// Hide Fyne window and launch Ludo
				w.Hide()
				paymentPrompt.Hide()

				// Launch Ludo in its own goroutine, locked to OS thread
				go func() {
					runtime.LockOSThread()
					defer runtime.UnlockOSThread()
					fmt.Println("About to call ludo.RunGame")
					err := ludo.RunGame(corePath, gamePath, durationSecs, timerChan, resumeChan)
					if err != nil {
						fmt.Printf("Error running game: %v\n", err)
					}
				}()
			}

		// ─── State 3: EXTEND TIME ───
		case stateExtendTime:
			switch key.Name {
			case fyne.KeyRight:
				if timeSlider.Value < timeSlider.Max {
					timeSlider.SetValue(timeSlider.Value + 1)
					updatePrice()
				}
			case fyne.KeyLeft:
				if timeSlider.Value > timeSlider.Min {
					timeSlider.SetValue(timeSlider.Value - 1)
					updatePrice()
				}
			case fyne.KeyX:
				status.Text = "RESUMING GAME..."
				status.Refresh()

				mins := int(timeSlider.Value)

				// Hide Fyne window and UI elements
				timeSlider.Hide()
				priceLabel.Hide()
				paymentPrompt.Hide()
				w.Hide()

				// Send resume signal and timer duration in separate goroutine
				go func() {
					resumeChan <- true
					timerChan <- mins * 2
				}()
			}
		}
	})
	w.ShowAndRun()
}