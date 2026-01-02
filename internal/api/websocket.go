// Package api - WebSocket handler for real-time game sessions
package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/alexbotov/rgs/internal/domain"
	"github.com/alexbotov/rgs/internal/game"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for development
	},
}

// WSMessage represents a WebSocket message
type WSMessage struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// WSClient represents a WebSocket client connection
type WSClient struct {
	conn      *websocket.Conn
	send      chan []byte
	sessionID string
	playerID  string
	mu        sync.Mutex
}

// HandleWebSocket handles WebSocket connections for game sessions
func (h *Handler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	session := r.Context().Value("session").(*domain.Session)
	player := r.Context().Value("player").(*domain.Player)
	gameSessionID := mux.Vars(r)["session_id"]

	// Verify game session exists and belongs to player
	gameSession, err := h.game.GetSession(r.Context(), gameSessionID)
	if err != nil {
		http.Error(w, "Game session not found", http.StatusNotFound)
		return
	}
	if gameSession.PlayerID != player.ID {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	if gameSession.Status != domain.GameSessionActive {
		http.Error(w, "Game session is not active", http.StatusBadRequest)
		return
	}

	// Upgrade connection
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	client := &WSClient{
		conn:      conn,
		send:      make(chan []byte, 256),
		sessionID: gameSessionID,
		playerID:  player.ID,
	}

	// Start goroutines for reading and writing
	go client.writePump()
	go h.readPump(client, session.ID)
}

// writePump pumps messages from the send channel to the WebSocket connection
func (c *WSClient) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)
			w.Close()

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// readPump pumps messages from the WebSocket connection to the handler
func (h *Handler) readPump(c *WSClient, authSessionID string) {
	defer func() {
		close(c.send)
		c.conn.Close()
	}()

	c.conn.SetReadLimit(4096)
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	// Send welcome message
	h.sendMessage(c, "connected", map[string]interface{}{
		"session_id": c.sessionID,
		"message":    "Connected to game session",
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		// Parse message
		var msg WSMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			h.sendError(c, "INVALID_MESSAGE", "Invalid message format")
			continue
		}

		// Handle message
		h.handleWSMessage(c, &msg)
	}
}

// handleWSMessage processes incoming WebSocket messages
func (h *Handler) handleWSMessage(c *WSClient, msg *WSMessage) {
	ctx := context.Background()

	switch msg.Type {
	case "spin", "play":
		h.handlePlayMessage(c, msg)

	case "balance":
		balance, err := h.wallet.GetBalance(ctx, c.playerID)
		if err != nil {
			h.sendError(c, "BALANCE_ERROR", "Failed to get balance")
			return
		}
		h.sendMessage(c, "balance", map[string]interface{}{
			"real_money": balance.RealMoney.Float64(),
			"available":  balance.Available.Float64(),
			"currency":   balance.Currency,
		})

	case "history":
		history, err := h.game.GetHistory(ctx, c.playerID, 10)
		if err != nil {
			h.sendError(c, "HISTORY_ERROR", "Failed to get history")
			return
		}
		h.sendMessage(c, "history", history)

	case "session_info":
		session, err := h.game.GetSession(ctx, c.sessionID)
		if err != nil {
			h.sendError(c, "SESSION_ERROR", "Failed to get session")
			return
		}
		h.sendMessage(c, "session_info", map[string]interface{}{
			"session_id":    session.ID,
			"game_id":       session.GameID,
			"games_played":  session.GamesPlayed,
			"total_wagered": session.TotalWagered.Float64(),
			"total_won":     session.TotalWon.Float64(),
			"status":        session.Status,
		})

	case "ping":
		h.sendMessage(c, "pong", map[string]interface{}{
			"timestamp": time.Now().Unix(),
		})

	default:
		h.sendError(c, "UNKNOWN_MESSAGE", "Unknown message type: "+msg.Type)
	}
}

// handlePlayMessage processes play/spin messages
func (h *Handler) handlePlayMessage(c *WSClient, msg *WSMessage) {
	ctx := context.Background()

	// Parse wager amount
	var payload struct {
		WagerAmount int64 `json:"wager_amount"`
	}
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		h.sendError(c, "INVALID_PAYLOAD", "Invalid wager payload")
		return
	}

	// Execute play
	result, err := h.game.Play(ctx, &game.PlayRequest{
		SessionID:   c.sessionID,
		WagerAmount: payload.WagerAmount,
	})
	if err != nil {
		switch err {
		case game.ErrInsufficientBalance:
			h.sendError(c, "INSUFFICIENT_BALANCE", "Insufficient balance")
		case game.ErrInvalidWager:
			h.sendError(c, "INVALID_WAGER", "Invalid wager amount")
		case game.ErrSessionNotActive:
			h.sendError(c, "SESSION_NOT_ACTIVE", "Game session is not active")
		default:
			h.sendError(c, "GAME_ERROR", err.Error())
		}
		return
	}

	// Send result
	h.sendMessage(c, "outcome", map[string]interface{}{
		"cycle_id":     result.CycleID,
		"outcome":      result.Outcome,
		"wager_amount": result.WagerAmount.Float64(),
		"win_amount":   result.WinAmount.Float64(),
		"balance":      result.Balance.Float64(),
		"is_win":       result.Outcome.IsWin,
	})
}

// sendMessage sends a message to the client
func (h *Handler) sendMessage(c *WSClient, msgType string, payload interface{}) {
	payloadBytes, _ := json.Marshal(payload)
	msg := WSMessage{
		Type:    msgType,
		Payload: payloadBytes,
	}
	msgBytes, _ := json.Marshal(msg)

	c.mu.Lock()
	defer c.mu.Unlock()

	select {
	case c.send <- msgBytes:
	default:
		// Channel full, drop message
	}
}

// sendError sends an error message to the client
func (h *Handler) sendError(c *WSClient, code, message string) {
	h.sendMessage(c, "error", map[string]string{
		"code":    code,
		"message": message,
	})
}

