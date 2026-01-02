// Package api - Router setup
package api

import (
	"net/http"

	"github.com/gorilla/mux"
)

// SetupRouter creates and configures the HTTP router
func (h *Handler) SetupRouter() *mux.Router {
	r := mux.NewRouter()

	// Apply global middleware
	r.Use(RecoveryMiddleware)
	r.Use(CORSMiddleware)
	r.Use(LoggingMiddleware)

	// Public routes
	r.HandleFunc("/", h.ServerInfo).Methods("GET")
	r.HandleFunc("/health", h.HealthCheck).Methods("GET")

	// API v1 routes
	api := r.PathPrefix("/api/v1").Subrouter()

	// Auth routes (public)
	auth := api.PathPrefix("/auth").Subrouter()
	auth.HandleFunc("/login", h.Login).Methods("POST")

	// Protected routes
	protected := api.PathPrefix("").Subrouter()
	protected.Use(h.AuthMiddleware)

	// Auth (protected)
	protected.HandleFunc("/auth/logout", h.Logout).Methods("POST")
	protected.HandleFunc("/auth/session", h.GetSession).Methods("GET")

	// Wallet
	protected.HandleFunc("/wallet/balance", h.GetBalance).Methods("GET")
	protected.HandleFunc("/wallet/deposit", h.Deposit).Methods("POST")
	protected.HandleFunc("/wallet/withdraw", h.Withdraw).Methods("POST")
	protected.HandleFunc("/wallet/transactions", h.GetTransactions).Methods("GET")

	// Games
	protected.HandleFunc("/games", h.GetGames).Methods("GET")
	protected.HandleFunc("/games/history", h.GetGameHistory).Methods("GET")
	protected.HandleFunc("/games/play", h.Play).Methods("POST")
	protected.HandleFunc("/games/{id}", h.GetGame).Methods("GET")
	protected.HandleFunc("/games/{id}/session", h.StartGameSession).Methods("POST")
	protected.HandleFunc("/games/{id}/session", h.EndGameSession).Methods("DELETE")

	// WebSocket for real-time games
	protected.HandleFunc("/ws/game/{session_id}", h.HandleWebSocket).Methods("GET")

	return r
}

// NotFoundHandler handles 404 errors
func NotFoundHandler(w http.ResponseWriter, r *http.Request) {
	respondError(w, http.StatusNotFound, "NOT_FOUND", "Resource not found")
}

