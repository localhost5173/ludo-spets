package webui

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
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

// launchBrowserFullscreen opens a browser in fullscreen/kiosk mode on the primary display
func (s *Server) launchBrowserFullscreen(url string) {
	// Close any existing browser windows first
	s.closeBrowserWindows()

	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "linux":
		if _, err := exec.LookPath("google-chrome"); err == nil {
			cmd = exec.Command("google-chrome",
				"--kiosk",
				"--new-window",
				"--window-position=0,0",
				"--display=:0.0",
				"--start-maximized",
				"--start-fullscreen",
				"--no-first-run",
				"--app="+url)
		} else if _, err := exec.LookPath("chromium-browser"); err == nil {
			cmd = exec.Command("chromium-browser", "--kiosk", "--window-position=0,0", "--display=:0.0", "--app="+url)
		} else if _, err := exec.LookPath("firefox"); err == nil {
			cmd = exec.Command("firefox", "--kiosk", "--display=:0.0", "--new-instance", url)
		}
	case "darwin":
		if _, err := exec.LookPath("firefox"); err == nil {
			cmd = exec.Command("open", "-a", "Firefox", url)
		} else {
			cmd = exec.Command("open", "-a", "Safari", url)
		}
	case "windows":
		firefoxPath := "C:\\Program Files\\Mozilla Firefox\\firefox.exe"
		if _, err := os.Stat(firefoxPath); err == nil {
			cmd = exec.Command(firefoxPath, "--kiosk", url)
		} else {
			cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
		}
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
		log.Println("Could not find a suitable browser to launch")
	}
}

// closeBrowserWindows closes all browser windows
func (s *Server) closeBrowserWindows() {
	log.Println("Closing all browser windows...")

	// Try using command-line tools first for graceful termination
	switch runtime.GOOS {
	case "linux":
		closeCmds := []string{
			"wmctrl -c 'SPETS ARCADE'",
			"wmctrl -c 'Chrome'",
			"wmctrl -c 'Google Chrome'",
			"pkill -f 'chrome.*localhost:8080'",
			"pkill -f 'chromium.*localhost:8080'",
		}

		for _, cmdStr := range closeCmds {
			exec.Command("bash", "-c", cmdStr).Run()
		}

	case "darwin":
		exec.Command("osascript", "-e", `tell application "Google Chrome" to quit`).Run()
		exec.Command("osascript", "-e", `tell application "Chrome" to quit`).Run()

	case "windows":
		exec.Command("taskkill", "/F", "/IM", "chrome.exe").Run()
	}

	// Then terminate our tracked processes
	if s.browserCmd != nil && s.browserCmd.Process != nil {
		s.browserCmd.Process.Kill()
		s.browserCmd = nil
	}

	// Also kill any previously tracked browser processes
	for _, cmd := range s.previousBrowsers {
		if cmd != nil && cmd.Process != nil {
			cmd.Process.Kill()
		}
	}

	// Clear the array
	s.previousBrowsers = nil
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

		// Wait for game loaded confirmation or timeout
		select {
		case <-s.gameLoadedChan:
			log.Println("Game loaded successfully, closing browser")
			s.closeBrowserWindows()
		case <-time.After(10 * time.Second):
			log.Println("Timeout waiting for game to load, closing browser anyway")
			s.closeBrowserWindows()
		}
	}()
}

// OnGameLoaded should be called when Ludo has successfully loaded the game
func (s *Server) OnGameLoaded() {
	log.Println("Server: Game loaded confirmation received")
	select {
	case s.gameLoadedChan <- true:
	default:
		// Channel is full, ignore
	}
}

// PrepareTimeout opens a browser window in preparation for timeout (called ~10 seconds before)
func (s *Server) PrepareTimeout() {
	log.Println("Preparing for timeout - opening browser window")
	
	// Close any existing browser windows first
	s.closeBrowserWindows()
	
	// Launch a new browser window but keep it in background
	url := "http://localhost:8080"
	s.launchBrowserFullscreen(url)
	
	// Give the browser time to fully load
	time.Sleep(2 * time.Second)
}

