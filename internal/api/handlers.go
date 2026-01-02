// Package api provides HTTP API handlers for the RGS
// Implements REST API as per Technical Specification §12
package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/alexbotov/rgs/internal/auth"
	"github.com/alexbotov/rgs/internal/domain"
	"github.com/alexbotov/rgs/internal/game"
	"github.com/alexbotov/rgs/internal/rng"
	"github.com/alexbotov/rgs/internal/wallet"
	"github.com/gorilla/mux"
)

// Handler contains all HTTP handlers
type Handler struct {
	auth   *auth.Service
	wallet *wallet.Service
	game   *game.Engine
	rng    *rng.Service
}

// New creates a new API handler
func New(authSvc *auth.Service, walletSvc *wallet.Service, gameEngine *game.Engine, rngSvc *rng.Service) *Handler {
	return &Handler{
		auth:   authSvc,
		wallet: walletSvc,
		game:   gameEngine,
		rng:    rngSvc,
	}
}

// Response helpers

type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   *APIError   `json:"error,omitempty"`
}

type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(APIResponse{
		Success: status >= 200 && status < 300,
		Data:    data,
	})
}

func respondError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(APIResponse{
		Success: false,
		Error: &APIError{
			Code:    code,
			Message: message,
		},
	})
}

// getClientIP extracts client IP from request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		return strings.TrimSpace(ips[0])
	}
	// Check X-Real-IP header
	if xrip := r.Header.Get("X-Real-IP"); xrip != "" {
		return xrip
	}
	// Fall back to RemoteAddr
	ip := r.RemoteAddr
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		ip = ip[:idx]
	}
	return ip
}

// === Health & Info ===

// HealthCheck handles GET /health
func (h *Handler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	// Check RNG health (GLI-19 §3.3.3)
	rngHealth, _ := h.rng.HealthCheck()
	
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status":     "healthy",
		"rng_status": rngHealth,
	})
}

// ServerInfo handles GET /
func (h *Handler) ServerInfo(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"name":        "RGS",
		"version":     "1.0.0",
		"description": "Remote Gaming Server - GLI-19 Compliant",
	})
}

// === Authentication ===

// Register handles POST /api/v1/auth/register
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req auth.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
		return
	}

	player, err := h.auth.Register(r.Context(), &req, getClientIP(r))
	if err != nil {
		switch err {
		case auth.ErrUserExists:
			respondError(w, http.StatusConflict, "USER_EXISTS", "Username or email already exists")
		default:
			respondError(w, http.StatusBadRequest, "REGISTRATION_FAILED", err.Error())
		}
		return
	}

	respondJSON(w, http.StatusCreated, map[string]interface{}{
		"player_id": player.ID,
		"username":  player.Username,
		"message":   "Registration successful",
	})
}

// Login handles POST /api/v1/auth/login
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req auth.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
		return
	}

	result, err := h.auth.Login(r.Context(), &req, getClientIP(r), r.UserAgent())
	if err != nil {
		switch err {
		case auth.ErrInvalidCredentials:
			respondError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Invalid username or password")
		case auth.ErrAccountLocked:
			respondError(w, http.StatusForbidden, "ACCOUNT_LOCKED", "Account is temporarily locked")
		case auth.ErrAccountNotActive:
			respondError(w, http.StatusForbidden, "ACCOUNT_INACTIVE", "Account is not active")
		default:
			respondError(w, http.StatusInternalServerError, "LOGIN_FAILED", "Login failed")
		}
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"token":      result.Token,
		"session_id": result.Session.ID,
		"player": map[string]interface{}{
			"id":       result.Player.ID,
			"username": result.Player.Username,
			"email":    result.Player.Email,
		},
		"expires_at": result.Session.ExpiresAt,
	})
}

// Logout handles POST /api/v1/auth/logout
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	session := r.Context().Value("session").(*domain.Session)
	
	if err := h.auth.Logout(r.Context(), session.ID); err != nil {
		respondError(w, http.StatusInternalServerError, "LOGOUT_FAILED", "Logout failed")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{
		"message": "Logged out successfully",
	})
}

// GetSession handles GET /api/v1/auth/session
func (h *Handler) GetSession(w http.ResponseWriter, r *http.Request) {
	session := r.Context().Value("session").(*domain.Session)
	player := r.Context().Value("player").(*domain.Player)

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"session_id": session.ID,
		"player": map[string]interface{}{
			"id":       player.ID,
			"username": player.Username,
			"email":    player.Email,
			"status":   player.Status,
		},
		"created_at":       session.CreatedAt,
		"last_activity_at": session.LastActivityAt,
		"expires_at":       session.ExpiresAt,
	})
}

