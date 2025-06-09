package webui

import (
	"encoding/json"
	"log"
	"net/http"
	"os/exec"
	"sync"
	"time"

	"github.com/libretro/ludo/ludo"
)

// ServerState represents different states of the application
type ServerState int

const (
	StateSelectGame ServerState = iota
	StateTimeSelect
	StatePayment
	StateExtendTime
	StateExtendPayment
	StateGameLoading // Add new state for when game is loading
	StateGameActive  // New state for when game is active after extension
)

// Server holds the web server state and data
type Server struct {
	games            map[string]string
	cores            map[string]string
	gamePictures     map[string]string
	timerChan        chan int
	resumeChan       chan bool
	gameLoadedChan   chan bool // Add channel for game loading confirmation
	hub              *Hub
	state            ServerState
	stateMutex       sync.RWMutex
	gameWindowX      int
	gameWindowY      int
	gameWindowWidth  int
	gameWindowHeight int
	gameWindowMutex  sync.RWMutex
	browserCmd       *exec.Cmd   // Current browser process
	browserPID       int         // Current browser PID
	previousBrowsers []*exec.Cmd // Track previously opened browsers
}

const PricePerMinute = 0.5

// NewServer creates a new web server instance
func NewServer(games, cores, gamePictures map[string]string, timerChan chan int, resumeChan chan bool) *Server {
	s := &Server{
		games:          games,
		cores:          cores,
		gamePictures:   gamePictures,
		timerChan:      timerChan,
		resumeChan:     resumeChan,
		gameLoadedChan: make(chan bool, 1), // Add buffered channel for game loading
		state:          StateSelectGame,
	}

	// Create WebSocket hub
	s.hub = newHub(s)
	go s.hub.run()

	return s
}

// Start launches the web server
func (s *Server) Start(addr string) error {
	// Serve static files
	fs := http.FileServer(http.Dir("./webui/static"))
	http.Handle("/", fs)

	// API endpoints
	http.HandleFunc("/api/games", s.handleGames)

	// WebSocket endpoint
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWs(s.hub, w, r)
	})

	// Start the server in a goroutine so we can launch the browser
	go func() {
		log.Printf("Web UI server starting on %s", addr)
		err := http.ListenAndServe(addr, nil)
		if err != nil {
			log.Fatalf("Failed to start web server: %v", err)
		}
	}()

	// Give the server a moment to start
	time.Sleep(500 * time.Millisecond)

	// Launch browser in fullscreen/kiosk mode
	url := "http://localhost" + addr
	s.launchBrowserFullscreen(url)

	// Keep the main function from exiting
	select {}
}

func (s *Server) maximizeGame() {
	// Use wmctrl to maximize the game window
	log.Println("Attempting to maximize the Ludo game window")
	cmd := exec.Command("wmctrl", "-a", "Ludo -")
	if err := cmd.Run(); err != nil {
		log.Printf("Failed to maximize Ludo game window: %v", err)
		return
	}
	log.Println("Successfully maximized Ludo game window using wmctrl")
}

// minimizeGameWindow sends the Ludo game window to background/minimizes it
func (s *Server) maximizeBrowser() {
	// Use wmctrl to maximize the browser window
	log.Println("Attempting to maximize the browser window")
	cmd := exec.Command("wmctrl", "-a", "SPETS ARCADE")
	if err := cmd.Run(); err != nil {
		log.Printf("Failed to maximize browser window: %v", err)
		return
	}
}

// launchBrowserFullscreen opens a browser in fullscreen/kiosk mode on the primary display
func (s *Server) launchBrowserFullscreen(url string) {
	var cmd *exec.Cmd

	// Linux-only implementation for KDE X11 with chromium-browser
	if _, err := exec.LookPath("chromium-browser"); err == nil {
		cmd = exec.Command("chromium-browser",
			"--new-window",
			"--window-position=0,0",
			"--display=:0.0",
			"--start-maximized",
			"--no-first-run",
			"--disable-web-security",
			"--disable-features=VizDisplayCompositor",
			"--app="+url)
	} else {
		log.Println("chromium-browser not found")
		return
	}

	if cmd != nil {
		log.Printf("Launching browser: %s", cmd.String())
		err := cmd.Start()
		if err != nil {
			log.Printf("Failed to open browser: %v", err)
		} else {
			log.Printf("Browser launched with PID: %d", cmd.Process.Pid)
			s.browserCmd = cmd
			s.browserPID = cmd.Process.Pid
			s.previousBrowsers = append(s.previousBrowsers, cmd)
		}
	} else {
		log.Println("Could not find chromium-browser to launch")
	}
}

