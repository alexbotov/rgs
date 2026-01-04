// Package control provides gaming system control functionality
// Compliant with GLI-19 §2.4: Gaming Management
//
// Key Requirements:
//   - Operator must be able to disable all gaming on demand
//   - Individual games can be disabled
//   - Player accounts can be disabled
//   - All state changes must be logged
package control

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/alexbotov/rgs/internal/audit"
	"github.com/alexbotov/rgs/internal/domain"
)

var (
	ErrGamingDisabled = errors.New("gaming is currently disabled")
	ErrGameDisabled   = errors.New("game is currently disabled")
	ErrPlayerDisabled = errors.New("player account is disabled")
)

// Service provides gaming system control functionality
// GLI-19 §2.4 - Gaming Management: System must support disabling gaming operations
type Service struct {
	db    *sql.DB
	audit *audit.Service

	mu            sync.RWMutex
	gamingEnabled bool
	disabledGames map[string]bool
	disabledAt    *time.Time
	disabledBy    string
	disabledReason string
}

// New creates a new control service
func New(db *sql.DB, auditSvc *audit.Service) *Service {
	return &Service{
		db:            db,
		audit:         auditSvc,
		gamingEnabled: true,
		disabledGames: make(map[string]bool),
	}
}

// DisableAllGaming stops all gaming activity
// GLI-19 §2.4.1 - Gaming Management: Ability to disable on demand
func (s *Service) DisableAllGaming(ctx context.Context, reason, authorizedBy string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	s.gamingEnabled = false
	s.disabledAt = &now
	s.disabledBy = authorizedBy
	s.disabledReason = reason

	// Log to database for persistence
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO system_state (key, value, updated_at, updated_by)
		VALUES ('gaming_enabled', 'false', $1, $2)
		ON CONFLICT (key) DO UPDATE SET value = 'false', updated_at = $1, updated_by = $2
	`, now, authorizedBy)
	if err != nil {
		return fmt.Errorf("failed to persist gaming state: %w", err)
	}

	// Audit log - GLI-19 §2.8.8 significant event
	s.audit.Log(ctx, "gaming_disabled", domain.SeverityCritical,
		fmt.Sprintf("All gaming disabled: %s", reason),
		map[string]interface{}{
			"authorized_by": authorizedBy,
			"reason":        reason,
		},
		audit.WithComponent("control"))

	return nil
}

// EnableAllGaming resumes gaming operations
// GLI-19 §2.4.1 - Gaming Management
func (s *Service) EnableAllGaming(ctx context.Context, authorizedBy string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	s.gamingEnabled = true
	s.disabledAt = nil
	s.disabledBy = ""
	s.disabledReason = ""

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO system_state (key, value, updated_at, updated_by)
		VALUES ('gaming_enabled', 'true', $1, $2)
		ON CONFLICT (key) DO UPDATE SET value = 'true', updated_at = $1, updated_by = $2
	`, now, authorizedBy)
	if err != nil {
		return fmt.Errorf("failed to persist gaming state: %w", err)
	}

	// Audit log
	s.audit.Log(ctx, "gaming_enabled", domain.SeverityInfo,
		"All gaming enabled",
		map[string]interface{}{"authorized_by": authorizedBy},
		audit.WithComponent("control"))

	return nil
}

