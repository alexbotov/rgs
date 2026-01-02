// Package api - Middleware for authentication and request processing
package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/alexbotov/rgs/internal/auth"
)

// AuthMiddleware validates JWT tokens and adds session/player to context
func (h *Handler) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract token from Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			respondError(w, http.StatusUnauthorized, "NO_TOKEN", "Authorization header required")
			return
		}

		// Expect "Bearer <token>"
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			respondError(w, http.StatusUnauthorized, "INVALID_TOKEN_FORMAT", "Invalid authorization header format")
			return
		}

		token := parts[1]

		// Validate token
		session, player, err := h.auth.ValidateToken(r.Context(), token)
		if err != nil {
			switch err {
			case auth.ErrSessionExpired:
				respondError(w, http.StatusUnauthorized, "SESSION_EXPIRED", "Session has expired")
			case auth.ErrSessionNotFound:
				respondError(w, http.StatusUnauthorized, "SESSION_NOT_FOUND", "Session not found")
			default:
				respondError(w, http.StatusUnauthorized, "INVALID_TOKEN", "Invalid token")
			}
			return
		}

		// Add session and player to context
		ctx := context.WithValue(r.Context(), "session", session)
		ctx = context.WithValue(ctx, "player", player)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// LoggingMiddleware logs all requests
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simple request logging
		// In production, use structured logging
		next.ServeHTTP(w, r)
	})
}

// CORSMiddleware adds CORS headers
func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// RecoveryMiddleware recovers from panics
func RecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

