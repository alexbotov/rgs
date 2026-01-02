# RGS - Remote Gaming Server

A Go-based Remote Gaming Server designed for GLI-19 compliance.

## Features

- **GLI-19 Compliant** - Implements key requirements from GLI-19 Standards for Interactive Gaming Systems V3.0
- **Cryptographic RNG** - CSPRNG with rejection sampling and chi-square testing
- **Player Management** - Registration, authentication, session management
- **Wallet System** - Deposits, withdrawals, wagers, and transaction history
- **Game Engine** - Extensible game engine with sample slot games
- **Audit Logging** - Comprehensive event logging per GLI-19 §2.8.8
- **REST API** - Full REST API with JWT authentication
- **WebSocket** - Real-time game sessions

## Quick Start

```bash
# Build
go build -o rgs .

# Run
./rgs
```

The server will start on port 8080.

## API Usage

### 1. Register a Player

```bash
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "username": "player1",
    "email": "player1@example.com",
    "password": "password123",
    "accept_tc": true
  }'
```

### 2. Login

```bash
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "username": "player1",
    "password": "password123"
  }'
```

Save the returned `token` for subsequent requests.

### 3. Deposit Funds

```bash
curl -X POST http://localhost:8080/api/v1/wallet/deposit \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{
    "amount": 100.00,
    "reference": "initial-deposit"
  }'
```

### 4. Check Balance

```bash
curl http://localhost:8080/api/v1/wallet/balance \
  -H "Authorization: Bearer YOUR_TOKEN"
```

### 5. List Games

```bash
curl http://localhost:8080/api/v1/games \
  -H "Authorization: Bearer YOUR_TOKEN"
```

### 6. Start Game Session

```bash
curl -X POST http://localhost:8080/api/v1/games/fortune-slots/session \
  -H "Authorization: Bearer YOUR_TOKEN"
```

Save the returned `session_id`.

### 7. Play Game

```bash
curl -X POST http://localhost:8080/api/v1/games/play \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{
    "session_id": "YOUR_SESSION_ID",
    "wager_amount": 100
  }'
```

Note: `wager_amount` is in cents (100 = $1.00)

### 8. View Game History

```bash
curl http://localhost:8080/api/v1/games/history \
  -H "Authorization: Bearer YOUR_TOKEN"
```

## WebSocket Usage

Connect to the WebSocket endpoint for real-time game sessions:

```javascript
const ws = new WebSocket('ws://localhost:8080/api/v1/ws/game/SESSION_ID', [], {
  headers: { 'Authorization': 'Bearer YOUR_TOKEN' }
});

ws.onmessage = (event) => {
  const msg = JSON.parse(event.data);
  console.log(msg.type, msg.payload);
};

// Play a spin
ws.send(JSON.stringify({
  type: 'spin',
  payload: { wager_amount: 100 }
}));
```

## API Endpoints

| Endpoint | Method | Description | Auth |
|----------|--------|-------------|------|
| `/` | GET | Server info | No |
| `/health` | GET | Health check + RNG status | No |
| `/api/v1/auth/register` | POST | Register player | No |
| `/api/v1/auth/login` | POST | Login | No |
| `/api/v1/auth/logout` | POST | Logout | Yes |
| `/api/v1/auth/session` | GET | Session info | Yes |
| `/api/v1/wallet/balance` | GET | Get balance | Yes |
| `/api/v1/wallet/deposit` | POST | Deposit funds | Yes |
| `/api/v1/wallet/withdraw` | POST | Withdraw funds | Yes |
| `/api/v1/wallet/transactions` | GET | Transaction history | Yes |
| `/api/v1/games` | GET | List games | Yes |
| `/api/v1/games/{id}` | GET | Game details | Yes |
| `/api/v1/games/{id}/session` | POST | Start game session | Yes |
| `/api/v1/games/{id}/session` | DELETE | End game session | Yes |
| `/api/v1/games/play` | POST | Play game | Yes |
| `/api/v1/games/history` | GET | Game history | Yes |
| `/api/v1/ws/game/{session_id}` | WS | WebSocket game | Yes |

## Available Games

| Game ID | Name | Type | RTP | Min Bet | Max Bet |
|---------|------|------|-----|---------|---------|
| `fortune-slots` | Fortune Slots | Slots | 96% | $0.10 | $100.00 |
| `lucky-sevens` | Lucky Sevens | Slots | 94% | $0.25 | $50.00 |

## Project Structure

