// RGS - Remote Gaming Server
// GLI-19 Compliant Implementation
//
// This is the main entry point for the Remote Gaming Server.
// It initializes all services and starts the HTTP server.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alexbotov/rgs/internal/api"
	"github.com/alexbotov/rgs/internal/audit"
	"github.com/alexbotov/rgs/internal/auth"
	"github.com/alexbotov/rgs/internal/config"
	"github.com/alexbotov/rgs/internal/database"
	"github.com/alexbotov/rgs/internal/game"
	"github.com/alexbotov/rgs/internal/rng"
	"github.com/alexbotov/rgs/internal/wallet"
)

func main() {
	// Print banner
	printBanner()

	// Load configuration
	cfg := config.Load()
	log.Printf("Configuration loaded (port: %s, db: %s)", cfg.Server.Port, cfg.Database.DSN)

	// Initialize database
	db, err := database.New(cfg.Database.Driver, cfg.Database.DSN)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()
	log.Println("✓ Database connected")

	// Run migrations
	if err := db.Migrate(); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}
	log.Println("✓ Database migrations complete")

	// Initialize services
	auditSvc := audit.New(db.DB)
	log.Println("✓ Audit service initialized")

	rngSvc := rng.New()
	// Perform initial RNG health check (GLI-19 §3.3.3)
	rngHealth, err := rngSvc.HealthCheck()
	if err != nil || !rngHealth.Healthy {
		log.Fatalf("RNG health check failed: %v", err)
	}
	log.Printf("✓ RNG service initialized (Chi-Square: %.2f, Passed: %v)", rngHealth.ChiSquare, rngHealth.ChiSquarePassed)

	authSvc := auth.New(db.DB, &cfg.Auth, auditSvc)
	log.Println("✓ Auth service initialized")

	walletSvc := wallet.New(db.DB, auditSvc, cfg.Game.DefaultCurrency)
	log.Println("✓ Wallet service initialized")

	gameEngine := game.New(db.DB, rngSvc, walletSvc, auditSvc, cfg.Game.DefaultCurrency)
	log.Printf("✓ Game engine initialized (%d games available)", len(gameEngine.GetGames()))

	// Initialize API handlers
	handler := api.New(authSvc, walletSvc, gameEngine, rngSvc)
	router := handler.SetupRouter()
	log.Println("✓ API routes configured")

	// Create HTTP server
	server := &http.Server{
		Addr:         ":" + cfg.Server.Port,
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	// Start server in goroutine
	go func() {
		log.Printf("🎰 RGS Server starting on http://localhost:%s", cfg.Server.Port)
		log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		printEndpoints(cfg.Server.Port)
		log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Log startup event
	auditSvc.Log(context.Background(), "system_startup", "info",
		"RGS server started",
		map[string]interface{}{
			"port":    cfg.Server.Port,
			"version": "1.0.0",
		},
		audit.WithComponent("main"))

	// Wait for interrupt signal for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("\nShutdown signal received...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	// Log shutdown event
	auditSvc.Log(context.Background(), "system_shutdown", "info",
		"RGS server stopped",
		nil,
		audit.WithComponent("main"))

	log.Println("Server stopped gracefully")
}

func printBanner() {
	banner := `
╔═══════════════════════════════════════════════════════════════╗
║                                                               ║
║   ██████╗  ██████╗ ███████╗    Remote Gaming Server           ║
║   ██╔══██╗██╔════╝ ██╔════╝    GLI-19 Compliant v1.0.0        ║
║   ██████╔╝██║  ███╗███████╗                                   ║
║   ██╔══██╗██║   ██║╚════██║    Interactive Gaming Platform    ║
║   ██║  ██║╚██████╔╝███████║                                   ║
║   ╚═╝  ╚═╝ ╚═════╝ ╚══════╝                                   ║
║                                                               ║
╚═══════════════════════════════════════════════════════════════╝
`
	fmt.Println(banner)
}

func printEndpoints(port string) {
	log.Println("Available Endpoints:")
	log.Println("")
	log.Println("  Public:")
	log.Printf("    GET  http://localhost:%s/              Server info", port)
	log.Printf("    GET  http://localhost:%s/health        Health check", port)
	log.Println("")
	log.Println("  Authentication:")
	log.Printf("    POST http://localhost:%s/api/v1/auth/register   Register", port)
	log.Printf("    POST http://localhost:%s/api/v1/auth/login      Login", port)
	log.Printf("    POST http://localhost:%s/api/v1/auth/logout     Logout", port)
	log.Printf("    GET  http://localhost:%s/api/v1/auth/session    Session info", port)
	log.Println("")
	log.Println("  Wallet:")
	log.Printf("    GET  http://localhost:%s/api/v1/wallet/balance      Get balance", port)
	log.Printf("    POST http://localhost:%s/api/v1/wallet/deposit      Deposit funds", port)
	log.Printf("    POST http://localhost:%s/api/v1/wallet/withdraw     Withdraw funds", port)
	log.Printf("    GET  http://localhost:%s/api/v1/wallet/transactions Transaction history", port)
	log.Println("")
	log.Println("  Games:")
	log.Printf("    GET  http://localhost:%s/api/v1/games               List games", port)
	log.Printf("    GET  http://localhost:%s/api/v1/games/{id}          Game details", port)
	log.Printf("    POST http://localhost:%s/api/v1/games/{id}/session  Start session", port)
	log.Printf("    POST http://localhost:%s/api/v1/games/play          Play game", port)
	log.Printf("    GET  http://localhost:%s/api/v1/games/history       Game history", port)
	log.Println("")
	log.Println("  WebSocket:")
	log.Printf("    WS   ws://localhost:%s/api/v1/ws/game/{session_id}  Real-time game", port)
}
