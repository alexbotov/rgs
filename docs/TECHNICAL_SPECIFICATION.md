# RGS Technical Specification

## GLI-19 Compliant Remote Gaming Server Implementation

**Version:** 1.0  
**Based on:** GLI-19 Standards for Interactive Gaming Systems V3.0 (July 2020)  
**Status:** Draft

---

## Table of Contents

1. [Overview](#1-overview)
2. [System Architecture](#2-system-architecture)
3. [Platform Requirements](#3-platform-requirements)
4. [Player Account Management](#4-player-account-management)
5. [Random Number Generator (RNG)](#5-random-number-generator-rng)
6. [Game Engine](#6-game-engine)
7. [Financial Transactions](#7-financial-transactions)
8. [Data Management & Logging](#8-data-management--logging)
9. [Security Controls](#9-security-controls)
10. [Communications](#10-communications)
11. [Reporting](#11-reporting)
12. [API Specification](#12-api-specification)
13. [Appendix: Data Models](#appendix-data-models)

---

## 1. Overview

### 1.1 Purpose

This document provides a technical specification for implementing a Remote Gaming Server (RGS) that is compliant with GLI-19 Standards for Interactive Gaming Systems. The RGS serves as the central platform for hosting, managing, and delivering online casino games to players via remote client devices.

### 1.2 Scope

The RGS implementation covers:

- Platform/System core functionality
- Player account management and authentication
- Cryptographically strong Random Number Generator (RNG)
- Game execution and outcome determination
- Financial transaction processing
- Comprehensive audit logging and reporting
- Security controls and data protection
- Real-time communications

### 1.3 Compliance Target

| Standard | Version | Release Date |
|----------|---------|--------------|
| GLI-19 | 3.0 | July 17, 2020 |

### 1.4 Key Principles

1. **Fairness** - All game outcomes are determined by approved RNG, with no adaptive behavior
2. **Security** - Multi-layered defense with encryption, access controls, and audit trails
3. **Transparency** - Complete logging of all gaming and financial transactions
4. **Reliability** - Fault-tolerant design with data recovery capabilities
5. **Accountability** - Full traceability of all system operations

---

## 2. System Architecture

### 2.1 High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              REMOTE PLAYER DEVICES                          │
│         (Web Browser / Mobile App / Desktop Client)                         │
└─────────────────────────────────────┬───────────────────────────────────────┘
                                      │ HTTPS/WSS (TLS 1.2+)
                                      ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                              LOAD BALANCER / API GATEWAY                    │
│                         (Rate Limiting, DDoS Protection)                    │
└─────────────────────────────────────┬───────────────────────────────────────┘
                                      │
          ┌───────────────────────────┼───────────────────────────┐
          ▼                           ▼                           ▼
┌─────────────────────┐   ┌─────────────────────┐   ┌─────────────────────┐
│   AUTH SERVICE      │   │   GAME SERVICE      │   │  WALLET SERVICE     │
│                     │   │                     │   │                     │
│ • Authentication    │   │ • Game Sessions     │   │ • Deposits          │
│ • Session Mgmt      │   │ • RNG Integration   │   │ • Withdrawals       │
│ • MFA               │   │ • Outcome Calc      │   │ • Balance Mgmt      │
│ • Token Validation  │   │ • Game State        │   │ • Transaction Log   │
└─────────┬───────────┘   └─────────┬───────────┘   └─────────┬───────────┘
          │                         │                         │
          └───────────────────────────────────────────────────┘
                                      │
                                      ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                              CORE SERVICES                                  │
├─────────────────────┬─────────────────────┬─────────────────────────────────┤
│   RNG SERVICE       │   JACKPOT SERVICE   │   AUDIT SERVICE                 │
│                     │                     │                                 │
│ • CSPRNG            │ • Progressive       │ • Event Logging                 │
│ • Statistical Test  │ • Incrementing      │ • Transaction Audit             │
│ • Output Monitor    │ • Mystery Trigger   │ • Compliance Reports            │
└─────────────────────┴─────────────────────┴─────────────────────────────────┘
                                      │
                                      ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                              DATA LAYER                                     │
├─────────────────────┬─────────────────────┬─────────────────────────────────┤
│   PRIMARY DB        │   REPLICA DB        │   CACHE (Redis)                 │
│   (PostgreSQL)      │   (Read Replicas)   │                                 │
│                     │                     │   • Session State               │
│ • Player Accounts   │ • Query Offload     │   • Game State                  │
│ • Transactions      │ • Reporting         │   • Rate Limiting               │
│ • Game History      │                     │                                 │
└─────────────────────┴─────────────────────┴─────────────────────────────────┘
```

### 2.2 Component Overview

| Component | Responsibility | GLI-19 Reference |
|-----------|---------------|------------------|
| Auth Service | Player authentication, session management | §2.5 Player Account Management |
| Game Service | Game execution, outcome determination | Chapter 4: Game Requirements |
| Wallet Service | Financial transactions, balance management | §2.5.6 Financial Transactions |
| RNG Service | Random number generation | Chapter 3: RNG Requirements |
| Jackpot Service | Progressive/incrementing jackpots | §4.13 Progressive Jackpots |
| Audit Service | Logging, reporting, compliance | §2.8, §2.9 Information/Reporting |

### 2.3 Technology Stack

| Layer | Technology | Justification |
|-------|------------|---------------|
| Language | Go 1.21+ | Performance, concurrency, type safety |
| Database | PostgreSQL 15+ | ACID compliance, JSON support, reliability |
| Cache | Redis 7+ | Session state, real-time data |
| Message Queue | NATS | Low latency, reliability |
| API | REST + WebSocket | Standard protocols, real-time support |

---

## 3. Platform Requirements

### 3.1 System Clock (GLI-19 §2.2)

**Requirement:** Maintain synchronized internal clock for all time-stamping.

#### Implementation

```go
// ClockService provides synchronized time across all system components
type ClockService interface {
    // Now returns the current synchronized time
    Now() time.Time
    
    // Timestamp generates a unique timestamp for transactions
    Timestamp() Timestamp
    
    // SyncStatus returns NTP synchronization status
    SyncStatus() SyncStatus
}

type Timestamp struct {
    Time       time.Time `json:"time"`
    UnixNano   int64     `json:"unix_nano"`
    Timezone   string    `json:"timezone"`
    NTPSynced  bool      `json:"ntp_synced"`
}
```

#### Requirements

| Requirement | Specification |
|-------------|---------------|
| Time Source | NTP synchronized (stratum 2 or better) |
| Sync Frequency | Every 60 seconds |
| Drift Tolerance | ±100ms maximum |
| Timezone | UTC for all internal operations |
| Display | Convert to player's local timezone for UI |

### 3.2 Control Program Verification (GLI-19 §2.3)

**Requirement:** Self-verification of critical program components.

#### Implementation

```go
// IntegrityService handles program verification
type IntegrityService interface {
    // VerifyAll performs full system integrity check
    VerifyAll(ctx context.Context) (*VerificationResult, error)
    
    // VerifyComponent checks a specific component
    VerifyComponent(ctx context.Context, componentID string) (*ComponentResult, error)
    
    // ScheduleVerification sets up periodic verification
    ScheduleVerification(interval time.Duration) error
    
    // GetLastVerification returns last verification result
    GetLastVerification() *VerificationResult
}

type VerificationResult struct {
    Timestamp     time.Time                   `json:"timestamp"`
    Success       bool                        `json:"success"`
    Components    map[string]ComponentResult  `json:"components"`
    Hash          string                      `json:"hash"`
    Algorithm     string                      `json:"algorithm"`
}

type ComponentResult struct {
    Name          string    `json:"name"`
    ExpectedHash  string    `json:"expected_hash"`
    ActualHash    string    `json:"actual_hash"`
    Valid         bool      `json:"valid"`
    VerifiedAt    time.Time `json:"verified_at"`
}
```

#### Requirements

| Requirement | Specification |
|-------------|---------------|
| Hash Algorithm | SHA-256 minimum (128-bit digest) |
| Verification Frequency | At least every 24 hours + on demand |
| Critical Components | Executables, libraries, configs, game logic |
| Failure Action | Alert operator, disable affected games |

### 3.3 Gaming Management (GLI-19 §2.4)

**Requirement:** Ability to disable gaming operations on demand.

```go
// GamingControlService manages gaming state
type GamingControlService interface {
    // DisableAllGaming stops all gaming activity
    DisableAllGaming(ctx context.Context, reason string) error
    
    // DisableGame disables a specific game
    DisableGame(ctx context.Context, gameID string, reason string) error
    
    // DisablePlayer disables a player's account
    DisablePlayer(ctx context.Context, playerID string, reason string) error
    
    // EnableAllGaming resumes gaming operations
    EnableAllGaming(ctx context.Context, authorizedBy string) error
    
    // GetSystemStatus returns current gaming status
    GetSystemStatus(ctx context.Context) (*SystemStatus, error)
}

type SystemStatus struct {
    GamingEnabled     bool                 `json:"gaming_enabled"`
    DisabledGames     []string             `json:"disabled_games"`
    DisabledPlayers   []string             `json:"disabled_players"`
    ActiveSessions    int64                `json:"active_sessions"`
    LastStateChange   time.Time            `json:"last_state_change"`
    StateChangeReason string               `json:"state_change_reason"`
}
```

---

## 4. Player Account Management

### 4.1 Registration & Verification (GLI-19 §2.5.2)

#### Player Registration Flow

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│  REGISTRATION   │     │  VERIFICATION   │     │  ACTIVATION     │
│                 │     │                 │     │                 │
│ • Collect PII   │────▶│ • Age Check     │────▶│ • Account Live  │
│ • T&C Accept    │     │ • ID Verify     │     │ • Login Enabled │
│ • Privacy Policy│     │ • Exclusion List│     │ • Welcome Email │
└─────────────────┘     └─────────────────┘     └─────────────────┘
```

#### Data Model

```go
type Player struct {
    ID                string          `json:"id" db:"id"`
    Username          string          `json:"username" db:"username"`
    Email             string          `json:"email" db:"email"`
    
    // PII (Encrypted at rest)
    LegalName         EncryptedField  `json:"-" db:"legal_name_enc"`
    DateOfBirth       EncryptedField  `json:"-" db:"dob_enc"`
    ResidentialAddr   EncryptedField  `json:"-" db:"address_enc"`
    GovernmentID      EncryptedField  `json:"-" db:"government_id_enc"`
    
    // Authentication
    PasswordHash      string          `json:"-" db:"password_hash"`
    MFAEnabled        bool            `json:"mfa_enabled" db:"mfa_enabled"`
    MFASecret         EncryptedField  `json:"-" db:"mfa_secret_enc"`
    
    // Verification Status
    AgeVerified       bool            `json:"age_verified" db:"age_verified"`
    IdentityVerified  bool            `json:"identity_verified" db:"identity_verified"`
    VerificationDate  *time.Time      `json:"verification_date" db:"verification_date"`
    VerificationMethod string         `json:"verification_method" db:"verification_method"`
    
    // Account Status
    Status            PlayerStatus    `json:"status" db:"status"`
    RegistrationDate  time.Time       `json:"registration_date" db:"registration_date"`
    LastLoginAt       *time.Time      `json:"last_login_at" db:"last_login_at"`
    
    // Consents
    TCAcceptedAt      time.Time       `json:"tc_accepted_at" db:"tc_accepted_at"`
    TCVersion         string          `json:"tc_version" db:"tc_version"`
    PrivacyAcceptedAt time.Time       `json:"privacy_accepted_at" db:"privacy_accepted_at"`
    PrivacyVersion    string          `json:"privacy_version" db:"privacy_version"`
}

type PlayerStatus string

const (
    PlayerStatusPending     PlayerStatus = "pending"
    PlayerStatusActive      PlayerStatus = "active"
    PlayerStatusSuspended   PlayerStatus = "suspended"
    PlayerStatusExcluded    PlayerStatus = "excluded"
    PlayerStatusClosed      PlayerStatus = "closed"
)
```

#### Registration Requirements

| Field | Required | Validation |
|-------|----------|------------|
| Email | Yes | Valid format, unique |
| Password | Yes | Min 8 chars, complexity rules |
| Legal Name | Yes | Non-empty |
| Date of Birth | Yes | Must be ≥ legal age |
| Residential Address | Yes | Valid format |
| Government ID | Yes | Format validation per jurisdiction |
| T&C Acceptance | Yes | Must be true |
| Privacy Policy | Yes | Must be true |

### 4.2 Authentication (GLI-19 §2.5.3)

#### Session Management

```go
type Session struct {
    ID              string         `json:"id" db:"id"`
    PlayerID        string         `json:"player_id" db:"player_id"`
    Token           string         `json:"-" db:"token_hash"`
    DeviceID        string         `json:"device_id" db:"device_id"`
    IPAddress       string         `json:"ip_address" db:"ip_address"`
    UserAgent       string         `json:"user_agent" db:"user_agent"`
    CreatedAt       time.Time      `json:"created_at" db:"created_at"`
    LastActivityAt  time.Time      `json:"last_activity_at" db:"last_activity_at"`
    ExpiresAt       time.Time      `json:"expires_at" db:"expires_at"`
    MFAVerified     bool           `json:"mfa_verified" db:"mfa_verified"`
    Status          SessionStatus  `json:"status" db:"status"`
}

type AuthService interface {
    // Login authenticates player and creates session
    Login(ctx context.Context, req *LoginRequest) (*Session, error)
    
    // VerifyMFA validates MFA token
    VerifyMFA(ctx context.Context, sessionID string, token string) error
    
    // Logout terminates session
    Logout(ctx context.Context, sessionID string) error
    
    // ValidateSession checks session validity
    ValidateSession(ctx context.Context, token string) (*Session, error)
    
    // RefreshSession extends session lifetime
    RefreshSession(ctx context.Context, sessionID string) (*Session, error)
    
    // ResetPassword initiates password reset with MFA
    ResetPassword(ctx context.Context, req *PasswordResetRequest) error
}
```

#### Authentication Requirements

| Requirement | Specification |
|-------------|---------------|
| Password Storage | bcrypt or Argon2id hash |
| Failed Login Lockout | 3 failures in 30 minutes → lock |
| Session Timeout | 30 minutes of inactivity |
| Re-authentication | Required after timeout |
| MFA for Sensitive Ops | Password reset, payment methods, PII changes |

### 4.3 Player Inactivity (GLI-19 §2.5.4)

```go
type InactivityConfig struct {
    SessionTimeout       time.Duration  // 30 minutes default
    ReauthTimeout        time.Duration  // Simpler auth allowed
    FullReauthInterval   time.Duration  // 30 days - full auth required
    BiometricAllowed     bool           // OS-level auth for reauth
}

func (s *SessionService) CheckInactivity(ctx context.Context, session *Session) error {
    elapsed := time.Since(session.LastActivityAt)
    
    if elapsed > s.config.SessionTimeout {
        session.Status = SessionStatusRequiresReauth
        // No gaming or financial transactions until reauth
        return ErrReauthenticationRequired
    }
    
    return nil
}
```

### 4.4 Limitations & Exclusions (GLI-19 §2.5.5)

```go
type PlayerLimits struct {
    ID              string         `json:"id" db:"id"`
    PlayerID        string         `json:"player_id" db:"player_id"`
    
    // Deposit Limits
    DailyDeposit    *Money         `json:"daily_deposit" db:"daily_deposit"`
    WeeklyDeposit   *Money         `json:"weekly_deposit" db:"weekly_deposit"`
    MonthlyDeposit  *Money         `json:"monthly_deposit" db:"monthly_deposit"`
    
    // Wager Limits
    DailyWager      *Money         `json:"daily_wager" db:"daily_wager"`
    WeeklyWager     *Money         `json:"weekly_wager" db:"weekly_wager"`
    
    // Loss Limits
    DailyLoss       *Money         `json:"daily_loss" db:"daily_loss"`
    WeeklyLoss      *Money         `json:"weekly_loss" db:"weekly_loss"`
    
    // Session Limits
    SessionDuration *time.Duration `json:"session_duration" db:"session_duration"`
    
    // Cooling-off
    CoolingOffUntil *time.Time     `json:"cooling_off_until" db:"cooling_off_until"`
    
    // Source of limit
    Source          LimitSource    `json:"source" db:"source"`
    EffectiveAt     time.Time      `json:"effective_at" db:"effective_at"`
    UpdatedAt       time.Time      `json:"updated_at" db:"updated_at"`
}

type LimitSource string

const (
    LimitSourcePlayer   LimitSource = "player"    // Self-imposed
    LimitSourceOperator LimitSource = "operator"  // Operator-imposed
    LimitSourceRegulator LimitSource = "regulator" // Regulatory requirement
)
```

#### Limit Change Rules

| Change Type | Waiting Period |
|-------------|----------------|
| Decrease limit | Immediate |
| Increase limit | 24 hours minimum |
| Remove limit | 24 hours minimum |
| Self-exclusion | Immediate |
| Exclusion removal | Minimum cooling-off period |

---

## 5. Random Number Generator (RNG)

### 5.1 Requirements (GLI-19 Chapter 3)

The RNG is the most critical component for game fairness. GLI-19 requires:

1. **Cryptographic Strength** - Resistant to attack by intelligent attacker with source code knowledge
2. **Statistical Randomness** - Pass comprehensive statistical tests at 99% confidence
3. **Independence** - Knowledge of past outputs provides no information about future outputs
4. **Uniform Distribution** - All outcomes equally likely (unless game design specifies otherwise)

### 5.2 Implementation

```go
// RNGService provides cryptographically strong random number generation
type RNGService interface {
    // GenerateBytes returns n random bytes
    GenerateBytes(n int) ([]byte, error)
    
    // GenerateInt returns random integer in range [0, max)
    GenerateInt(max int64) (int64, error)
    
    // GenerateIntRange returns random integer in range [min, max]
    GenerateIntRange(min, max int64) (int64, error)
    
    // GenerateFloat returns random float in range [0.0, 1.0)
    GenerateFloat() (float64, error)
    
    // Shuffle performs Fisher-Yates shuffle on slice
    Shuffle(slice interface{}) error
    
    // SelectWeighted selects index based on weighted probabilities
    SelectWeighted(weights []float64) (int, error)
    
    // GetEntropy returns current entropy estimation
    GetEntropy() EntropyStatus
    
    // HealthCheck verifies RNG is functioning correctly
    HealthCheck(ctx context.Context) (*RNGHealthResult, error)
}

type EntropyStatus struct {
    Available     int       `json:"available"`
    Source        string    `json:"source"`
    LastRefresh   time.Time `json:"last_refresh"`
    Healthy       bool      `json:"healthy"`
}

type RNGHealthResult struct {
    Timestamp       time.Time `json:"timestamp"`
    Healthy         bool      `json:"healthy"`
    EntropyStatus   EntropyStatus `json:"entropy"`
    LastTestResult  *StatisticalTestResult `json:"last_test"`
}
```

### 5.3 Cryptographic Requirements

```go
// CryptoRNG implements cryptographically strong RNG
type CryptoRNG struct {
    entropy    io.Reader           // crypto/rand.Reader
    mutex      sync.Mutex
    buffer     []byte
    lastReseed time.Time
}

func NewCryptoRNG() (*CryptoRNG, error) {
    rng := &CryptoRNG{
        entropy: rand.Reader,
        buffer:  make([]byte, 256),
    }
    
    // Initial entropy collection
    if _, err := io.ReadFull(rng.entropy, rng.buffer); err != nil {
        return nil, fmt.Errorf("failed to seed RNG: %w", err)
    }
    
    rng.lastReseed = time.Now()
    return rng, nil
}

// GenerateInt returns unbiased random integer in range [0, max)
func (r *CryptoRNG) GenerateInt(max int64) (int64, error) {
    if max <= 0 {
        return 0, ErrInvalidRange
    }
    
    // Use rejection sampling to eliminate modulo bias
    // Calculate the largest multiple of max that fits in int64
    threshold := (1<<63 - 1) - ((1<<63 - 1) % uint64(max))
    
    var n uint64
    for {
        buf := make([]byte, 8)
        if _, err := io.ReadFull(r.entropy, buf); err != nil {
            return 0, err
        }
        n = binary.BigEndian.Uint64(buf) >> 1 // Use 63 bits for positive
        
        if n < threshold {
            break
        }
        // Reject and retry to avoid bias
    }
    
    return int64(n % uint64(max)), nil
}
```

### 5.4 Attack Resistance (GLI-19 §3.3.2)

| Attack Type | Protection |
|-------------|------------|
| Direct Cryptanalytic | Use of cryptographic primitives (ChaCha20, AES-CTR) |
| Known Input | Never seed from time alone; use hardware entropy |
| State Compromise Extension | Periodic reseeding with external entropy |

### 5.5 Statistical Testing

```go
type StatisticalTester interface {
    // RunAllTests executes full test suite
    RunAllTests(ctx context.Context, samples []int64) (*TestSuiteResult, error)
    
    // ChiSquareTest tests uniform distribution
    ChiSquareTest(samples []int64, bins int) (*TestResult, error)
    
    // RunsTest tests independence of sequences
    RunsTest(samples []int64) (*TestResult, error)
    
    // SerialCorrelationTest tests correlation between values
    SerialCorrelationTest(samples []int64, lag int) (*TestResult, error)
    
    // OverlapsTest tests for overlapping patterns
    OverlapsTest(samples []int64) (*TestResult, error)
}

type TestResult struct {
    Name        string    `json:"name"`
    Passed      bool      `json:"passed"`
    PValue      float64   `json:"p_value"`
    Statistic   float64   `json:"statistic"`
    Threshold   float64   `json:"threshold"`
    SampleSize  int       `json:"sample_size"`
    TestedAt    time.Time `json:"tested_at"`
}
```

#### Test Requirements

| Test | Purpose | Confidence Level |
|------|---------|------------------|
| Chi-Square | Distribution uniformity | 99% |
| Runs Test | Sequence independence | 99% |
| Serial Correlation | Value correlation | 99% |
| Overlaps | Pattern detection | 99% |
| Coupon Collector | Coverage verification | 99% |

---

## 6. Game Engine

### 6.1 Game Session (GLI-19 §4.3)

```go
type GameSession struct {
    ID              string          `json:"id" db:"id"`
    PlayerID        string          `json:"player_id" db:"player_id"`
    GameID          string          `json:"game_id" db:"game_id"`
    GameThemeID     string          `json:"game_theme_id" db:"game_theme_id"`
    
    // Timing
    StartedAt       time.Time       `json:"started_at" db:"started_at"`
    EndedAt         *time.Time      `json:"ended_at" db:"ended_at"`
    LastActivityAt  time.Time       `json:"last_activity_at" db:"last_activity_at"`
    
    // State
    Status          GameSessionStatus `json:"status" db:"status"`
    CurrentCycleID  *string         `json:"current_cycle_id" db:"current_cycle_id"`
    
    // Balance
    OpeningBalance  Money           `json:"opening_balance" db:"opening_balance"`
    CurrentBalance  Money           `json:"current_balance" db:"current_balance"`
    
    // Denomination
    Denomination    Money           `json:"denomination" db:"denomination"`
    Currency        string          `json:"currency" db:"currency"`
    
    // Location
    IPAddress       string          `json:"ip_address" db:"ip_address"`
    GeoLocation     *GeoLocation    `json:"geo_location" db:"geo_location"`
}

type GameSessionStatus string

const (
    GameSessionActive      GameSessionStatus = "active"
    GameSessionPaused      GameSessionStatus = "paused"
    GameSessionCompleted   GameSessionStatus = "completed"
    GameSessionInterrupted GameSessionStatus = "interrupted"
)
```

### 6.2 Game Cycle (GLI-19 §4.3.3)

```go
type GameCycle struct {
    ID              string          `json:"id" db:"id"`
    SessionID       string          `json:"session_id" db:"session_id"`
    PlayerID        string          `json:"player_id" db:"player_id"`
    GameID          string          `json:"game_id" db:"game_id"`
    
    // Timing
    StartedAt       time.Time       `json:"started_at" db:"started_at"`
    CompletedAt     *time.Time      `json:"completed_at" db:"completed_at"`
    
    // Wager
    WagerAmount     Money           `json:"wager_amount" db:"wager_amount"`
    WagerPlacement  json.RawMessage `json:"wager_placement" db:"wager_placement"`
    
    // Outcome
    RNGOutput       []byte          `json:"-" db:"rng_output"`  // Encrypted
    Outcome         json.RawMessage `json:"outcome" db:"outcome"`
    OutcomeDisplay  string          `json:"outcome_display" db:"outcome_display"`
    
    // Winnings
    WinAmount       Money           `json:"win_amount" db:"win_amount"`
    
    // Jackpots
    JackpotContrib  Money           `json:"jackpot_contrib" db:"jackpot_contrib"`
    JackpotWin      *JackpotWin     `json:"jackpot_win,omitempty" db:"jackpot_win"`
    
    // Bonus/Features
    BonusTriggered  bool            `json:"bonus_triggered" db:"bonus_triggered"`
    BonusDetails    json.RawMessage `json:"bonus_details,omitempty" db:"bonus_details"`
    
    // Balance
    BalanceBefore   Money           `json:"balance_before" db:"balance_before"`
    BalanceAfter    Money           `json:"balance_after" db:"balance_after"`
    
    // Status
    Status          GameCycleStatus `json:"status" db:"status"`
}

type GameCycleStatus string

const (
    CycleStatusPending    GameCycleStatus = "pending"
    CycleStatusInProgress GameCycleStatus = "in_progress"
    CycleStatusCompleted  GameCycleStatus = "completed"
    CycleStatusVoided     GameCycleStatus = "voided"
    CycleStatusInterrupted GameCycleStatus = "interrupted"
)
```

### 6.3 Game Outcome Determination (GLI-19 §4.5)

```go
// GameEngine processes game logic and outcomes
type GameEngine interface {
    // InitiateGame starts a new game cycle
    InitiateGame(ctx context.Context, req *GameRequest) (*GameCycle, error)
    
    // ProcessOutcome determines game result using RNG
    ProcessOutcome(ctx context.Context, cycleID string) (*GameOutcome, error)
    
    // ProcessPlayerChoice handles player decisions (e.g., hold cards)
    ProcessPlayerChoice(ctx context.Context, cycleID string, choice *PlayerChoice) (*GameOutcome, error)
    
    // CompleteGame finalizes game cycle
    CompleteGame(ctx context.Context, cycleID string) (*GameCycle, error)
    
    // RecallGame retrieves game history for display
    RecallGame(ctx context.Context, cycleID string) (*GameRecall, error)
}

// GameOutcome represents the result of a game cycle
type GameOutcome struct {
    CycleID         string          `json:"cycle_id"`
    Symbols         []string        `json:"symbols,omitempty"`
    Cards           []Card          `json:"cards,omitempty"`
    Numbers         []int           `json:"numbers,omitempty"`
    WinningLines    []WinLine       `json:"winning_lines,omitempty"`
    TotalWin        Money           `json:"total_win"`
    BonusTriggered  bool            `json:"bonus_triggered"`
    JackpotTriggered bool           `json:"jackpot_triggered"`
    DisplayData     json.RawMessage `json:"display_data"`
}
```

### 6.4 Game Fairness Requirements (GLI-19 §4.6)

| Requirement | Implementation |
|-------------|----------------|
| No outcome manipulation | RNG output used directly, no post-selection modification |
| No near-miss substitution | Display matches actual outcome |
| No adaptive behavior | Probability constant regardless of history |
| Live game correlation | Simulated games match real-world probabilities |

### 6.5 Return to Player (RTP) (GLI-19 §4.7)

```go
type GameMath struct {
    GameID          string    `json:"game_id"`
    Version         string    `json:"version"`
    TheoreticalRTP  float64   `json:"theoretical_rtp"`  // e.g., 0.96 = 96%
    MinRTP          float64   `json:"min_rtp"`          // Must be >= 75%
    MaxRTP          float64   `json:"max_rtp"`
    HitFrequency    float64   `json:"hit_frequency"`
    Volatility      string    `json:"volatility"`       // low, medium, high
    MaxWinMultiple  float64   `json:"max_win_multiple"` // Max win / base bet
}
```

**Minimum RTP:** 75% for all wagering configurations (GLI-19 §4.7.1)

### 6.6 Game Recall (GLI-19 §4.14)

```go
type GameRecall struct {
    CycleID         string          `json:"cycle_id"`
    PlayedAt        time.Time       `json:"played_at"`
    Denomination    Money           `json:"denomination"`
    OutcomeDisplay  json.RawMessage `json:"outcome_display"`
    BalanceBefore   Money           `json:"balance_before"`
    BalanceAfter    Money           `json:"balance_after"`
    WagerAmount     Money           `json:"wager_amount"`
    WinAmount       Money           `json:"win_amount"`
    PlayerChoices   []PlayerChoice  `json:"player_choices,omitempty"`
    BonusDetails    json.RawMessage `json:"bonus_details,omitempty"`
    JackpotDetails  *JackpotWin     `json:"jackpot_details,omitempty"`
}

// RecallService provides game history access
type RecallService interface {
    // GetLastGame returns most recent completed game
    GetLastGame(ctx context.Context, playerID string, gameID string) (*GameRecall, error)
    
    // GetGameHistory returns player's game history
    GetGameHistory(ctx context.Context, playerID string, limit int) ([]*GameRecall, error)
    
    // GetBonusHistory returns last 50 bonus events
    GetBonusHistory(ctx context.Context, playerID string, gameID string) ([]*GameRecall, error)
}
```

### 6.7 Interrupted Games (GLI-19 §4.16)

```go
type InterruptedGameHandler interface {
    // DetectInterruption checks for interrupted games
    DetectInterruption(ctx context.Context, playerID string) ([]*InterruptedGame, error)
    
    // ResumeGame continues an interrupted game
    ResumeGame(ctx context.Context, cycleID string) (*GameCycle, error)
    
    // VoidGame cancels and refunds an interrupted game
    VoidGame(ctx context.Context, cycleID string, reason string) error
}

type InterruptedGame struct {
    CycleID       string          `json:"cycle_id"`
    GameID        string          `json:"game_id"`
    InterruptedAt time.Time       `json:"interrupted_at"`
    Reason        string          `json:"reason"`
    WagerHeld     Money           `json:"wager_held"`
    CanResume     bool            `json:"can_resume"`
    GameState     json.RawMessage `json:"game_state"`
}
```

---

## 7. Financial Transactions

### 7.1 Transaction Types (GLI-19 §2.5.6, §2.5.7)

```go
type Transaction struct {
    ID              string            `json:"id" db:"id"`
    PlayerID        string            `json:"player_id" db:"player_id"`
    Type            TransactionType   `json:"type" db:"type"`
    
    // Amounts
    Amount          Money             `json:"amount" db:"amount"`
    Fee             Money             `json:"fee" db:"fee"`
    
    // Balances
    BalanceBefore   Money             `json:"balance_before" db:"balance_before"`
    BalanceAfter    Money             `json:"balance_after" db:"balance_after"`
    
    // Status
    Status          TransactionStatus `json:"status" db:"status"`
    StatusReason    string            `json:"status_reason,omitempty" db:"status_reason"`
    
    // Timestamps
    CreatedAt       time.Time         `json:"created_at" db:"created_at"`
    ProcessedAt     *time.Time        `json:"processed_at" db:"processed_at"`
    CompletedAt     *time.Time        `json:"completed_at" db:"completed_at"`
    
    // Payment Details
    PaymentMethod   string            `json:"payment_method" db:"payment_method"`
    PaymentRef      string            `json:"-" db:"payment_ref_enc"` // Encrypted
    AuthorizationNo string            `json:"authorization_no" db:"authorization_no"`
    
    // Audit
    IPAddress       string            `json:"ip_address" db:"ip_address"`
    ProcessedBy     string            `json:"processed_by,omitempty" db:"processed_by"`
}

type TransactionType string

const (
    TxTypeDeposit     TransactionType = "deposit"
    TxTypeWithdrawal  TransactionType = "withdrawal"
    TxTypeWager       TransactionType = "wager"
    TxTypeWin         TransactionType = "win"
    TxTypeBonus       TransactionType = "bonus"
    TxTypeAdjustment  TransactionType = "adjustment"
    TxTypeRefund      TransactionType = "refund"
    TxTypeFee         TransactionType = "fee"
    TxTypeJackpot     TransactionType = "jackpot"
)

type TransactionStatus string

const (
    TxStatusPending   TransactionStatus = "pending"
    TxStatusApproved  TransactionStatus = "approved"
    TxStatusCompleted TransactionStatus = "completed"
    TxStatusFailed    TransactionStatus = "failed"
    TxStatusCancelled TransactionStatus = "cancelled"
    TxStatusReversed  TransactionStatus = "reversed"
)
```

### 7.2 Wallet Service

```go
// WalletService manages player balances and transactions
type WalletService interface {
    // GetBalance returns current player balance
    GetBalance(ctx context.Context, playerID string) (*Balance, error)
    
    // Deposit adds funds to player account
    Deposit(ctx context.Context, req *DepositRequest) (*Transaction, error)
    
    // Withdraw removes funds from player account
    Withdraw(ctx context.Context, req *WithdrawRequest) (*Transaction, error)
    
    // PlaceWager deducts wager amount (game integration)
    PlaceWager(ctx context.Context, req *WagerRequest) (*Transaction, error)
    
    // CreditWin adds winnings to balance (game integration)
    CreditWin(ctx context.Context, req *WinRequest) (*Transaction, error)
    
    // GetTransactionHistory returns player transactions
    GetTransactionHistory(ctx context.Context, playerID string, filter *TxFilter) ([]*Transaction, error)
    
    // GetStatement generates account statement
    GetStatement(ctx context.Context, playerID string, period TimePeriod) (*Statement, error)
}

type Balance struct {
    PlayerID        string    `json:"player_id"`
    RealMoney       Money     `json:"real_money"`
    BonusBalance    Money     `json:"bonus_balance"`
    RestrictedBonus Money     `json:"restricted_bonus"`
    PendingWithdraw Money     `json:"pending_withdraw"`
    Available       Money     `json:"available"` // Real - Pending
    Currency        string    `json:"currency"`
    UpdatedAt       time.Time `json:"updated_at"`
}
```

### 7.3 Transaction Requirements

| Requirement | Specification |
|-------------|---------------|
| Authorization | Funds not available until issuer authorization |
| No Transfers | Player-to-player transfers prohibited |
| Limit Enforcement | Respect deposit/wager limits |
| Withdrawal Address | Must match registration details |
| Negative Balance | Not permitted (except chargebacks) |

---

## 8. Data Management & Logging

### 8.1 Required Data (GLI-19 §2.8)

The system must maintain the following data categories:

#### 8.1.1 Game Play Information

```go
type GamePlayRecord struct {
    CycleID         string          `json:"cycle_id" db:"cycle_id"`
    SessionID       string          `json:"session_id" db:"session_id"`
    PlayerID        string          `json:"player_id" db:"player_id"`
    GameThemeID     string          `json:"game_theme_id" db:"game_theme_id"`
    
    // Timing
    PlayedAt        time.Time       `json:"played_at" db:"played_at"`
    
    // Denomination & Wager
    Denomination    Money           `json:"denomination" db:"denomination"`
    TotalWagered    Money           `json:"total_wagered" db:"total_wagered"`
    IncentiveWagered Money          `json:"incentive_wagered" db:"incentive_wagered"`
    
    // Outcome
    OutcomeDisplay  json.RawMessage `json:"outcome_display" db:"outcome_display"`
    TotalWon        Money           `json:"total_won" db:"total_won"`
    IncentiveWon    Money           `json:"incentive_won" db:"incentive_won"`
    JackpotWon      Money           `json:"jackpot_won" db:"jackpot_won"`
    
    // Balances
    BalanceStart    Money           `json:"balance_start" db:"balance_start"`
    BalanceEnd      Money           `json:"balance_end" db:"balance_end"`
    
    // Player Choices
    PlayerChoices   json.RawMessage `json:"player_choices" db:"player_choices"`
    
    // Bonus/Features
    BonusResults    json.RawMessage `json:"bonus_results" db:"bonus_results"`
    DoubleUpResult  json.RawMessage `json:"double_up_result" db:"double_up_result"`
    
    // Jackpot
    JackpotContrib  Money           `json:"jackpot_contrib" db:"jackpot_contrib"`
    JackpotAwarded  bool            `json:"jackpot_awarded" db:"jackpot_awarded"`
    
    // Status
    Status          string          `json:"status" db:"status"`
    
    // Location
    Location        json.RawMessage `json:"location" db:"location"`
}
```

#### 8.1.2 Significant Events (GLI-19 §2.8.8)

```go
type SignificantEvent struct {
    ID              string          `json:"id" db:"id"`
    Type            EventType       `json:"type" db:"type"`
    Severity        Severity        `json:"severity" db:"severity"`
    Timestamp       time.Time       `json:"timestamp" db:"timestamp"`
    
    // Context
    PlayerID        *string         `json:"player_id" db:"player_id"`
    SessionID       *string         `json:"session_id" db:"session_id"`
    GameID          *string         `json:"game_id" db:"game_id"`
    TransactionID   *string         `json:"transaction_id" db:"transaction_id"`
    
    // Details
    Description     string          `json:"description" db:"description"`
    Data            json.RawMessage `json:"data" db:"data"`
    
    // Source
    IPAddress       string          `json:"ip_address" db:"ip_address"`
    UserAgent       string          `json:"user_agent" db:"user_agent"`
    Component       string          `json:"component" db:"component"`
}

type EventType string

const (
    EventFailedLogin        EventType = "failed_login"
    EventProgramError       EventType = "program_error"
    EventAuthMismatch       EventType = "auth_mismatch"
    EventSystemUnavailable  EventType = "system_unavailable"
    EventLargeWin           EventType = "large_win"
    EventLargeWager         EventType = "large_wager"
    EventSystemOverride     EventType = "system_override"
    EventDataFileChange     EventType = "data_file_change"
    EventConfigChange       EventType = "config_change"
    EventTimeChange         EventType = "time_change"
    EventGameParamChange    EventType = "game_param_change"
    EventJackpotChange      EventType = "jackpot_change"
    EventAccountAdjustment  EventType = "account_adjustment"
    EventPIIChange          EventType = "pii_change"
    EventAccountDeactivation EventType = "account_deactivation"
    EventLargeTransaction   EventType = "large_transaction"
    EventNegativeBalance    EventType = "negative_balance"
    EventDataLoss           EventType = "data_loss"
)
```

### 8.2 Data Retention

| Data Type | Retention Period |
|-----------|------------------|
| Player Accounts | 5+ years after closure |
| Game History | 5+ years |
| Financial Transactions | 5+ years |
| Significant Events | 5+ years |
| Session Logs | 90 days minimum |
| Verification Records | 90 days minimum |

### 8.3 Audit Service

```go
type AuditService interface {
    // LogEvent records a significant event
    LogEvent(ctx context.Context, event *SignificantEvent) error
    
    // LogTransaction records a financial transaction
    LogTransaction(ctx context.Context, tx *Transaction) error
    
    // LogGamePlay records game play information
    LogGamePlay(ctx context.Context, play *GamePlayRecord) error
    
    // LogAccess records user/system access
    LogAccess(ctx context.Context, access *AccessLog) error
    
    // Query retrieves audit records
    Query(ctx context.Context, filter *AuditFilter) (*AuditResult, error)
    
    // Export generates audit export
    Export(ctx context.Context, filter *AuditFilter, format ExportFormat) (io.Reader, error)
}
```

---

## 9. Security Controls

### 9.1 Encryption (GLI-19 Appendix B)

#### At Rest

| Data Type | Encryption |
|-----------|------------|
| PII | AES-256-GCM |
| Authentication credentials | Argon2id hash |
| Financial data | AES-256-GCM |
| RNG output | AES-256-GCM |
| Backups | AES-256-GCM |

#### In Transit

| Protocol | Minimum Version |
|----------|-----------------|
| TLS | 1.2 (prefer 1.3) |
| Cipher Suites | ECDHE-RSA-AES256-GCM-SHA384 |
| Certificate | 2048-bit RSA or 256-bit ECDSA |

### 9.2 Access Control (GLI-19 §B.2.3)

```go
type Role string

const (
    RolePlayer        Role = "player"
    RoleOperator      Role = "operator"
    RoleAuditor       Role = "auditor"
    RoleAdmin         Role = "admin"
    RoleSuperAdmin    Role = "super_admin"
    RoleSystem        Role = "system"
)

type Permission string

const (
    PermReadPlayer      Permission = "player:read"
    PermWritePlayer     Permission = "player:write"
    PermReadGame        Permission = "game:read"
    PermManageGame      Permission = "game:manage"
    PermReadTransaction Permission = "transaction:read"
    PermWriteTransaction Permission = "transaction:write"
    PermAuditRead       Permission = "audit:read"
    PermAuditExport     Permission = "audit:export"
    PermConfigRead      Permission = "config:read"
    PermConfigWrite     Permission = "config:write"
    PermSystemAdmin     Permission = "system:admin"
)
```

### 9.3 Authentication Requirements

| User Type | Requirements |
|-----------|--------------|
| Players | Password + optional MFA |
| Operators | Password + MFA required |
| Admins | Password + MFA + IP whitelist |
| API Access | API key + signature |

### 9.4 Password Policy

```go
type PasswordPolicy struct {
    MinLength           int           // 8 minimum
    RequireUppercase    bool          // true
    RequireLowercase    bool          // true
    RequireDigit        bool          // true
    RequireSpecial      bool          // recommended
    MaxAge              time.Duration // 90 days for operators
    HistoryCount        int           // 5 previous passwords
    MaxFailedAttempts   int           // 3
    LockoutDuration     time.Duration // 30 minutes
}
```

---

## 10. Communications

### 10.1 API Protocols

| Protocol | Use Case |
|----------|----------|
| HTTPS REST | Standard API operations |
| WebSocket (WSS) | Real-time game state, live updates |
| gRPC | Internal service communication |

### 10.2 Connection Security (GLI-19 §B.4)

```go
type ConnectionConfig struct {
    // TLS Configuration
    MinTLSVersion       uint16        // tls.VersionTLS12
    CipherSuites        []uint16      // Strong ciphers only
    CertFile            string
    KeyFile             string
    
    // Connection Limits
    MaxIdleConns        int           // 100
    MaxConnsPerHost     int           // 10
    IdleConnTimeout     time.Duration // 90 seconds
    
    // Rate Limiting
    RequestsPerSecond   float64       // Per client
    BurstSize           int           // Max burst
    
    // Timeouts
    ReadTimeout         time.Duration // 30 seconds
    WriteTimeout        time.Duration // 30 seconds
    HandshakeTimeout    time.Duration // 10 seconds
}
```

### 10.3 Error Handling

```go
type APIError struct {
    Code        string `json:"code"`
    Message     string `json:"message"`
    Details     string `json:"details,omitempty"`
    RequestID   string `json:"request_id"`
    Timestamp   string `json:"timestamp"`
}

// Error codes - never expose internal details
var (
    ErrInvalidCredentials = APIError{Code: "AUTH001", Message: "Invalid credentials"}
    ErrSessionExpired     = APIError{Code: "AUTH002", Message: "Session expired"}
    ErrInsufficientFunds  = APIError{Code: "WAL001", Message: "Insufficient funds"}
    ErrGameUnavailable    = APIError{Code: "GAME001", Message: "Game unavailable"}
    ErrLimitExceeded      = APIError{Code: "LIMIT001", Message: "Limit exceeded"}
)
```

---

## 11. Reporting

### 11.1 Required Reports (GLI-19 §2.9)

#### Game Performance Report

```go
type GamePerformanceReport struct {
    ReportID        string    `json:"report_id"`
    GeneratedAt     time.Time `json:"generated_at"`
    Period          string    `json:"period"` // daily, MTD, YTD, LTD
    
    GameThemeID     string    `json:"game_theme_id"`
    GameName        string    `json:"game_name"`
    GameType        string    `json:"game_type"`
    
    TheoreticalRTP  float64   `json:"theoretical_rtp"`
    ActualRTP       float64   `json:"actual_rtp"`
    
    GamesPlayed     int64     `json:"games_played"`
    TotalWagered    Money     `json:"total_wagered"`
    IncentiveWagered Money    `json:"incentive_wagered"`
    TotalWon        Money     `json:"total_won"`
    IncentiveWon    Money     `json:"incentive_won"`
    WagersVoided    Money     `json:"wagers_voided"`
    FeesCollected   Money     `json:"fees_collected"`
    InterruptedFunds Money    `json:"interrupted_funds"`
    
    Status          string    `json:"status"` // active, disabled, decommissioned
}
```

#### Operator Liability Report

```go
type LiabilityReport struct {
    ReportID        string    `json:"report_id"`
    GeneratedAt     time.Time `json:"generated_at"`
    
    TotalPlayerFunds Money    `json:"total_player_funds"`
    OperationalFunds Money    `json:"operational_funds"`
    TotalLiability   Money    `json:"total_liability"`
}
```

#### Large Jackpot Payout Report

```go
type JackpotPayoutReport struct {
    ReportID        string    `json:"report_id"`
    GeneratedAt     time.Time `json:"generated_at"`
    
    JackpotID       string    `json:"jackpot_id"`
    WinnerPlayerID  string    `json:"winner_player_id"`
    GameThemeID     string    `json:"game_theme_id"`
    GameCycleID     string    `json:"game_cycle_id"`
    
    TriggerTime     time.Time `json:"trigger_time"`
    PayoutAmount    Money     `json:"payout_amount"`
    
    ProcessedBy     string    `json:"processed_by"`
    ConfirmedBy     string    `json:"confirmed_by"`
}
```

#### Significant Events Report

```go
type SignificantEventsReport struct {
    ReportID        string              `json:"report_id"`
    GeneratedAt     time.Time           `json:"generated_at"`
    Period          string              `json:"period"`
    
    Events          []SignificantEvent  `json:"events"`
    Summary         map[EventType]int   `json:"summary"`
}
```

### 11.2 Report Service

```go
type ReportService interface {
    // GenerateGamePerformance generates game performance report
    GenerateGamePerformance(ctx context.Context, params *ReportParams) (*GamePerformanceReport, error)
    
    // GenerateLiability generates operator liability report
    GenerateLiability(ctx context.Context, params *ReportParams) (*LiabilityReport, error)
    
    // GenerateJackpotPayouts generates jackpot payout report
    GenerateJackpotPayouts(ctx context.Context, params *ReportParams) ([]*JackpotPayoutReport, error)
    
    // GenerateSignificantEvents generates events report
    GenerateSignificantEvents(ctx context.Context, params *ReportParams) (*SignificantEventsReport, error)
    
    // ExportReport exports report in specified format
    ExportReport(ctx context.Context, reportID string, format ExportFormat) (io.Reader, error)
    
    // ScheduleReport schedules automatic report generation
    ScheduleReport(ctx context.Context, config *ReportSchedule) error
}

type ReportParams struct {
    StartDate   time.Time
    EndDate     time.Time
    Period      string    // daily, weekly, monthly, MTD, YTD, LTD
    GameIDs     []string  // Filter by games
    PlayerIDs   []string  // Filter by players
    Format      ExportFormat
}

type ExportFormat string

const (
    FormatJSON ExportFormat = "json"
    FormatCSV  ExportFormat = "csv"
    FormatXLS  ExportFormat = "xlsx"
    FormatPDF  ExportFormat = "pdf"
)
```

---

## 12. API Specification

### 12.1 Authentication Endpoints

```
POST   /api/v1/auth/register          Register new player
POST   /api/v1/auth/login             Player login
POST   /api/v1/auth/logout            Player logout
POST   /api/v1/auth/mfa/verify        Verify MFA token
POST   /api/v1/auth/password/reset    Request password reset
POST   /api/v1/auth/password/change   Change password
GET    /api/v1/auth/session           Get current session
POST   /api/v1/auth/session/refresh   Refresh session token
```

### 12.2 Player Endpoints

```
GET    /api/v1/player/profile         Get player profile
PUT    /api/v1/player/profile         Update player profile
GET    /api/v1/player/limits          Get player limits
PUT    /api/v1/player/limits          Set player limits
POST   /api/v1/player/exclusion       Self-exclude
GET    /api/v1/player/history         Get gaming history
GET    /api/v1/player/statement       Get account statement
```

### 12.3 Wallet Endpoints

```
GET    /api/v1/wallet/balance         Get current balance
POST   /api/v1/wallet/deposit         Initiate deposit
POST   /api/v1/wallet/withdraw        Request withdrawal
GET    /api/v1/wallet/transactions    Get transaction history
GET    /api/v1/wallet/transaction/:id Get transaction details
```

### 12.4 Game Endpoints

```
GET    /api/v1/games                  List available games
GET    /api/v1/games/:id              Get game details
GET    /api/v1/games/:id/rules        Get game rules/paytable
POST   /api/v1/games/:id/session      Start game session
DELETE /api/v1/games/:id/session      End game session
GET    /api/v1/games/interrupted      Get interrupted games
```

### 12.5 Game Session WebSocket

```
WS     /api/v1/ws/game/:session_id    Game session WebSocket

// Message Types
-> { "type": "wager", "amount": 100, "placement": {...} }
<- { "type": "outcome", "result": {...}, "win": 250 }
-> { "type": "choice", "action": "hold", "positions": [0,2,4] }
<- { "type": "final_outcome", "result": {...}, "win": 500 }
-> { "type": "recall" }
<- { "type": "recall_data", "history": [...] }
```

### 12.6 Admin Endpoints

```
POST   /api/v1/admin/games/:id/disable      Disable game
POST   /api/v1/admin/games/:id/enable       Enable game
POST   /api/v1/admin/players/:id/suspend    Suspend player
POST   /api/v1/admin/players/:id/unsuspend  Unsuspend player
POST   /api/v1/admin/system/disable         Disable all gaming
POST   /api/v1/admin/system/enable          Enable all gaming
GET    /api/v1/admin/reports/:type          Generate report
GET    /api/v1/admin/audit                  Query audit logs
POST   /api/v1/admin/verify                 Trigger verification
```

---

## Appendix: Data Models

### Money Type

```go
// Money represents monetary values with precision
type Money struct {
    Amount   int64  `json:"amount"`   // Amount in smallest unit (cents)
    Currency string `json:"currency"` // ISO 4217 currency code
}

func (m Money) Float64() float64 {
    return float64(m.Amount) / 100.0
}

func (m Money) String() string {
    return fmt.Sprintf("%.2f %s", m.Float64(), m.Currency)
}

func (m Money) Add(other Money) (Money, error) {
    if m.Currency != other.Currency {
        return Money{}, ErrCurrencyMismatch
    }
    return Money{Amount: m.Amount + other.Amount, Currency: m.Currency}, nil
}
```

### GeoLocation Type

```go
type GeoLocation struct {
    Latitude    float64 `json:"latitude"`
    Longitude   float64 `json:"longitude"`
    Accuracy    float64 `json:"accuracy_meters"`
    Country     string  `json:"country"`
    Region      string  `json:"region"`
    City        string  `json:"city"`
    Timezone    string  `json:"timezone"`
    IPAddress   string  `json:"ip_address"`
    VPNDetected bool    `json:"vpn_detected"`
    Timestamp   time.Time `json:"timestamp"`
}
```

### Encrypted Field Type

```go
type EncryptedField struct {
    Ciphertext []byte `db:"ciphertext"`
    Nonce      []byte `db:"nonce"`
    KeyID      string `db:"key_id"`
}

func (e *EncryptedField) Decrypt(key []byte) (string, error) {
    block, err := aes.NewCipher(key)
    if err != nil {
        return "", err
    }
    
    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return "", err
    }
    
    plaintext, err := gcm.Open(nil, e.Nonce, e.Ciphertext, nil)
    if err != nil {
        return "", err
    }
    
    return string(plaintext), nil
}
```

---

## Document History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2024-XX-XX | RGS Team | Initial specification |

---

## References

1. GLI-19: Standards for Interactive Gaming Systems V3.0 (July 2020)
2. FIPS 140-2: Security Requirements for Cryptographic Modules
3. ISO/IEC 19790: Information technology — Security techniques
4. OWASP Application Security Verification Standard