```
rgs/
├── main.go                      # Entry point
├── go.mod                       # Go modules
├── internal/
│   ├── api/                     # HTTP handlers & routing
│   │   ├── handlers.go          # REST API handlers
│   │   ├── middleware.go        # Auth middleware
│   │   ├── router.go            # Route setup
│   │   └── websocket.go         # WebSocket handler
│   ├── audit/                   # Audit logging (GLI-19 §2.8.8)
│   │   └── audit.go
│   ├── auth/                    # Authentication (GLI-19 §2.5)
│   │   ├── auth.go
│   │   └── auth_test.go
│   ├── config/                  # Configuration
│   │   └── config.go
│   ├── database/                # Database layer
│   │   └── database.go
│   ├── domain/                  # Domain models
│   │   ├── models.go
│   │   └── models_test.go
│   ├── game/                    # Game engine (GLI-19 Chapter 4)
│   │   ├── engine.go
│   │   ├── slots.go
│   │   └── game_test.go
│   ├── rng/                     # RNG service (GLI-19 Chapter 3)
│   │   ├── rng.go
│   │   └── rng_test.go
│   └── wallet/                  # Wallet service (GLI-19 §2.5.6)
│       ├── wallet.go
│       └── wallet_test.go
├── tests/
│   └── integration/             # E2E integration tests
│       └── integration_test.go
└── docs/
    ├── README.md
    ├── TECHNICAL_SPECIFICATION.md
    └── GLI-19-Interactive-Gaming-Systems-v3.0.pdf
```

## Documentation

| Document | Description |
|----------|-------------|
| **[Technical Specification](docs/TECHNICAL_SPECIFICATION.md)** | Complete RGS implementation guide |
| **[GLI-19 V3.0](docs/GLI-19-Interactive-Gaming-Systems-v3.0.pdf)** | Official GLI standard |

## Configuration

Environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `RGS_PORT` | `8080` | Server port |
| `RGS_DB_DRIVER` | `postgres` | Database driver |
| `RGS_DB_DSN` | `host=localhost dbname=rgs sslmode=disable` | PostgreSQL connection string |
| `RGS_JWT_SECRET` | `rgs-dev-secret...` | JWT signing secret |
| `RGS_CURRENCY` | `USD` | Default currency |

## GLI-19 Compliance

This implementation addresses key GLI-19 requirements:

| Section | Requirement | Implementation |
|---------|-------------|----------------|
| §2.2 | System Clock | UTC time synchronization |
| §2.5 | Player Account Management | Registration, auth, sessions |
| §2.5.3 | Authentication | JWT tokens, session timeout |
| §2.5.4 | Inactivity | 30-minute session timeout |
| §2.5.6 | Financial Transactions | Wallet service with audit trail |
| §2.8 | Information to be Maintained | Comprehensive database schema |
| §2.8.8 | Significant Events | Audit logging service |
| Chapter 3 | RNG Requirements | CSPRNG with chi-square testing |
| §4.3 | Gaming Session | Game sessions and cycles |
| §4.5 | Game Outcome | RNG-based outcome determination |
| §4.7 | Payout Percentages | Minimum 75% RTP (96% implemented) |
| §4.14 | Game Recall | Game history API |

## Development

```bash
# Run all tests
go test ./...

# Run tests with verbose output
go test -v ./...

# Run integration tests only
go test -v ./tests/integration/...

# Run unit tests for specific package
go test -v ./internal/rng/...

# Run tests with coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Run with hot reload (requires air)
air

# Build for production
CGO_ENABLED=1 go build -ldflags="-s -w" -o rgs .
```

## Test Suite

The project includes comprehensive tests organized as follows:

### Integration Tests (`tests/integration/`)

End-to-end tests that verify the complete system flow:

- **TestHealthEndpoint** - Health check API
- **TestServerInfoEndpoint** - Server info API
- **TestPlayerRegistration** - Player registration flow
- **TestPlayerLogin** - Authentication flow
- **TestSessionManagement** - Session lifecycle
- **TestWalletOperations** - Deposit, withdraw, balance
- **TestGameOperations** - Game sessions and play
- **TestRNGService** - RNG functionality
- **TestCompletePlayerJourney** - Full E2E player flow
- **TestAuditLogging** - Audit event recording

### Unit Tests

- **internal/rng/** - RNG generation, distribution, chi-square tests
- **internal/auth/** - Authentication, token validation, lockout
- **internal/wallet/** - Balance operations, transactions
- **internal/game/** - Game engine, sessions, play
- **internal/domain/** - Domain model tests

## Security Notes

⚠️ **For Production Use:**

1. Change `RGS_JWT_SECRET` to a strong random value
2. Configure PostgreSQL with proper authentication
3. Enable TLS/HTTPS
4. Implement rate limiting
5. Add proper logging infrastructure
6. Configure firewall rules
7. Regular security audits

## License

MIT
