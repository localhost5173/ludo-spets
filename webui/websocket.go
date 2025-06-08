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
	server    *Server
	clients   map[*Client]bool
	broadcast chan []byte
	register  chan *Client
	unregister chan *Client
}

// newHub creates a new Hub instance
func newHub(server *Server) *Hub {
	return &Hub{
		server:    server,
		broadcast: make(chan []byte),
		register:  make(chan *Client),
		unregister: make(chan *Client),
		clients:   make(map[*Client]bool),
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
		if c.hub.server.GetState() == StateTimeSelect {
			c.hub.server.SetState(StatePayment)
		} else if c.hub.server.GetState() == StateExtendTime {
			// When in timeout extension mode, move to payment confirmation step
			log.Printf("Moving from time selection to payment confirmation")
			c.hub.server.SetState(StateExtendPayment)
		}
		
	case "payment":
		if c.hub.server.GetState() == StatePayment || c.hub.server.GetState() == StateExtendPayment {
			// Parse payload
			paymentData := struct {
				GameName string `json:"gameName"`
				Minutes  int    `json:"minutes"`
			}{}
			
			payloadBytes, _ := json.Marshal(msg.Payload)
			if err := json.Unmarshal(payloadBytes, &paymentData); err != nil {
				log.Printf("Error parsing payment data: %v", err)
				return
			}
			
			// Handle payment and game launch
			if c.hub.server.GetState() == StatePayment {
				 // Initial game launch - close browser windows before launching game
				c.hub.server.LaunchGame(paymentData.GameName, paymentData.Minutes)
			} else {
				// Handle time extension
				log.Printf("Resuming game with %d more minutes", paymentData.Minutes)
				
				// Send resume signal before closing browser - in case client needs to respond
				c.hub.server.resumeChan <- true
				c.hub.server.timerChan <- paymentData.Minutes * 2 // Change to 2 minutes for testing
				
				// Small delay to ensure signals are processed
				time.Sleep(100 * time.Millisecond)
				
				// Now close the browser windows
				c.hub.server.closeBrowserWindows()
				
				// Change state back to game selection after sending signals
				// This is important so if a new window opens, it shows the game selection
				time.AfterFunc(500*time.Millisecond, func() {
					c.hub.server.SetState(StateSelectGame)
				})
			}
		}
		
	case "quit":
		// Handle player choosing to quit the game
		if c.hub.server.GetState() == StateExtendTime {
			log.Println("Player chose to quit game")
			c.hub.server.closeBrowserWindows()
			c.hub.server.SetState(StateSelectGame)
			
			// Launch a new browser window for game selection
			time.AfterFunc(500*time.Millisecond, func() {
				url := "http://localhost:8080"
				c.hub.server.launchBrowserFullscreen(url)
			})
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