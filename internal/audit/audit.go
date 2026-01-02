// Package audit provides audit logging for the RGS
// Compliant with GLI-19 §2.8.8: Significant Event Information
package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/alexbotov/rgs/internal/domain"
	"github.com/google/uuid"
)

// Event types per GLI-19 §2.8.8
const (
	EventPlayerRegistered    = "player_registered"
	EventPlayerLogin         = "player_login"
	EventPlayerLogout        = "player_logout"
	EventLoginFailed         = "login_failed"
	EventSessionExpired      = "session_expired"
	EventDeposit             = "deposit"
	EventWithdrawal          = "withdrawal"
	EventGameSessionStart    = "game_session_start"
	EventGameSessionEnd      = "game_session_end"
	EventGameCycleComplete   = "game_cycle_complete"
	EventLargeWin            = "large_win"
	EventLargeWager          = "large_wager"
	EventBalanceAdjustment   = "balance_adjustment"
	EventAccountStatusChange = "account_status_change"
	EventSystemError         = "system_error"
	EventRNGHealthCheck      = "rng_health_check"
)

// Service provides audit logging functionality
type Service struct {
	db *sql.DB
}

// New creates a new audit service
func New(db *sql.DB) *Service {
	return &Service{db: db}
}

// LogEvent records a significant event
func (s *Service) LogEvent(ctx context.Context, event *domain.AuditEvent) error {
	if event.ID == "" {
		event.ID = uuid.New().String()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}

	dataJSON, _ := json.Marshal(event.Data)

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO audit_events (id, type, severity, timestamp, player_id, session_id, description, data, ip_address, component)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, event.ID, event.Type, event.Severity, event.Timestamp, event.PlayerID, event.SessionID,
		event.Description, string(dataJSON), event.IPAddress, event.Component)

	return err
}

// Log is a convenience method for logging events
func (s *Service) Log(ctx context.Context, eventType string, severity domain.EventSeverity, description string, data interface{}, opts ...EventOption) error {
	event := &domain.AuditEvent{
		ID:          uuid.New().String(),
		Type:        eventType,
		Severity:    severity,
		Timestamp:   time.Now().UTC(),
		Description: description,
		Component:   "rgs",
	}

	if data != nil {
		jsonData, err := json.Marshal(data)
		if err == nil {
			event.Data = jsonData
		}
	}

	for _, opt := range opts {
		opt(event)
	}

	return s.LogEvent(ctx, event)
}

// EventOption is a functional option for configuring audit events
type EventOption func(*domain.AuditEvent)

// WithPlayer sets the player ID for the event
func WithPlayer(playerID string) EventOption {
	return func(e *domain.AuditEvent) {
		e.PlayerID = &playerID
	}
}

// WithSession sets the session ID for the event
func WithSession(sessionID string) EventOption {
	return func(e *domain.AuditEvent) {
		e.SessionID = &sessionID
	}
}

// WithIP sets the IP address for the event
func WithIP(ip string) EventOption {
	return func(e *domain.AuditEvent) {
		e.IPAddress = ip
	}
}

// WithComponent sets the component for the event
func WithComponent(component string) EventOption {
	return func(e *domain.AuditEvent) {
		e.Component = component
	}
}

// GetEvents retrieves audit events with optional filtering
func (s *Service) GetEvents(ctx context.Context, filter *EventFilter) ([]*domain.AuditEvent, error) {
	query := `SELECT id, type, severity, timestamp, player_id, session_id, description, data, ip_address, component 
			  FROM audit_events WHERE 1=1`
	args := []interface{}{}
	paramIdx := 1

	if filter != nil {
		if filter.PlayerID != "" {
			query += fmt.Sprintf(" AND player_id = $%d", paramIdx)
			args = append(args, filter.PlayerID)
			paramIdx++
		}
		if filter.Type != "" {
			query += fmt.Sprintf(" AND type = $%d", paramIdx)
			args = append(args, filter.Type)
			paramIdx++
		}
		if !filter.From.IsZero() {
			query += fmt.Sprintf(" AND timestamp >= $%d", paramIdx)
			args = append(args, filter.From)
			paramIdx++
		}
		if !filter.To.IsZero() {
			query += fmt.Sprintf(" AND timestamp <= $%d", paramIdx)
			args = append(args, filter.To)
			paramIdx++
		}
	}

	query += " ORDER BY timestamp DESC"

	if filter != nil && filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", paramIdx)
		args = append(args, filter.Limit)
	} else {
		query += " LIMIT 100"
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*domain.AuditEvent
	for rows.Next() {
		var event domain.AuditEvent
		var playerID, sessionID sql.NullString
		var data string

		err := rows.Scan(&event.ID, &event.Type, &event.Severity, &event.Timestamp,
			&playerID, &sessionID, &event.Description, &data, &event.IPAddress, &event.Component)
		if err != nil {
			return nil, err
		}

		if playerID.Valid {
			event.PlayerID = &playerID.String
		}
		if sessionID.Valid {
			event.SessionID = &sessionID.String
		}
		if data != "" {
			event.Data = json.RawMessage(data)
		}

		events = append(events, &event)
	}

	return events, nil
}

// EventFilter defines criteria for filtering audit events
type EventFilter struct {
	PlayerID string
	Type     string
	From     time.Time
	To       time.Time
	Limit    int
}
