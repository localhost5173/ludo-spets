package ui

import (
	"fmt"
	"image/color"
	"runtime"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/libretro/ludo/ludo"
)

// UIState defines the different states of the UI.
type UIState int

const (
	stateSelectGame UIState = iota
	stateTimeSelect
	statePayment
	stateExtendTime
)

const pricePerMinute = 0.5

// arcadeTheme defines the custom theme colors.
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

// UI wraps all Fyne UI elements and state.
type UI struct {
	app           fyne.App
	window        fyne.Window
	status        *canvas.Text
	list          *widget.List
	timeSlider    *widget.Slider
	priceLabel    *canvas.Text
	paymentPrompt *canvas.Text
	content       *fyne.Container

	keys        []string
	selectedIdx int
	currentState UIState
	stateMutex  sync.Mutex

	games      map[string]string
	cores      map[string]string
	timerChan  chan int
	resumeChan chan bool
}

// NewUI creates and initializes a new UI instance.
func NewUI(games, cores map[string]string, timerChan chan int, resumeChan chan bool) *UI {
	a := app.New()
	a.Settings().SetTheme(&arcadeTheme{})

	w := a.NewWindow("SPETS ARCADE")
	w.SetFullScreen(true)
	w.CenterOnScreen()

	keys := make([]string, 0, len(games))
	for k := range games {
		keys = append(keys, k)
	}

	ui := &UI{
		app:          a,
		window:       w,
		keys:         keys,
		selectedIdx:  0,
		currentState: stateSelectGame,
		games:        games,
		cores:        cores,
		timerChan:    timerChan,
		resumeChan:   resumeChan,
	}

	ui.status = canvas.NewText("◄ ► SELECT GAME    ENTER TO CONTINUE", color.White)
	ui.status.TextSize = 16
	ui.status.Alignment = fyne.TextAlignCenter

	ui.list = widget.NewList(
		func() int { return len(ui.keys) },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			o.(*widget.Label).SetText(ui.keys[i])
		},
	)
	ui.list.Select(ui.selectedIdx)

	ui.timeSlider = widget.NewSlider(1, 60)
	ui.timeSlider.Step = 1
	ui.timeSlider.SetValue(5)

	ui.priceLabel = canvas.NewText("", color.White) // Initial text set by updatePrice
	ui.priceLabel.TextSize = 18
	ui.priceLabel.Alignment = fyne.TextAlignCenter
	ui.updatePrice() // Set initial price

	ui.paymentPrompt = canvas.NewText("PRESS 'P' TO INSERT COIN", color.NRGBA{R: 0xff, G: 0xff, B: 0x00, A: 0xff})
	ui.paymentPrompt.TextSize = 24
	ui.paymentPrompt.Alignment = fyne.TextAlignCenter

	ui.content = container.NewVBox(
		ui.status,
		ui.list,
		container.NewVBox(ui.timeSlider, ui.priceLabel),
		ui.paymentPrompt,
	)

	ui.timeSlider.Hide()
	ui.priceLabel.Hide()
	ui.paymentPrompt.Hide()

	ui.window.SetContent(ui.content)
	ui.setKeyHandler()

	return ui
}

func (ui *UI) updatePrice() {
	mins := int(ui.timeSlider.Value)
	price := float64(mins) * pricePerMinute
	ui.priceLabel.Text = fmt.Sprintf("TOTAL COST: $%.2f", price)
	ui.priceLabel.Refresh()
}

