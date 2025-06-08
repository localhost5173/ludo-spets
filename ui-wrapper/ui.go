package ui

import (
	"fmt"
	"image/color"
	"io/ioutil"
	"runtime"
	"sync"

	"fyne.io/fyne/driver/desktop"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
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

// Define our custom colors for dark theme
var (
	colorPrimary    = color.NRGBA{R: 0xcf, G: 0x2e, B: 0x2e, A: 0xff} // #cf2e2e - Red accent
	colorAccent     = color.NRGBA{R: 0xfe, G: 0xcd, B: 0xa5, A: 0xff} // #fecda5 - Cream accent
	colorBackground = color.NRGBA{R: 0x0f, G: 0x0f, B: 0x0f, A: 0xff} // Very dark background
	colorSurface    = color.NRGBA{R: 0x1a, G: 0x1a, B: 0x1a, A: 0xff} // Dark surface for cards
	colorOnSurface  = color.NRGBA{R: 0xe0, G: 0xe0, B: 0xe0, A: 0xff} // Light text on dark surface
	colorBorder     = color.NRGBA{R: 0x2d, G: 0x2d, B: 0x2d, A: 0xff} // Subtle borders
	colorShadow     = color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0x40} // Deeper shadows for dark theme
)

// arcadeTheme defines the custom dark theme colors
type arcadeTheme struct{}

func (t arcadeTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameBackground:
		return colorBackground
	case theme.ColorNameForeground:
		return colorOnSurface
	case theme.ColorNamePrimary:
		return colorPrimary
	case theme.ColorNameFocus:
		return colorPrimary
	case theme.ColorNameHover:
		return color.NRGBA{R: 0x25, G: 0x25, B: 0x25, A: 0xff}
	case theme.ColorNameSelection:
		return color.NRGBA{R: 0xcf, G: 0x2e, B: 0x2e, A: 0x60}
	case theme.ColorNameButton:
		return colorSurface
	default:
		return theme.DefaultTheme().Color(name, variant)
	}
}

// Custom Karelia font resource
type kareliFont struct{}

func (f kareliFont) Name() string {
	return "Karelia"
}

func (f kareliFont) Content() []byte {
	// Try to load Karelia font from file
	fontBytes, err := ioutil.ReadFile("/usr/share/fonts/truetype/karelia.ttf")
	if err != nil {
		// Fallback to default font if unable to load Karelia
		fmt.Println("Unable to load Karelia font:", err)
		return nil
	}
	return fontBytes
}

func (t arcadeTheme) Font(style fyne.TextStyle) fyne.Resource {
	kFont := &kareliFont{}
	if kFont.Content() == nil {
		return theme.DefaultTheme().Font(style)
	}
	return kFont
}

func (t arcadeTheme) Icon(name fyne.ThemeIconName) fyne.Resource { return theme.DefaultTheme().Icon(name) }
func (t arcadeTheme) Size(name fyne.ThemeSizeName) float32       { return theme.DefaultTheme().Size(name) }

// GameTile represents a selectable game in the grid
type GameTile struct {
	widget.BaseWidget
	Name      string
	ImagePath string
	Selected  bool
	OnSelect  func()
	Hovered   bool
}

// NewGameTile creates a new game tile with image and name
func NewGameTile(name, imagePath string, onSelect func()) *GameTile {
	tile := &GameTile{
		Name:      name,
		ImagePath: imagePath,
		OnSelect:  onSelect,
	}
	tile.ExtendBaseWidget(tile)
	return tile
}

// CreateRenderer implements widget.Widget
func (g *GameTile) CreateRenderer() fyne.WidgetRenderer {
	// Create card background (dark surface)
	cardBackground := canvas.NewRectangle(colorSurface)
	cardBackground.CornerRadius = 6

	// Create shadow effect (deeper for dark theme)
	shadow := canvas.NewRectangle(colorShadow)
	shadow.CornerRadius = 6

	// Create selected border (red outline when selected)
	selectedBorder := canvas.NewRectangle(color.Transparent)
	selectedBorder.CornerRadius = 6
	selectedBorder.StrokeWidth = 2
	selectedBorder.StrokeColor = colorPrimary
	selectedBorder.Hide()

	// Create hover effect
	hoverOverlay := canvas.NewRectangle(color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0x08})
	hoverOverlay.CornerRadius = 6
	hoverOverlay.Hide()

	// Create the game image (much larger proportion)
	image := canvas.NewImageFromFile(g.ImagePath)
	image.FillMode = canvas.ImageFillContain

	// Create game name label (smaller to give more space to image)
	nameLabel := canvas.NewText(g.Name, colorOnSurface)
	nameLabel.TextSize = 12
	nameLabel.Alignment = fyne.TextAlignCenter
	nameLabel.TextStyle.Bold = true

	// Create image container that takes up most of the space
	imageContainer := container.NewPadded(image)

	// Create name container with minimal spacing
	nameContainer := container.NewCenter(nameLabel)

	// Create layout with larger image area and smaller text area
	content := container.NewBorder(
		nil, nameContainer, nil, nil,
		imageContainer,
	)

	// Minimal padding for compact design
	paddedContent := container.NewPadded(content)

	return &gameTileRenderer{
		tile:           g,
		shadow:         shadow,
		cardBackground: cardBackground,
		selectedBorder: selectedBorder,
		hoverOverlay:   hoverOverlay,
		content:        paddedContent,
		objects:        []fyne.CanvasObject{shadow, cardBackground, hoverOverlay, selectedBorder, paddedContent},
	}
}

