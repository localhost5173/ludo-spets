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

// Define our custom colors
var (
	colorCream     = color.NRGBA{R: 0xfe, G: 0xcd, B: 0xa5, A: 0xff} // #fecda5
	colorRed       = color.NRGBA{R: 0xcf, G: 0x2e, B: 0x2e, A: 0xff} // #cf2e2e
	colorBlack     = color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0xff} // #000000
	colorDarkGray  = color.NRGBA{R: 0x1a, G: 0x1a, B: 0x1a, A: 0xff} // Dark background for tiles
	colorMedGray   = color.NRGBA{R: 0x2d, G: 0x2d, B: 0x2d, A: 0xff} // Medium gray for borders
	colorLightGray = color.NRGBA{R: 0x40, G: 0x40, B: 0x40, A: 0xff} // Light gray for hover
)

// arcadeTheme defines the custom theme colors.
type arcadeTheme struct{}

func (t arcadeTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameBackground:
		return colorBlack
	case theme.ColorNameForeground:
		return colorCream
	case theme.ColorNamePrimary:
		return colorRed
	case theme.ColorNameFocus:
		return colorRed
	case theme.ColorNameHover:
		return colorLightGray
	case theme.ColorNameSelection:
		return color.NRGBA{R: 0xcf, G: 0x2e, B: 0x2e, A: 0x80} // Semi-transparent red
	case theme.ColorNameButton:
		return colorDarkGray
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
	// Create tile background
	background := canvas.NewRectangle(colorDarkGray)
	background.CornerRadius = 8

	// Create selected state border
	selectedBorder := canvas.NewRectangle(colorRed)
	selectedBorder.CornerRadius = 8
	selectedBorder.StrokeWidth = 3
	selectedBorder.StrokeColor = colorRed
	selectedBorder.FillColor = color.Transparent
	selectedBorder.Hide()

	// Create hover overlay
	hoverOverlay := canvas.NewRectangle(color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0x10})
	hoverOverlay.CornerRadius = 8
	hoverOverlay.Hide()

	// Create the game image
	image := canvas.NewImageFromFile(g.ImagePath)
	image.FillMode = canvas.ImageFillContain

	// Create a fixed size square container for the image with a border
	imageBox := canvas.NewRectangle(colorMedGray)
	imageBox.CornerRadius = 4

	// Create a container to position the image within the square
	imageContainer := container.NewStack(
		imageBox,
		container.NewPadded(image),
	)

	// Create game name label with improved styling
	nameLabel := canvas.NewText(g.Name, colorCream)
	nameLabel.TextSize = 16
	nameLabel.Alignment = fyne.TextAlignCenter
	nameLabel.TextStyle.Bold = true

	// Container for the name with a bit of padding and fixed height
	nameContainer := container.NewVBox(
		layout.NewSpacer(),
		container.NewCenter(nameLabel),
		layout.NewSpacer(),
	)

	// Create vertical layout with image above name
	content := container.NewVBox(
		container.NewPadded(imageContainer),
		nameContainer,
	)

	return &gameTileRenderer{
		tile:           g,
		background:     background,
		selectedBorder: selectedBorder,
		hoverOverlay:   hoverOverlay,
		imageBox:       imageBox,
		imageContainer: imageContainer,
		nameContainer:  nameContainer,
		content:        content,
		objects:        []fyne.CanvasObject{background, hoverOverlay, selectedBorder, content},
	}
}

type gameTileRenderer struct {
	tile           *GameTile
	background     *canvas.Rectangle
	selectedBorder *canvas.Rectangle
	hoverOverlay   *canvas.Rectangle
	imageBox       *canvas.Rectangle
	imageContainer *fyne.Container
	nameContainer  *fyne.Container
	content        *fyne.Container
	objects        []fyne.CanvasObject
}

func (r *gameTileRenderer) Destroy() {}

func (r *gameTileRenderer) Layout(size fyne.Size) {
	// Set the background, selected border, and hover overlay to fill the entire tile
	r.background.Resize(size)
	r.selectedBorder.Resize(size)
	r.hoverOverlay.Resize(size)

	// Calculate dimensions for a square image area
	imageSize := fyne.Min(size.Width-20, size.Width-20) // Subtract padding

	// Position the content
	r.content.Resize(size)

	// Size the image container as a square
	r.imageContainer.Resize(fyne.NewSize(imageSize, imageSize))
	r.imageBox.Resize(fyne.NewSize(imageSize, imageSize))

	// Size the name container
	nameHeight := float32(40) // Fixed height for name
	r.nameContainer.Resize(fyne.NewSize(size.Width, nameHeight))
}

func (r *gameTileRenderer) MinSize() fyne.Size {
	// Fixed size for all game tiles - width: 180, height: 220
	return fyne.NewSize(180, 220)
}

func (r *gameTileRenderer) Objects() []fyne.CanvasObject {
	return r.objects
}