// HandleTimeout is called when the game timer expires - updated to minimize game window
func (s *Server) HandleTimeout() {
	// Read window positioning information
	var x, y, width, height int

	// Use select with short timeout to avoid blocking if values aren't available
	timeout := time.After(200 * time.Millisecond)

	// Try to read X position
	select {
	case x = <-s.timerChan:
	case <-timeout:
		log.Println("Timeout reading game window X position")
		x = 0
	}

	// Try to read Y position
	select {
	case y = <-s.timerChan:
	case <-timeout:
		log.Println("Timeout reading game window Y position")
		y = 0
	}

	// Try to read width
	select {
	case width = <-s.timerChan:
	case <-timeout:
		log.Println("Timeout reading game window width")
		width = 800
	}

	// Try to read height
	select {
	case height = <-s.timerChan:
	case <-timeout:
		log.Println("Timeout reading game window height")
		height = 600
	}

	// Store window information
	s.gameWindowMutex.Lock()
	s.gameWindowX = x
	s.gameWindowY = y
	s.gameWindowWidth = width
	s.gameWindowHeight = height
	s.gameWindowMutex.Unlock()

	log.Println("Timeout occurred, maximizing browser window to reveal web UI")

	// IMPORTANT: Force the state change and ensure it's broadcast
	log.Printf("Current state before timeout: %v", s.GetState())
	s.SetState(StateExtendTime)
	log.Printf("State after timeout: %v", s.GetState())

	// Give a moment for the state to be processed
	time.Sleep(100 * time.Millisecond)

	// Send window position to clients
	s.broadcastWindowPosition()

	// Use wmctrl to maximize the browser window
	s.maximizeBrowser()

	// Additional attempt with a slight delay
	time.AfterFunc(1*time.Second, func() {
		log.Println("Making additional attempt to maximize browser window")
		s.maximizeBrowser()
	})
}

// PrepareTimeout just ensures browser is ready and warns about upcoming timeout
func (s *Server) PrepareTimeout() {
	log.Println("Preparing for timeout - game will be minimized in 10 seconds")

	// Send a message to prepare the browser for timeout
	msg := Message{
		Type: "prepare_timeout",
		Payload: map[string]interface{}{
			"message": "Game will pause in 10 seconds",
		},
	}

	jsonMsg, _ := json.Marshal(msg)
	s.hub.broadcast <- jsonMsg
}

// handleGames returns the list of available games
func (s *Server) handleGames(w http.ResponseWriter, r *http.Request) {
	type GameInfo struct {
		Name      string `json:"name"`
		ImagePath string `json:"imagePath"`
	}

	games := make([]GameInfo, 0, len(s.games))
	for name := range s.games {
		games = append(games, GameInfo{
			Name:      name,
			ImagePath: s.gamePictures[name],
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(games)
}

// GetState returns the current UI state
func (s *Server) GetState() ServerState {
	s.stateMutex.RLock()
	defer s.stateMutex.RUnlock()
	return s.state
}

// SetState updates the UI state
func (s *Server) SetState(state ServerState) {
	s.stateMutex.Lock()
	s.state = state
	s.stateMutex.Unlock()

	// Broadcast state change to all clients
	s.hub.broadcastState()
}

// LaunchGame starts a game with the given name and time
func (s *Server) LaunchGame(gameName string, minutes int) {
	log.Printf("Launching game: %s for %d minutes", gameName, minutes)

	// Set state to loading
	s.SetState(StateGameLoading)

	// Send loading message to clients
	msg := Message{
		Type: "game_loading",
		Payload: map[string]interface{}{
			"message": "Starting game...",
		},
	}

	jsonMsg, _ := json.Marshal(msg)
	s.hub.broadcast <- jsonMsg

	corePath := s.cores[gameName]
	gamePath := s.games[gameName]
	durationSecs := minutes * 2 // Change to 2 minutes for testing

	// Launch game in goroutine
	go func() {
		err := s.launchLudoGame(corePath, gamePath, durationSecs)
		if err != nil {
			log.Printf("Error launching game: %v", err)
			// If game launch fails, go back to game selection
			s.SetState(StateSelectGame)
			return
		}
	}()
}

// OnGameLoaded should be called when Ludo has successfully loaded the game
func (s *Server) OnGameLoaded() {
	log.Println("Server: Game loaded confirmation received")

	// Set game active state first
	s.SetState(StateGameActive)

	// Send message to browser to indicate game is running
	msg := Message{
		Type: "game_started",
		Payload: map[string]interface{}{
			"message": "Game loaded successfully, game is now active",
		},
	}

	jsonMsg, _ := json.Marshal(msg)
	s.hub.broadcast <- jsonMsg

	// Let the game channel know we received the signal
	select {
	case s.gameLoadedChan <- true:
	default:
		// Channel is full, ignore
	}

	// Use wmctrl to maximize the game window
	s.maximizeGame()
}

// broadcastWindowPosition sends the game window position to all clients
func (s *Server) broadcastWindowPosition() {
	s.gameWindowMutex.RLock()
	windowInfo := struct {
		X      int `json:"x"`
		Y      int `json:"y"`
		Width  int `json:"width"`
		Height int `json:"height"`
	}{
		X:      s.gameWindowX,
		Y:      s.gameWindowY,
		Width:  s.gameWindowWidth,
		Height: s.gameWindowHeight,
	}
	s.gameWindowMutex.RUnlock()

	msg := Message{
		Type:    "window_position",
		Payload: windowInfo,
	}

	// Broadcast to all clients through hub
	jsonData, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Error marshaling window position: %v", err)
		return
	}

	s.hub.broadcast <- jsonData
}

// launchLudoGame launches a game using the ludo package
func (s *Server) launchLudoGame(corePath, gamePath string, durationSecs int) error {
	return ludo.RunGame(corePath, gamePath, durationSecs, s.timerChan, s.resumeChan)
}

// RunGame is a wrapper around ludo.RunGame
func RunGame(corePath, gamePath string, durationSecs int, timerChan chan int, resumeChan chan bool) error {
	// Forward the call to ludo package
	return ludo.RunGame(corePath, gamePath, durationSecs, timerChan, resumeChan)
}
