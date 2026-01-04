// Package auth provides authentication and session management
// Compliant with GLI-19 §2.5: Player Account Management
package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/alexbotov/rgs/internal/audit"
	"github.com/alexbotov/rgs/internal/config"
	"github.com/alexbotov/rgs/internal/domain"
	"github.com/alexbotov/rgs/pkg/pateplay"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrAccountLocked      = errors.New("account temporarily locked")
	ErrAccountNotActive   = errors.New("account is not active")
	ErrSessionExpired     = errors.New("session expired")
	ErrSessionNotFound    = errors.New("session not found")
	ErrUserExists         = errors.New("username or email already exists")
)

// Service provides authentication functionality
type Service struct {
	db       *sql.DB
	config   *config.AuthConfig
	audit    *audit.Service
	pateplay *pateplay.Client
}

// New creates a new auth service
func New(db *sql.DB, cfg *config.AuthConfig, auditSvc *audit.Service, pateplayClient *pateplay.Client) *Service {
	return &Service{
		db:       db,
		config:   cfg,
		audit:    auditSvc,
		pateplay: pateplayClient,
	}
}

// RegisterRequest contains registration data
type RegisterRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
	AcceptTC bool   `json:"accept_tc"`
}

// Register creates a new player account (GLI-19 §2.5.2)
func (s *Service) Register(ctx context.Context, req *RegisterRequest, ip string) (*domain.Player, error) {
	// Validate input
	if req.Username == "" || req.Email == "" || req.Password == "" {
		return nil, errors.New("username, email, and password are required")
	}
	if !req.AcceptTC {
		return nil, errors.New("terms and conditions must be accepted")
	}
	if len(req.Password) < 8 {
		return nil, errors.New("password must be at least 8 characters")
	}

	// Check if user exists
	var exists int
	err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM players WHERE username = $1 OR email = $2",
		req.Username, req.Email).Scan(&exists)
	if err != nil {
		return nil, fmt.Errorf("database error: %w", err)
	}
	if exists > 0 {
		return nil, ErrUserExists
	}

	// Hash password using bcrypt (GLI-19 §B.2.3)
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	now := time.Now().UTC()
	player := &domain.Player{
		ID:               uuid.New().String(),
		Username:         req.Username,
		Email:            req.Email,
		PasswordHash:     string(hash),
		Status:           domain.PlayerStatusActive,
		RegistrationDate: now,
		TCAcceptedAt:     now,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	// Insert player
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO players (id, username, email, password_hash, status, registration_date, tc_accepted_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, player.ID, player.Username, player.Email, player.PasswordHash, player.Status,
		player.RegistrationDate, player.TCAcceptedAt, player.CreatedAt, player.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create player: %w", err)
	}

	// Create initial balance (GLI-19 §2.5.7)
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO balances (player_id, real_money_amount, real_money_currency, bonus_amount, bonus_currency, updated_at)
		VALUES ($1, 0, 'USD', 0, 'USD', $2)
	`, player.ID, now)
	if err != nil {
		return nil, fmt.Errorf("failed to create balance: %w", err)
	}

	// Audit log
	s.audit.Log(ctx, audit.EventPlayerRegistered, domain.SeverityInfo,
		fmt.Sprintf("Player registered: %s", player.Username),
		map[string]string{"player_id": player.ID},
		audit.WithPlayer(player.ID), audit.WithIP(ip))

	return player, nil
}

// LoginRequest contains login credentials
type LoginRequest struct {
	AuthToken  string `json:"auth_token"`
	DeviceType string `json:"device_type"`
}

// LoginResponse contains login result
type LoginResponse struct {
	Player  *domain.Player  `json:"player"`
	Session *domain.Session `json:"session"`
	Token   string          `json:"token"`
}

// Login authenticates a player (GLI-19 §2.5.3)
func (s *Service) Login(ctx context.Context, req *LoginRequest, ip, userAgent string) (*LoginResponse, error) {
	authResult, err := s.pateplay.Authenticate(ctx, req.AuthToken, pateplay.DeviceTypeDesktop)
	if err != nil {
		// authResult may be nil on error, so we can't access its fields
		s.audit.Log(ctx, audit.EventLoginFailed, domain.SeverityWarning,
			fmt.Sprintf("Pateplay authentication failed: %v", err),
			map[string]string{"error": err.Error()},
			audit.WithIP(ip))
		return nil, ErrInvalidCredentials
	}

	// Get player
	var player domain.Player
	err = s.db.QueryRowContext(ctx, `
		SELECT id, username, email, password_hash, status, registration_date, last_login_at, tc_accepted_at, created_at, updated_at
		FROM players WHERE id = $1
	`, authResult.PlayerID).Scan(
		&player.ID, &player.Username, &player.Email, &player.PasswordHash,
		&player.Status, &player.RegistrationDate, &player.LastLoginAt,
		&player.TCAcceptedAt, &player.CreatedAt, &player.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			s.Register(ctx, &RegisterRequest{
				Username: authResult.PlayerName,
				Email:    authResult.PlayerName,
				Password: authResult.PlayerName,
				AcceptTC: true,
			}, ip)
		} else {
			return nil, fmt.Errorf("database error: %w", err)
		}
	}

	// Check for lockout (GLI-19 §2.5.3.d)
	if s.isLockedOut(ctx, player.ID, player.LastLoginAt) {
		return nil, ErrAccountLocked
	}

	// Check account status
	if player.Status != domain.PlayerStatusActive {
		return nil, ErrAccountNotActive
	}

	// Create session
	session, token, err := s.createSession(ctx, &player, ip, userAgent)
	if err != nil {
		return nil, err
	}

	// Update last login
	now := time.Now().UTC()
	s.db.ExecContext(ctx, "UPDATE players SET last_login_at = $1, updated_at = $2 WHERE id = $3",
		now, now, player.ID)
	player.LastLoginAt = &now

	// Clear failed login attempts
	s.db.ExecContext(ctx, "DELETE FROM failed_logins WHERE player_id = $1", player.ID)

	// Audit log
	s.audit.Log(ctx, audit.EventPlayerLogin, domain.SeverityInfo,
		fmt.Sprintf("Player logged in: %s", player.Username),
		map[string]string{"session_id": session.ID},
		audit.WithPlayer(player.ID), audit.WithSession(session.ID), audit.WithIP(ip))

	return &LoginResponse{
		Player:  &player,
		Session: session,
		Token:   token,
	}, nil
}

// createSession creates a new session with JWT token
func (s *Service) createSession(ctx context.Context, player *domain.Player, ip, userAgent string) (*domain.Session, string, error) {
	now := time.Now().UTC()
	session := &domain.Session{
		ID:             uuid.New().String(),
		PlayerID:       player.ID,
		IPAddress:      ip,
		UserAgent:      userAgent,
		CreatedAt:      now,
		LastActivityAt: now,
		ExpiresAt:      now.Add(s.config.TokenExpiry),
		Status:         domain.SessionStatusActive,
	}

	// Generate JWT token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"session_id": session.ID,
		"player_id":  player.ID,
		"username":   player.Username,
		"exp":        session.ExpiresAt.Unix(),
		"iat":        now.Unix(),
	})

	tokenString, err := token.SignedString([]byte(s.config.JWTSecret))
	if err != nil {
		return nil, "", fmt.Errorf("failed to sign token: %w", err)
	}

	session.Token = tokenString

	// Store session
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO sessions (id, player_id, token, ip_address, user_agent, created_at, last_activity_at, expires_at, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, session.ID, session.PlayerID, session.Token, session.IPAddress, session.UserAgent,
		session.CreatedAt, session.LastActivityAt, session.ExpiresAt, session.Status)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create session: %w", err)
	}

	return session, tokenString, nil
}

// ValidateToken validates a JWT token and returns the session
func (s *Service) ValidateToken(ctx context.Context, tokenString string) (*domain.Session, *domain.Player, error) {
	// Parse and validate token
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.config.JWTSecret), nil
	})
	if err != nil {
		return nil, nil, ErrSessionExpired
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, nil, ErrSessionExpired
	}

	sessionID, ok := claims["session_id"].(string)
	if !ok {
		return nil, nil, ErrSessionExpired
	}

	// Get session from database
	var session domain.Session
	err = s.db.QueryRowContext(ctx, `
		SELECT id, player_id, token, ip_address, user_agent, created_at, last_activity_at, expires_at, status
		FROM sessions WHERE id = $1
	`, sessionID).Scan(
		&session.ID, &session.PlayerID, &session.Token, &session.IPAddress, &session.UserAgent,
		&session.CreatedAt, &session.LastActivityAt, &session.ExpiresAt, &session.Status)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, ErrSessionNotFound
		}
		return nil, nil, err
	}

	// Check session status
	if session.Status != domain.SessionStatusActive {
		return nil, nil, ErrSessionExpired
	}

	// Check expiry
	if time.Now().After(session.ExpiresAt) {
		s.db.ExecContext(ctx, "UPDATE sessions SET status = $1 WHERE id = $2",
			domain.SessionStatusExpired, session.ID)
		return nil, nil, ErrSessionExpired
	}

	// Check inactivity timeout (GLI-19 §2.5.4)
	if time.Since(session.LastActivityAt) > s.config.SessionTimeout {
		s.db.ExecContext(ctx, "UPDATE sessions SET status = $1 WHERE id = $2",
			domain.SessionStatusRequiresAuth, session.ID)
		return nil, nil, ErrSessionExpired
	}

	// Get player
	var player domain.Player
	err = s.db.QueryRowContext(ctx, `
		SELECT id, username, email, status, registration_date, last_login_at, tc_accepted_at, created_at, updated_at
		FROM players WHERE id = $1
	`, session.PlayerID).Scan(
		&player.ID, &player.Username, &player.Email, &player.Status,
		&player.RegistrationDate, &player.LastLoginAt, &player.TCAcceptedAt,
		&player.CreatedAt, &player.UpdatedAt)
	if err != nil {
		return nil, nil, err
	}

	// Update last activity
	now := time.Now().UTC()
	s.db.ExecContext(ctx, "UPDATE sessions SET last_activity_at = $1 WHERE id = $2", now, session.ID)
	session.LastActivityAt = now

	return &session, &player, nil
}

// Logout terminates a session
func (s *Service) Logout(ctx context.Context, sessionID string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE sessions SET status = $1 WHERE id = $2",
		domain.SessionStatusLoggedOut, sessionID)
	if err != nil {
		return err
	}

	s.audit.Log(ctx, audit.EventPlayerLogout, domain.SeverityInfo,
		"Player logged out",
		map[string]string{"session_id": sessionID},
		audit.WithSession(sessionID))

	return nil
}

// GetPlayer retrieves a player by ID
func (s *Service) GetPlayer(ctx context.Context, playerID string) (*domain.Player, error) {
	var player domain.Player
	err := s.db.QueryRowContext(ctx, `
		SELECT id, username, email, status, registration_date, last_login_at, tc_accepted_at, created_at, updated_at
		FROM players WHERE id = $1
	`, playerID).Scan(
		&player.ID, &player.Username, &player.Email, &player.Status,
		&player.RegistrationDate, &player.LastLoginAt, &player.TCAcceptedAt,
		&player.CreatedAt, &player.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("player not found")
		}
		return nil, err
	}
	return &player, nil
}

// isLockedOut checks if account is locked due to failed attempts (GLI-19 §2.5.3.d)
func (s *Service) isLockedOut(ctx context.Context, playerID string, lastLoginAt *time.Time) bool {
	if lastLoginAt == nil {
		return false
	}
	cutoff := lastLoginAt.Add(-s.config.LockoutDuration)
	var count int
	s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM failed_logins WHERE player_id = $1 AND attempted_at > $2 AND attempted_at < $3",
		playerID, cutoff, time.Now()).Scan(&count)
	return count >= s.config.MaxFailedAttempts
}

// recordFailedLogin records a failed login attempt
func (s *Service) recordFailedLogin(ctx context.Context, username, ip string) {
	s.db.ExecContext(ctx, `
		INSERT INTO failed_logins (id, username, ip_address, attempted_at)
		VALUES ($1, $2, $3, $4)
	`, uuid.New().String(), username, ip, time.Now().UTC())
}