type gameTileRenderer struct {
	tile           *GameTile
	shadow         *canvas.Rectangle
	cardBackground *canvas.Rectangle
	selectedBorder *canvas.Rectangle
	hoverOverlay   *canvas.Rectangle
	content        *fyne.Container
	objects        []fyne.CanvasObject
}

func (r *gameTileRenderer) Destroy() {}

func (r *gameTileRenderer) Layout(size fyne.Size) {
	// Position shadow slightly offset for depth effect
	shadowOffset := float32(3)
	r.shadow.Move(fyne.NewPos(shadowOffset, shadowOffset))
	r.shadow.Resize(size)

	// Card background fills the entire size
	r.cardBackground.Resize(size)
	r.selectedBorder.Resize(size)
	r.hoverOverlay.Resize(size)
	r.content.Resize(size)
}

func (r *gameTileRenderer) MinSize() fyne.Size {
	// More square tiles - 100x100 for a nearly square appearance
	return fyne.NewSize(100, 100)
}

func (r *gameTileRenderer) Objects() []fyne.CanvasObject {
	return r.objects
}

func (r *gameTileRenderer) Refresh() {
	if r.tile.Selected {
		r.selectedBorder.Show()
		r.selectedBorder.StrokeColor = colorPrimary
		r.selectedBorder.FillColor = color.Transparent
	} else {
		r.selectedBorder.Hide()
	}

	if r.tile.Hovered && !r.tile.Selected {
		r.hoverOverlay.Show()
	} else {
		r.hoverOverlay.Hide()
	}

	r.shadow.Refresh()
	r.cardBackground.Refresh()
	r.selectedBorder.Refresh()
	r.hoverOverlay.Refresh()
	r.content.Refresh()
}

// Tapped is called when the tile is clicked
func (g *GameTile) Tapped(_ *fyne.PointEvent) {
	if g.OnSelect != nil {
		g.OnSelect()
	}
}

// MouseIn is called when mouse enters the tile
func (g *GameTile) MouseIn(_ *desktop.MouseEvent) {
	g.Hovered = true
	g.Refresh()
}

// MouseOut is called when mouse leaves the tile
func (g *GameTile) MouseOut() {
	g.Hovered = false
	g.Refresh()
}

// UI wraps all Fyne UI elements and state.
type UI struct {
	app           fyne.App
	window        fyne.Window
	status        *canvas.Text
	gameGrid      *fyne.Container
	gameScroll    *container.Scroll
	gameTiles     []*GameTile
	timeSlider    *widget.Slider
	timeLabel     *widget.Label
	priceLabel    *canvas.Text
	paymentPrompt *canvas.Text
	content       *fyne.Container

	keys        []string
	selectedIdx int
	currentState UIState
	stateMutex  sync.Mutex

	games        map[string]string
	cores        map[string]string
	gamePictures map[string]string
	timerChan    chan int
	resumeChan   chan bool
}

// NewUI creates and initializes a new UI instance.
func NewUI(games, cores, gamePictures map[string]string, timerChan chan int, resumeChan chan bool) *UI {
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
		gamePictures: gamePictures,
		timerChan:    timerChan,
		resumeChan:   resumeChan,
	}

	// Status text without time selection hint
	ui.status = canvas.NewText("◄ ► ▲ ▼ NAVIGATE    ENTER TO CONTINUE", colorOnSurface)
	ui.status.TextSize = 18
	ui.status.Alignment = fyne.TextAlignCenter

	// Create game tiles and grid
	ui.createGameGrid()

	ui.timeSlider = widget.NewSlider(1, 60)
	ui.timeSlider.Step = 1
	ui.timeSlider.SetValue(5)

	// Create time selection label
	ui.timeLabel = widget.NewLabel("Select Playing Time (minutes):")
	ui.timeLabel.TextStyle.Bold = true

	ui.priceLabel = canvas.NewText("", colorOnSurface)
	ui.priceLabel.TextSize = 20
	ui.priceLabel.Alignment = fyne.TextAlignCenter
	ui.updatePrice()

	ui.paymentPrompt = canvas.NewText("PRESS 'P' TO INSERT COIN", colorPrimary)
	ui.paymentPrompt.TextSize = 26
	ui.paymentPrompt.Alignment = fyne.TextAlignCenter

	// Create stylish header
	ui.content = ui.createStylishHeader()

	// Hide elements that aren't needed initially
	ui.timeSlider.Hide()
	ui.timeLabel.Hide()
	ui.priceLabel.Hide()
	ui.paymentPrompt.Hide()

	ui.window.SetContent(ui.content)
	ui.setKeyHandler()

	return ui
}

