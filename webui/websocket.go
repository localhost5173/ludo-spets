package webui

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// Time allowed to write a message to the peer
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer
	pongWait = 60 * time.Second

	// Send pings to peer with this period
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer
	maxMessageSize = 512
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// Allow all origins for development
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Message represents a message sent between client and server
type Message struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

// Client is a middleman between the websocket connection and the hub
type Client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte
}

// Hub maintains the set of active clients and broadcasts messages
type Hub struct {
	server     *Server
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
}

// newHub creates a new Hub instance
func newHub(server *Server) *Hub {
	return &Hub{
		server:     server,
		broadcast:  make(chan []byte),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		clients:    make(map[*Client]bool),
	}
}

// run processes messages for the hub
func (h *Hub) run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
			// Send initial state to new client
			h.sendStateToClient(client)
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
		case message := <-h.broadcast:
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
		}
	}
}

// broadcastState sends current state to all clients
func (h *Hub) broadcastState() {
	state := h.server.GetState()
	msg := Message{
		Type:    "state",
		Payload: state,
	}

	jsonMsg, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Error marshaling state message: %v", err)
		return
	}

	h.broadcast <- jsonMsg
}

// sendStateToClient sends current state to a specific client
func (h *Hub) sendStateToClient(client *Client) {
	state := h.server.GetState()
	msg := Message{
		Type:    "state",
		Payload: state,
	}

	jsonMsg, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Error marshaling state message: %v", err)
		return
	}

	client.send <- jsonMsg
}

// readPump pumps messages from the websocket to the hub
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error { c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			break
		}

		var msg Message
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Printf("Error unmarshaling message: %v", err)
			continue
		}

		c.handleMessage(msg)
	}
}

// handleMessage processes incoming messages from clients
func (c *Client) handleMessage(msg Message) {
	switch msg.Type {
	case "selectGame":
		if c.hub.server.GetState() == StateSelectGame {
			if gameName, ok := msg.Payload.(string); ok {
				log.Printf("Game selected: %s", gameName)
				c.hub.server.SetState(StateTimeSelect)
			}
		}

	case "selectTime":
		log.Printf("Received selectTime in state: %v", c.hub.server.GetState())
		if c.hub.server.GetState() == StateTimeSelect {
			log.Printf("Transitioning from TimeSelect to Payment")
			c.hub.server.SetState(StatePayment)
		} else if c.hub.server.GetState() == StateExtendTime {
			// When in timeout extension mode, move to payment confirmation step
			log.Printf("Transitioning from ExtendTime to ExtendPayment")
			c.hub.server.SetState(StateExtendPayment)
			// Add a slight delay to ensure state is fully updated
			time.Sleep(100 * time.Millisecond)
			log.Printf("State after transition: %v", c.hub.server.GetState())
		}

	case "payment":
		log.Printf("Received payment message in state: %v", c.hub.server.GetState())

		// Parse payload regardless of state
		paymentData := struct {
			GameName string `json:"gameName"`
			Minutes  int    `json:"minutes"`
		}{}

		payloadBytes, _ := json.Marshal(msg.Payload)
		if err := json.Unmarshal(payloadBytes, &paymentData); err != nil {
			log.Printf("Error parsing payment data: %v", err)
			return
		}

		log.Printf("Payment data parsed: Game: %s, Minutes: %d", paymentData.GameName, paymentData.Minutes)

		// Expanded state handling for payment
		currentState := c.hub.server.GetState()
		if currentState == StatePayment {
			// Initial game launch
			log.Printf("Processing initial game payment")
			c.hub.server.LaunchGame(paymentData.GameName, paymentData.Minutes)
		} else if currentState == StateExtendPayment || currentState == StateExtendTime {
			// Accept payment in either ExtendTime or ExtendPayment state
			log.Printf("Processing time extension payment in state: %v", currentState)

			// Convert minutes to seconds (multiply by 2 for testing)
			newDurationSecs := paymentData.Minutes * 2

			// Send signals in the correct order with error handling
			log.Printf("Sending resume signal...")
			select {
			case c.hub.server.resumeChan <- true:
				log.Println("Resume signal sent successfully")
			case <-time.After(500 * time.Millisecond):
				log.Println("Warning: Resume channel is full or blocked")
			}

			// Small delay to ensure resume signal is processed
			time.Sleep(100 * time.Millisecond)

			log.Printf("Sending new timer duration: %d seconds", newDurationSecs)
			select {
			case c.hub.server.timerChan <- newDurationSecs:
				log.Println("Timer duration sent successfully")
			case <-time.After(500 * time.Millisecond):
				log.Println("Warning: Timer channel is full or blocked")
			}

			// Wait a bit more to ensure signals are processed
			time.Sleep(200 * time.Millisecond)

			// IMPORTANT: Do NOT close the browser - just minimize it
			log.Printf("Minimizing browser window...")
			c.hub.server.minimizeBrowser()

			// Set state to GameActive as the game should be running
			log.Printf("Setting state to GameActive")
			c.hub.server.SetState(StateGameActive)
		} else {
			log.Printf("Ignoring payment message in unsupported state: %v", currentState)
		}

	case "quit":
		// Handle player choosing to quit the game
		if c.hub.server.GetState() == StateExtendTime {
			log.Println("Player chose to quit game")
			// IMPORTANT: Do NOT close the browser
			// Just go back to game selection state
			c.hub.server.SetState(StateSelectGame)
		}
	}
}

// writePump pumps messages from the hub to the websocket connection
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The hub closed the channel
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Add queued chat messages to the current websocket message
			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write(bytes.TrimSpace([]byte{'\n'}))
				w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// serveWs handles websocket requests from the peer
func serveWs(hub *Hub, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}

	client := &Client{hub: hub, conn: conn, send: make(chan []byte, 256)}
	client.hub.register <- client

	// Allow collection of memory referenced by the caller by doing all work in new goroutines
	go client.writePump()
	go client.readPump()
}