func (ui *UI) setKeyHandler() {
	ui.window.Canvas().SetOnTypedKey(func(key *fyne.KeyEvent) {
		ui.stateMutex.Lock()
		currentState := ui.currentState
		ui.stateMutex.Unlock()

		needsRefresh := false

		switch currentState {
		case stateSelectGame:
			switch key.Name {
			case fyne.KeyDown:
				if ui.selectedIdx < len(ui.keys)-1 {
					ui.selectedIdx++
					ui.list.Select(ui.selectedIdx)
				}
			case fyne.KeyUp:
				if ui.selectedIdx > 0 {
					ui.selectedIdx--
					ui.list.Select(ui.selectedIdx)
				}
			case fyne.KeyReturn, fyne.KeyEnter:
				ui.stateMutex.Lock()
				ui.currentState = stateTimeSelect
				ui.stateMutex.Unlock()
				ui.status.Text = "SELECT TIME THEN ENTER"
				ui.list.Hide()
				ui.timeSlider.Show()
				ui.priceLabel.Show()
				needsRefresh = true
			}

		case stateTimeSelect:
			switch key.Name {
			case fyne.KeyRight:
				if ui.timeSlider.Value < ui.timeSlider.Max {
					ui.timeSlider.SetValue(ui.timeSlider.Value + 1)
					ui.updatePrice()
				}
			case fyne.KeyLeft:
				if ui.timeSlider.Value > ui.timeSlider.Min {
					ui.timeSlider.SetValue(ui.timeSlider.Value - 1)
					ui.updatePrice()
				}
			case fyne.KeyReturn, fyne.KeyEnter:
				ui.stateMutex.Lock()
				ui.currentState = statePayment
				ui.stateMutex.Unlock()
				ui.status.Text = "PRESS P TO PAY"
				ui.timeSlider.Hide()
				ui.priceLabel.Hide()
				ui.paymentPrompt.Show()
				needsRefresh = true
			}

		case statePayment:
			if key.Name == fyne.KeyP {
				ui.status.Text = "LAUNCHING GAME..."
				needsRefresh = true

				gameName := ui.keys[ui.selectedIdx]
				corePath := ui.cores[gameName]
				gamePath := ui.games[gameName]
				mins := int(ui.timeSlider.Value)
				durationSecs := mins * 2 // Keep original logic: mins * 2

				fmt.Printf("Launching: %s with core: %s for %d seconds\n", gamePath, corePath, durationSecs)

				ui.window.Hide()
				ui.paymentPrompt.Hide() // Ensure it's hidden before game starts

				go func() {
					runtime.LockOSThread()
					defer runtime.UnlockOSThread()
					fmt.Println("About to call ludo.RunGame")
					err := ludo.RunGame(corePath, gamePath, durationSecs, ui.timerChan, ui.resumeChan)
					if err != nil {
						fmt.Printf("Error running game: %v\n", err)
						// Optionally, signal UI to show an error
					}
				}()
			}

		case stateExtendTime:
			switch key.Name {
			case fyne.KeyRight:
				if ui.timeSlider.Value < ui.timeSlider.Max {
					ui.timeSlider.SetValue(ui.timeSlider.Value + 1)
					ui.updatePrice()
				}
			case fyne.KeyLeft:
				if ui.timeSlider.Value > ui.timeSlider.Min {
					ui.timeSlider.SetValue(ui.timeSlider.Value - 1)
					ui.updatePrice()
				}
			case fyne.KeyP:
				ui.status.Text = "RESUMING GAME..."
				needsRefresh = true
				mins := int(ui.timeSlider.Value)

				ui.timeSlider.Hide()
				ui.priceLabel.Hide()
				ui.paymentPrompt.Hide()
				ui.window.Hide()

				go func() {
					ui.resumeChan <- true
					ui.timerChan <- mins * 2 // Keep original logic: mins * 2
				}()
			}
		}
		if needsRefresh {
			ui.status.Refresh()
		}
	})
}

// Run starts the Fyne application and shows the main window.
func (ui *UI) Run() {
	ui.window.ShowAndRun()
}

// HandleTimeout is called when the game timer expires.
func (ui *UI) HandleTimeout() {
	ui.stateMutex.Lock()
	ui.currentState = stateExtendTime
	ui.stateMutex.Unlock()

	// Operations on Fyne objects should be done on the main thread if possible,
	// or ensure they are thread-safe. Show/Hide/RequestFocus are generally safe.
	// Refreshing canvas objects might need `QueueEvent` or similar if called from non-main goroutine,
	// but here it's fine as Fyne handles its event queue.
	ui.window.Show()
	ui.window.RequestFocus()
	ui.status.Text = "TIME OUT! SELECT TIME THEN PRESS P TO PAY"
	ui.status.Refresh()
	ui.timeSlider.Show()
	ui.priceLabel.Show()
	ui.paymentPrompt.Show()
}