// createStylishHeader creates a modern, stylish header for the UI
func (ui *UI) createStylishHeader() *fyne.Container {
	// Fixed title container with proper spacing
	titleMain := canvas.NewText("SPETS", colorPrimary)
	titleMain.TextSize = 48
	titleMain.Alignment = fyne.TextAlignCenter
	titleMain.TextStyle.Bold = true

	titleSub := canvas.NewText(" ARCADE", colorAccent)
	titleSub.TextSize = 48
	titleSub.Alignment = fyne.TextAlignCenter
	titleSub.TextStyle.Bold = true

	// Proper horizontal layout for title
	titleContainer := container.NewHBox(
		layout.NewSpacer(),
		titleMain,
		titleSub,
		layout.NewSpacer(),
	)

	// Decorative line
	decorLine := canvas.NewRectangle(colorPrimary)
	decorLine.Resize(fyne.NewSize(200, 2))

	// Status container
	statusContainer := container.NewCenter(ui.status)

	// Header background
	headerBg := canvas.NewRectangle(color.NRGBA{R: 0x12, G: 0x12, B: 0x12, A: 0xff})
	headerBg.CornerRadius = 0

	// Simplified header content without subtitle
	headerContent := container.NewVBox(
		layout.NewSpacer(),
		titleContainer,
		container.NewCenter(decorLine),
		layout.NewSpacer(),
		statusContainer,
		layout.NewSpacer(),
	)

	header := container.NewStack(
		headerBg,
		container.NewPadded(headerContent),
	)

	// Time selection container with dark styling
	timeBackground := canvas.NewRectangle(colorSurface)
	timeBackground.CornerRadius = 8

	timeContent := container.NewVBox(
		ui.timeLabel,
		ui.timeSlider,
		ui.priceLabel,
	)

	timeContainer := container.NewStack(
		timeBackground,
		container.NewPadded(timeContent),
	)

	// Main content area - wrap gameScroll in a Center container to prevent stretching
	centeredGameScroll := container.NewCenter(ui.gameScroll)

	mainContent := container.NewStack(
		centeredGameScroll,
		container.NewCenter(timeContainer),
		container.NewCenter(ui.paymentPrompt),
	)

	// Complete layout
	return container.NewBorder(
		header, nil, nil, nil,
		container.NewPadded(mainContent),
	)
}

// createGameGrid creates a fixed 8x4 grid of game tiles
func (ui *UI) createGameGrid() {
	ui.gameTiles = make([]*GameTile, len(ui.keys))
	gridItems := make([]fyne.CanvasObject, len(ui.keys))

	for i, gameName := range ui.keys {
		// Get the image path from the gamePictures map
		imagePath := ui.gamePictures[gameName]
		if imagePath == "" {
			// Fallback to a default image
			imagePath = "/home/simon/Dev/ludo-spets/assets/spets/games/default.png"
		}

		// Create the game tile with selection callback
		index := i // Capture loop variable
		ui.gameTiles[i] = NewGameTile(gameName, imagePath, func() {
			ui.selectGameTile(index)
		})

		// The first game is selected by default
		if i == 0 {
			ui.gameTiles[i].Selected = true
		}

		gridItems[i] = ui.gameTiles[i]
	}

	// Always use 8 columns
	ui.gameGrid = container.New(layout.NewGridLayoutWithColumns(8), gridItems...)

	// Create a fixed-size container for the grid that will show exactly 8x4 tiles
	paddedGrid := container.NewPadded(ui.gameGrid)

	// Use a VBox with spacer to ensure grid doesn't stretch if fewer than 4 rows
	gridContainer := container.NewVBox(paddedGrid, layout.NewSpacer())

	// Create scroll container with fixed size for 8x4 grid
	ui.gameScroll = container.NewScroll(gridContainer)

	// Calculate size for 8x4 grid of 100x100 tiles with padding
	// 8 tiles * 100px + 7 spaces * theme.Padding() + 2 * theme.Padding() (for container padding)
	gridWidth := 8*100 + 7*theme.Padding() + 2*theme.Padding()
	// 4 tiles * 100px + 3 spaces * theme.Padding() + 2 * theme.Padding() (for container padding)
	gridHeight := 4*100 + 3*theme.Padding() + 2*theme.Padding()

	ui.gameScroll.SetMinSize(fyne.NewSize(gridWidth, gridHeight))
}