// HandleTimeout is called when the game timer expires
func (s *Server) HandleTimeout() {
	// Read up to 4 more values from the channel for window positioning
	var x, y, width, height int

	// Use select with short timeout to avoid blocking if the values aren't available
	timeout := time.After(100 * time.Millisecond)

	// Try to read X position
	select {
	case x = <-s.timerChan:
		// Successfully read X position
	case <-timeout:
		log.Println("Timeout reading game window X position")
		x = 0
	}

	// Try to read Y position
	select {
	case y = <-s.timerChan:
		// Successfully read Y position
	case <-timeout:
		log.Println("Timeout reading game window Y position")
		y = 0
	}

	// Try to read width
	select {
	case width = <-s.timerChan:
		// Successfully read width
	case <-timeout:
		log.Println("Timeout reading game window width")
		width = 800
	}

	// Try to read height
	select {
	case height = <-s.timerChan:
		// Successfully read height
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

	// First launch a new browser window since the previous one was closed
	log.Println("Timeout occurred, launching a new browser window")
	url := "http://localhost:8080"
	s.launchBrowserFullscreen(url)
	
	// Give the browser window a moment to fully load before changing state
	// and trying to interact with it
	time.Sleep(500 * time.Millisecond)
	
	// Now that the browser is launched, change state to extend time
	// This will trigger the UI updates in the browser
	s.SetState(StateExtendTime)

	// Send window position to clients
	s.broadcastWindowPosition()

	// Try to force the browser window to the foreground
	log.Println("Bringing browser window to foreground for timeout screen")
	time.Sleep(200 * time.Millisecond) // Give window a moment to initialize
	s.forceBrowserToForeground()
	
	// Additional attempt to force focus with a slight delay
	time.AfterFunc(1*time.Second, func() {
		log.Println("Making additional attempt to focus browser window")
		s.forceBrowserToForeground()
	})
}

// forceBrowserToForeground brings the browser window to the foreground
func (s *Server) forceBrowserToForeground() {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "linux":
		if _, err := exec.LookPath("wmctrl"); err == nil {
			// Try multiple window title variations
			titles := []string{"SPETS ARCADE", "SPETS ARCADE - TIMEOUT OVERLAY", "Google Chrome", "Chrome"}
			
			for _, title := range titles {
				cmd = exec.Command("wmctrl", "-a", title)
				log.Printf("Trying to focus window with title: %s", title)
				err := cmd.Run()
				if err == nil {
					log.Printf("Successfully focused window with title: %s", title)
					return
				}
			}
			
			// If specific titles fail, try a more general approach
			generalCmd := exec.Command("bash", "-c", "wmctrl -a Chrome || wmctrl -a Firefox || wmctrl -R localhost:8080")
			if err := generalCmd.Run(); err != nil {
				log.Printf("Failed to focus any browser window: %v", err)
			}
		}
	case "darwin":
		cmd = exec.Command("osascript", "-e", `tell application "Google Chrome" to activate`)
	case "windows":
		ps := `
        Add-Type @"
        using System;
        using System.Runtime.InteropServices;
        public class WindowHelper {
            [DllImport("user32.dll")]
            [return: MarshalAs(UnmanagedType.Bool)]
            public static extern bool SetForegroundWindow(IntPtr hWnd);
            
            [DllImport("user32.dll")]
            public static extern IntPtr FindWindow(string lpClassName, string lpWindowName);
        }
"@
        $chromeWindow = [WindowHelper]::FindWindow("Chrome_WidgetWin_1", $null)
        if ($chromeWindow -ne [IntPtr]::Zero) {
            [WindowHelper]::SetForegroundWindow($chromeWindow)
        }
        `
		cmd = exec.Command("powershell", "-Command", ps)
	}

	if cmd != nil {
		log.Printf("Executing browser focus command: %s", cmd.String())
		err := cmd.Run()
		if err != nil {
			log.Printf("Failed to bring browser to foreground: %v", err)
		} else {
			log.Printf("Successfully brought browser to foreground")
		}
	}
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