// === Wallet ===

// GetBalance handles GET /api/v1/wallet/balance
func (h *Handler) GetBalance(w http.ResponseWriter, r *http.Request) {
	player := r.Context().Value("player").(*domain.Player)

	balance, err := h.wallet.GetBalance(r.Context(), player.ID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "BALANCE_ERROR", "Failed to get balance")
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"real_money":    balance.RealMoney.Float64(),
		"bonus_balance": balance.BonusBalance.Float64(),
		"available":     balance.Available.Float64(),
		"currency":      balance.Currency,
	})
}

// Deposit handles POST /api/v1/wallet/deposit
func (h *Handler) Deposit(w http.ResponseWriter, r *http.Request) {
	player := r.Context().Value("player").(*domain.Player)

	var req struct {
		Amount    float64 `json:"amount"`
		Reference string  `json:"reference"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
		return
	}

	if req.Amount <= 0 {
		respondError(w, http.StatusBadRequest, "INVALID_AMOUNT", "Amount must be positive")
		return
	}

	amount := domain.NewMoney(req.Amount, "USD")
	tx, err := h.wallet.Deposit(r.Context(), player.ID, amount, req.Reference)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DEPOSIT_FAILED", err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"transaction_id": tx.ID,
		"amount":         tx.Amount.Float64(),
		"balance_after":  tx.BalanceAfter.Float64(),
		"status":         tx.Status,
	})
}

// Withdraw handles POST /api/v1/wallet/withdraw
func (h *Handler) Withdraw(w http.ResponseWriter, r *http.Request) {
	player := r.Context().Value("player").(*domain.Player)

	var req struct {
		Amount    float64 `json:"amount"`
		Reference string  `json:"reference"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
		return
	}

	if req.Amount <= 0 {
		respondError(w, http.StatusBadRequest, "INVALID_AMOUNT", "Amount must be positive")
		return
	}

	amount := domain.NewMoney(req.Amount, "USD")
	tx, err := h.wallet.Withdraw(r.Context(), player.ID, amount, req.Reference)
	if err != nil {
		switch err {
		case wallet.ErrInsufficientFunds:
			respondError(w, http.StatusBadRequest, "INSUFFICIENT_FUNDS", "Insufficient funds")
		default:
			respondError(w, http.StatusInternalServerError, "WITHDRAWAL_FAILED", err.Error())
		}
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"transaction_id": tx.ID,
		"amount":         tx.Amount.Float64(),
		"balance_after":  tx.BalanceAfter.Float64(),
		"status":         tx.Status,
	})
}

// GetTransactions handles GET /api/v1/wallet/transactions
func (h *Handler) GetTransactions(w http.ResponseWriter, r *http.Request) {
	player := r.Context().Value("player").(*domain.Player)

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}

	transactions, err := h.wallet.GetTransactions(r.Context(), player.ID, limit)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "TRANSACTIONS_ERROR", "Failed to get transactions")
		return
	}

	// Convert to response format
	txList := make([]map[string]interface{}, len(transactions))
	for i, tx := range transactions {
		txList[i] = map[string]interface{}{
			"id":             tx.ID,
			"type":           tx.Type,
			"amount":         tx.Amount.Float64(),
			"balance_before": tx.BalanceBefore.Float64(),
			"balance_after":  tx.BalanceAfter.Float64(),
			"status":         tx.Status,
			"description":    tx.Description,
			"created_at":     tx.CreatedAt,
		}
	}

	respondJSON(w, http.StatusOK, txList)
}

// === Games ===

// GetGames handles GET /api/v1/games
func (h *Handler) GetGames(w http.ResponseWriter, r *http.Request) {
	games := h.game.GetGames()

	gameList := make([]map[string]interface{}, len(games))
	for i, g := range games {
		gameList[i] = map[string]interface{}{
			"id":              g.ID,
			"name":            g.Name,
			"type":            g.Type,
			"theoretical_rtp": g.TheoreticalRTP,
			"min_bet":         g.MinBet.Float64(),
			"max_bet":         g.MaxBet.Float64(),
			"enabled":         g.Enabled,
		}
	}

	respondJSON(w, http.StatusOK, gameList)
}

