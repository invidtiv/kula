package web

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type wsClient struct {
	conn   *websocket.Conn
	sendCh chan []byte
	paused bool
	mu     sync.Mutex
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// Strict origin check to prevent Cross-Site WebSocket Hijacking (CSWSH)
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true // Allow non-browser clients (like CLI tools) to connect
		}

		// Require the origin host to match the request host exactly
		// Parses the host and ignores scheme (http vs https)
		originHost := ""
		for i := 0; i < len(origin); i++ {
			if origin[i] == ':' && i+2 < len(origin) && origin[i+1] == '/' && origin[i+2] == '/' {
				originHost = origin[i+3:]
				break
			}
		}

		if originHost == "" {
			return false
		}

		if originHost != r.Host {
			log.Printf("WebSocket upgrade blocked: Origin (%s) does not match Host (%s)", originHost, r.Host)
			return false
		}

		return true
	},
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	conn.SetReadLimit(4096) // Limit incoming JSON commands to prevent memory exhaustion

	client := &wsClient{
		conn:   conn,
		sendCh: make(chan []byte, 64),
	}

	s.hub.regCh <- client
	defer func() {
		s.hub.unregCh <- client
		_ = conn.Close()
	}()

	// Send initial current data
	if sample := s.collector.Latest(); sample != nil {
		data, err := json.Marshal(sample)
		if err == nil {
			_ = conn.WriteMessage(websocket.TextMessage, data)
		}
	}

	// Read pump (for pause/resume commands)
	go func() {
		// Set an initial read deadline
		_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		conn.SetPongHandler(func(string) error {
			_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
			return nil
		})

		for {
			var cmd struct {
				Action string `json:"action"`
			}
			err := conn.ReadJSON(&cmd)
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("WebSocket read unexpected error: %v", err)
				}
				s.hub.unregCh <- client
				return
			}

			client.mu.Lock()
			switch cmd.Action {
			case "pause":
				client.paused = true
			case "resume":
				client.paused = false
			}
			client.mu.Unlock()
		}
	}()

	// Write pump
	ticker := time.NewTicker(50 * time.Second) // Must be less than read deadline
	defer ticker.Stop()

	for {
		select {
		case data, ok := <-client.sendCh:
			_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				// The hub closed the channel
				_ = conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				log.Printf("WebSocket write error: %v", err)
				return
			}
		case <-ticker.C:
			_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
