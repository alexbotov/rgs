# RGS Postman Test Suite

End-to-end API tests for the Remote Gaming Server, generated from the Go integration tests.

## Files

- `RGS_Collection.postman_collection.json` - Main test collection
- `RGS_Local.postman_environment.json` - Local development environment

## Import into Postman

1. Open Postman
2. Click **Import** (top-left)
3. Drag both JSON files or select them
4. The collection and environment will be imported

## Configuration

1. Select the **RGS Local** environment from the dropdown (top-right)
2. Update `base_url` if your server runs on a different port:
   - Click the environment dropdown → Edit
   - Change `http://localhost:8090` to your server URL

## Collection Structure

Each folder is **self-contained** and can be run independently. All folders follow the same structure:

| Folder | Prerequisites | Description |
|--------|---------------|-------------|
| **Health Check** | None | Basic server health (2 tests) |
| **Authentication** | None | Full auth flow with register/login (10 tests) |
| **Wallet** | None | Includes register/login, then wallet ops (8 tests) |
| **Games** | None | Includes register/login/deposit, then gameplay (11 tests) |
| **Complete Player Journey** | None | Full E2E flow (12 tests) |

Each folder starts with numbered steps (1, 2, 3...) and includes all setup needed (registration, login, deposit where applicable).

## Running Tests

### Run Entire Collection

1. Click the **RGS - Remote Gaming Server** collection
2. Click **Run** (or right-click → Run collection)
3. Select **Run RGS - Remote Gaming Server**

### Run Individual Folders

Each folder is self-contained - just select any folder and run it independently:
- Right-click folder → **Run folder**
- All tests will pass without running other folders first

### Run with Newman (CLI)

```bash
# Install Newman
npm install -g newman

# Run entire collection
newman run RGS_Collection.postman_collection.json \
  -e RGS_Local.postman_environment.json

# Run specific folder
newman run RGS_Collection.postman_collection.json \
  -e RGS_Local.postman_environment.json \
  --folder "Authentication"

# Run with HTML report
newman run RGS_Collection.postman_collection.json \
  -e RGS_Local.postman_environment.json \
  -r html
```

## Test Coverage

| Folder | Tests |
|--------|-------|
| Health Check | 2 |
| Authentication | 10 |
| Wallet | 8 |
| Games | 11 |
| Complete Player Journey | 12 |
| **Total** | **43** |

## Test Scenarios by Folder

### Health Check (2 tests)
1. Health endpoint - verify server is healthy
2. Server info - verify server name and version

### Authentication (10 tests)
1. Register new player
2. Register duplicate username (409 expected)
3. Register without T&C (400 expected)
4. Register short password (400 expected)
5. Login successful
6. Login invalid password (401 expected)
7. Login non-existent user (401 expected)
8. Get session
9. Get session unauthorized (401 expected)
10. Logout

### Wallet (8 tests)
1. Register new player
2. Login
3. Check initial balance (0)
4. Deposit $100
5. Check balance after deposit ($100)
6. Withdraw $25 ($75 remaining)
7. Withdraw insufficient funds (400 expected)
8. Transaction history

### Games (11 tests)
1. Register new player
2. Login
3. Deposit $100
4. List games
5. Get game details (Fortune Slots)
6. Start game session
7. Play game ($1.00 wager)
8. Play game ($0.50 wager)
9. Play game insufficient balance (400 expected)
10. Play game below minimum (400 expected)
11. Game history

### Complete Player Journey (12 tests)
Full end-to-end flow simulating a real player:
1. Register new account
2. Login
3. Check initial balance
4. Deposit $500
5. Browse available games
6. Start game session
7. Play 3 rounds at $5 each
8. Check game history
9. Check final balance
10. Withdraw winnings
11. View transaction history
12. Logout

## Notes

- Tests use dynamic timestamps to generate unique usernames/emails per run
- Authentication tokens are automatically stored and reused within each folder
- Each folder uses shared collection variables (`current_username`, `token`, etc.)
- Pre-request scripts handle test data generation
- Test scripts validate responses and log progress to console
- Console output shows step-by-step progress with results