func (ui *UI) selectGameTile(index int) {
	if index < 0 || index >= len(ui.gameTiles) {
		return
	}

	// Deselect the current tile
	if ui.selectedIdx >= 0 && ui.selectedIdx < len(ui.gameTiles) {
		ui.gameTiles[ui.selectedIdx].Selected = false
		ui.gameTiles[ui.selectedIdx].Refresh()
	}

	// Select the new tile
	ui.selectedIdx = index
	ui.gameTiles[ui.selectedIdx].Selected = true
	ui.gameTiles[ui.selectedIdx].Refresh()

	// Scroll to ensure the selected tile is visible
	ui.scrollToSelectedTile()
}

func (ui *UI) scrollToSelectedTile() {
	if ui.selectedIdx < 0 || ui.selectedIdx >= len(ui.gameTiles) {
		return
	}

	// Always use 8 columns
	cols := 8
	row := ui.selectedIdx / cols

	// Calculate tile height including spacing (100px tile + theme.Padding())
	tileHeight := float32(100 + theme.Padding())

	// Calculate scroll offset to show the selected row
	// Include the top padding of the container
	scrollOffset := float32(row) * tileHeight

	// Only scroll if needed
	if scrollOffset > 0 {
		ui.gameScroll.Offset = fyne.NewPos(0, scrollOffset)
		ui.gameScroll.Refresh()
	} else {
		ui.gameScroll.ScrollToTop()
	}
}

func (ui *UI) updatePrice() {
	mins := int(ui.timeSlider.Value)
	price := float64(mins) * pricePerMinute
	ui.priceLabel.Text = fmt.Sprintf("TOTAL COST: $%.2f (%d minutes)", price, mins)
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
				// Always use 8 columns
				cols := 8

				// Move down in the grid
				if ui.selectedIdx+cols < len(ui.keys) {
					ui.selectGameTile(ui.selectedIdx + cols)
				}
			case fyne.KeyUp:
				// Always use 8 columns
				cols := 8

				// Move up in the grid
				if ui.selectedIdx-cols >= 0 {
					ui.selectGameTile(ui.selectedIdx - cols)
				}
			case fyne.KeyLeft:
				if ui.selectedIdx > 0 {
					ui.selectGameTile(ui.selectedIdx - 1)
				}
			case fyne.KeyRight:
				if ui.selectedIdx < len(ui.keys)-1 {
					ui.selectGameTile(ui.selectedIdx + 1)
				}
			case fyne.KeyReturn, fyne.KeyEnter:
				ui.stateMutex.Lock()
				ui.currentState = stateTimeSelect
				ui.stateMutex.Unlock()
				ui.status.Text = "◄ ► ADJUST TIME    ENTER TO CONTINUE"
				ui.gameScroll.Hide()
				ui.timeLabel.Show()
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
				ui.status.Text = "PRESS 'P' TO INSERT COIN AND START GAME"
				ui.timeLabel.Hide()
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
				durationSecs := mins * 2

				fmt.Printf("Launching: %s with core: %s for %d seconds\n", gamePath, corePath, durationSecs)

				ui.window.Hide()
				ui.paymentPrompt.Hide()

				go func() {
					runtime.LockOSThread()
					defer runtime.UnlockOSThread()
					fmt.Println("About to call ludo.RunGame")
					err := ludo.RunGame(corePath, gamePath, durationSecs, ui.timerChan, ui.resumeChan)
					if err != nil {
						fmt.Printf("Error running game: %v\n", err)
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

				ui.timeLabel.Hide()
				ui.timeSlider.Hide()
				ui.priceLabel.Hide()
				ui.paymentPrompt.Hide()
				ui.window.Hide()

				go func() {
					ui.resumeChan <- true
					ui.timerChan <- mins * 2
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

	ui.window.Show()
	ui.window.RequestFocus()
	ui.status.Text = "TIME OUT! ◄ ► ADJUST TIME    PRESS 'P' TO PAY AND CONTINUE"
	ui.status.Refresh()
	ui.gameScroll.Hide()
	ui.timeLabel.Show()
	ui.timeSlider.Show()
	ui.priceLabel.Show()
	ui.paymentPrompt.Show()
}