// GetGame handles GET /api/v1/games/{id}
func (h *Handler) GetGame(w http.ResponseWriter, r *http.Request) {
	gameID := mux.Vars(r)["id"]

	g, err := h.game.GetGame(gameID)
	if err != nil {
		respondError(w, http.StatusNotFound, "GAME_NOT_FOUND", "Game not found")
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"id":              g.ID,
		"name":            g.Name,
		"type":            g.Type,
		"theoretical_rtp": g.TheoreticalRTP,
		"min_bet":         g.MinBet.Float64(),
		"max_bet":         g.MaxBet.Float64(),
		"enabled":         g.Enabled,
	})
}

// StartGameSession handles POST /api/v1/games/{id}/session
func (h *Handler) StartGameSession(w http.ResponseWriter, r *http.Request) {
	player := r.Context().Value("player").(*domain.Player)
	gameID := mux.Vars(r)["id"]

	session, err := h.game.StartSession(r.Context(), player.ID, gameID)
	if err != nil {
		switch err {
		case game.ErrGameNotFound:
			respondError(w, http.StatusNotFound, "GAME_NOT_FOUND", "Game not found")
		case game.ErrGameDisabled:
			respondError(w, http.StatusBadRequest, "GAME_DISABLED", "Game is currently disabled")
		default:
			respondError(w, http.StatusInternalServerError, "SESSION_ERROR", err.Error())
		}
		return
	}

	respondJSON(w, http.StatusCreated, map[string]interface{}{
		"session_id":      session.ID,
		"game_id":         session.GameID,
		"opening_balance": session.OpeningBalance.Float64(),
		"started_at":      session.StartedAt,
	})
}

// EndGameSession handles DELETE /api/v1/games/{id}/session
func (h *Handler) EndGameSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
		return
	}

	session, err := h.game.EndSession(r.Context(), req.SessionID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "SESSION_ERROR", err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"session_id":    session.ID,
		"games_played":  session.GamesPlayed,
		"total_wagered": session.TotalWagered.Float64(),
		"total_won":     session.TotalWon.Float64(),
		"ended_at":      session.EndedAt,
	})
}

// Play handles POST /api/v1/games/play
func (h *Handler) Play(w http.ResponseWriter, r *http.Request) {
	var req game.PlayRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
		return
	}

	result, err := h.game.Play(r.Context(), &req)
	if err != nil {
		switch err {
		case game.ErrSessionNotFound:
			respondError(w, http.StatusNotFound, "SESSION_NOT_FOUND", "Game session not found")
		case game.ErrSessionNotActive:
			respondError(w, http.StatusBadRequest, "SESSION_NOT_ACTIVE", "Game session is not active")
		case game.ErrInvalidWager:
			respondError(w, http.StatusBadRequest, "INVALID_WAGER", "Wager amount is invalid")
		case game.ErrInsufficientBalance:
			respondError(w, http.StatusBadRequest, "INSUFFICIENT_BALANCE", "Insufficient balance")
		default:
			respondError(w, http.StatusInternalServerError, "GAME_ERROR", err.Error())
		}
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"cycle_id":     result.CycleID,
		"outcome":      result.Outcome,
		"wager_amount": result.WagerAmount.Float64(),
		"win_amount":   result.WinAmount.Float64(),
		"balance":      result.Balance.Float64(),
	})
}

// GetGameHistory handles GET /api/v1/games/history
func (h *Handler) GetGameHistory(w http.ResponseWriter, r *http.Request) {
	player := r.Context().Value("player").(*domain.Player)

	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 50 {
			limit = n
		}
	}

	history, err := h.game.GetHistory(r.Context(), player.ID, limit)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "HISTORY_ERROR", "Failed to get game history")
		return
	}

	historyList := make([]map[string]interface{}, len(history))
	for i, h := range history {
		historyList[i] = map[string]interface{}{
			"cycle_id":       h.CycleID,
			"game_id":        h.GameID,
			"played_at":      h.PlayedAt,
			"wager_amount":   h.WagerAmount.Float64(),
			"win_amount":     h.WinAmount.Float64(),
			"balance_before": h.BalanceBefore.Float64(),
			"balance_after":  h.BalanceAfter.Float64(),
			"outcome":        h.Outcome,
		}
	}

	respondJSON(w, http.StatusOK, historyList)
}