func (r *gameTileRenderer) Refresh() {
	if r.tile.Selected {
		r.selectedBorder.Show()
	} else {
		r.selectedBorder.Hide()
	}

	if r.tile.Hovered && !r.tile.Selected {
		r.hoverOverlay.Show()
	} else {
		r.hoverOverlay.Hide()
	}

	r.background.Refresh()
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

	ui.status = canvas.NewText("◄ ► ▲ ▼ NAVIGATE    ENTER TO CONTINUE", colorCream)
	ui.status.TextSize = 24
	ui.status.Alignment = fyne.TextAlignCenter
	ui.status.TextStyle.Bold = true

	// Create game tiles and grid
	ui.createGameGrid()

	ui.timeSlider = widget.NewSlider(1, 60)
	ui.timeSlider.Step = 1
	ui.timeSlider.SetValue(5)

	ui.priceLabel = canvas.NewText("", colorCream)
	ui.priceLabel.TextSize = 20
	ui.priceLabel.Alignment = fyne.TextAlignCenter
	ui.updatePrice() // Set initial price

	ui.paymentPrompt = canvas.NewText("PRESS 'P' TO INSERT COIN", colorRed)
	ui.paymentPrompt.TextSize = 26
	ui.paymentPrompt.Alignment = fyne.TextAlignCenter

	// Create header with title and status
	title := canvas.NewText("SPETS ARCADE", colorRed)
	title.TextSize = 36
	title.Alignment = fyne.TextAlignCenter
	title.TextStyle.Bold = true

	header := container.NewVBox(
		container.NewCenter(title),
		container.NewCenter(ui.status),
	)

	// Create time selection container
	timeContainer := container.NewVBox(
		widget.NewLabel("Select Playing Time (minutes):"),
		ui.timeSlider,
		ui.priceLabel,
	)

	// Main content area switches between game grid and time selection
	mainContent := container.NewStack(
		ui.gameScroll, // Use scroll container for games
		container.NewCenter(timeContainer),
		container.NewCenter(ui.paymentPrompt),
	)

	ui.content = container.NewBorder(
		header, nil, nil, nil,
		container.NewPadded(mainContent),
	)

	// Hide elements that aren't needed initially
	ui.timeSlider.Hide()
	ui.priceLabel.Hide()
	ui.paymentPrompt.Hide()

	ui.window.SetContent(ui.content)
	ui.setKeyHandler()

	return ui
}

// createGameGrid creates a responsive grid of game tiles
func (ui *UI) createGameGrid() {
	ui.gameTiles = make([]*GameTile, len(ui.keys))

	// Calculate optimal grid columns based on screen and number of games
	cols := 4 // Default to 4 columns like CurseForge
	if len(ui.keys) <= 6 {
		cols = 3
	} else if len(ui.keys) <= 12 {
		cols = 4
	} else {
		cols = 5
	}

	gridItems := make([]fyne.CanvasObject, len(ui.keys))

	for i, gameName := range ui.keys {
		// Get the image path from the gamePictures map
		imagePath := ui.gamePictures[gameName]
		if imagePath == "" {
			// Fallback to a default image or create placeholder
			imagePath = "/home/simon/Dev/ludo-spets/assets/spets/games/default.png" // Default image
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

		// Add padding around each tile for consistent spacing
		paddedTile := container.NewPadded(ui.gameTiles[i])
		gridItems[i] = paddedTile
	}

	// Create the grid layout with consistent spacing
	ui.gameGrid = container.New(layout.NewGridLayoutWithColumns(cols), gridItems...)

	// Wrap the grid in a scroll container for better navigation
	ui.gameScroll = container.NewScroll(ui.gameGrid)
	ui.gameScroll.SetMinSize(fyne.NewSize(800, 600))
}

// selectGameTile updates the selected game
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

// scrollToSelectedTile ensures the selected game tile is visible
func (ui *UI) scrollToSelectedTile() {
	if ui.selectedIdx < 0 || ui.selectedIdx >= len(ui.gameTiles) {
		return
	}

	// Calculate the position of the selected tile
	cols := 4
	if len(ui.keys) <= 6 {
		cols = 3
	} else if len(ui.keys) <= 12 {
		cols = 4
	} else {
		cols = 5
	}

	row := ui.selectedIdx / cols
	tileHeight := float32(200) // Approximate tile height including padding

	// Scroll to the row containing the selected tile
	scrollOffset := float32(row) * tileHeight
	ui.gameScroll.ScrollToTop()
	if scrollOffset > 0 {
		ui.gameScroll.Offset = fyne.NewPos(0, scrollOffset)
		ui.gameScroll.Refresh()
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
				// Calculate number of columns in the grid
				cols := 4
				if len(ui.keys) <= 6 {
					cols = 3
				} else if len(ui.keys) <= 12 {
					cols = 4
				} else {
					cols = 5
				}

				// Move down in the grid
				if ui.selectedIdx+cols < len(ui.keys) {
					ui.selectGameTile(ui.selectedIdx + cols)
				}
			case fyne.KeyUp:
				// Calculate number of columns in the grid
				cols := 4
				if len(ui.keys) <= 6 {
					cols = 3
				} else if len(ui.keys) <= 12 {
					cols = 4
				} else {
					cols = 5
				}

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
				durationSecs := mins * 2 // Keep original logic

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
	ui.timeSlider.Show()
	ui.priceLabel.Show()
	ui.paymentPrompt.Show()
}