// DisableGame disables a specific game
// GLI-19 §2.4 - Gaming Management
func (s *Service) DisableGame(ctx context.Context, gameID, reason, authorizedBy string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.disabledGames[gameID] = true

	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO disabled_games (game_id, reason, disabled_at, disabled_by)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (game_id) DO UPDATE SET reason = $2, disabled_at = $3, disabled_by = $4
	`, gameID, reason, now, authorizedBy)
	if err != nil {
		return fmt.Errorf("failed to persist game state: %w", err)
	}

	s.audit.Log(ctx, "game_disabled", domain.SeverityWarning,
		fmt.Sprintf("Game disabled: %s - %s", gameID, reason),
		map[string]interface{}{
			"game_id":       gameID,
			"reason":        reason,
			"authorized_by": authorizedBy,
		},
		audit.WithComponent("control"))

	return nil
}

// EnableGame enables a specific game
// GLI-19 §2.4 - Gaming Management
func (s *Service) EnableGame(ctx context.Context, gameID, authorizedBy string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.disabledGames, gameID)

	_, err := s.db.ExecContext(ctx, `DELETE FROM disabled_games WHERE game_id = $1`, gameID)
	if err != nil {
		return fmt.Errorf("failed to persist game state: %w", err)
	}

	s.audit.Log(ctx, "game_enabled", domain.SeverityInfo,
		fmt.Sprintf("Game enabled: %s", gameID),
		map[string]interface{}{
			"game_id":       gameID,
			"authorized_by": authorizedBy,
		},
		audit.WithComponent("control"))

	return nil
}

// DisablePlayer disables a player's account
// GLI-19 §2.4 - Gaming Management: Ability to disable player accounts
func (s *Service) DisablePlayer(ctx context.Context, playerID, reason, authorizedBy string) error {
	now := time.Now().UTC()

	_, err := s.db.ExecContext(ctx, `
		UPDATE players SET status = $1, updated_at = $2 WHERE id = $3
	`, domain.PlayerStatusSuspended, now, playerID)
	if err != nil {
		return fmt.Errorf("failed to disable player: %w", err)
	}

	// Terminate active sessions
	_, err = s.db.ExecContext(ctx, `
		UPDATE sessions SET status = $1 WHERE player_id = $2 AND status = $3
	`, domain.SessionStatusExpired, playerID, domain.SessionStatusActive)
	if err != nil {
		return fmt.Errorf("failed to terminate sessions: %w", err)
	}

	// Audit log - GLI-19 §2.8.8
	s.audit.Log(ctx, audit.EventAccountStatusChange, domain.SeverityWarning,
		fmt.Sprintf("Player account disabled: %s", reason),
		map[string]interface{}{
			"player_id":     playerID,
			"reason":        reason,
			"authorized_by": authorizedBy,
		},
		audit.WithPlayer(playerID), audit.WithComponent("control"))

	return nil
}

// EnablePlayer enables a player's account
// GLI-19 §2.4 - Gaming Management
func (s *Service) EnablePlayer(ctx context.Context, playerID, authorizedBy string) error {
	now := time.Now().UTC()

	// Check for active self-exclusion first
	var exclusionCount int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM self_exclusions 
		WHERE player_id = $1 AND is_active = true 
		AND (expires_at IS NULL OR expires_at > $2)
	`, playerID, now).Scan(&exclusionCount)
	if err != nil {
		return err
	}
	if exclusionCount > 0 {
		return errors.New("cannot enable player with active self-exclusion")
	}

	_, err = s.db.ExecContext(ctx, `
		UPDATE players SET status = $1, updated_at = $2 WHERE id = $3
	`, domain.PlayerStatusActive, now, playerID)
	if err != nil {
		return fmt.Errorf("failed to enable player: %w", err)
	}

	s.audit.Log(ctx, audit.EventAccountStatusChange, domain.SeverityInfo,
		"Player account enabled",
		map[string]interface{}{
			"player_id":     playerID,
			"authorized_by": authorizedBy,
		},
		audit.WithPlayer(playerID), audit.WithComponent("control"))

	return nil
}

// IsGamingEnabled checks if gaming is currently enabled
// GLI-19 §2.4 - Must be able to check system state
func (s *Service) IsGamingEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.gamingEnabled
}

// IsGameEnabled checks if a specific game is enabled
func (s *Service) IsGameEnabled(gameID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return !s.disabledGames[gameID]
}

// GetSystemStatus returns current gaming system status
// GLI-19 §2.4 - System status must be available
func (s *Service) GetSystemStatus(ctx context.Context) (*domain.GamingSystemStatus, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Count active sessions
	var activeSessions int64
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM sessions WHERE status = $1
	`, domain.SessionStatusActive).Scan(&activeSessions)
	if err != nil {
		return nil, err
	}

	status := &domain.GamingSystemStatus{
		GamingEnabled:   s.gamingEnabled,
		DisabledAt:      s.disabledAt,
		DisabledBy:      s.disabledBy,
		DisabledReason:  s.disabledReason,
		ActiveSessions:  activeSessions,
		LastStateChange: time.Now().UTC(),
	}

	return status, nil
}

// CheckAccess verifies a player can access gaming
// GLI-19 §2.4, §2.5.5 - Combined check for gaming access
func (s *Service) CheckAccess(ctx context.Context, playerID, gameID string) error {
	// Check system-wide gaming status
	if !s.IsGamingEnabled() {
		return ErrGamingDisabled
	}

	// Check game-specific status
	if !s.IsGameEnabled(gameID) {
		return ErrGameDisabled
	}

	// Check player status
	var status domain.PlayerStatus
	err := s.db.QueryRowContext(ctx, `SELECT status FROM players WHERE id = $1`, playerID).Scan(&status)
	if err != nil {
		return err
	}

	if status != domain.PlayerStatusActive {
		return ErrPlayerDisabled
	}

	return nil
}

// LoadState loads persisted state from database on startup
func (s *Service) LoadState(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Load gaming enabled state
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM system_state WHERE key = 'gaming_enabled'`).Scan(&value)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	s.gamingEnabled = value != "false"

	// Load disabled games
	rows, err := s.db.QueryContext(ctx, `SELECT game_id FROM disabled_games`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var gameID string
		if err := rows.Scan(&gameID); err != nil {
			return err
		}
		s.disabledGames[gameID] = true
	}

	return nil
}

