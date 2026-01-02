# RGS Postman Test Suite

End-to-end API tests for the Remote Gaming Server, generated from the Go integration tests.

## Prerequisites

**Important:** The RGS does not expose a public registration API. Test users must be created before running these tests.

Create test users via the auth service or directly in the database. The collection expects the following variables to be set:

- `test_username` - Username of an existing test user (default: `testuser`)
- `test_password` - Password for the test user (default: `password123`)

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
2. Update variables as needed:
   - Click the environment dropdown → Edit
   - Set `base_url` to your server URL (default: `http://localhost:8090`)
   - Set `test_username` and `test_password` to valid credentials

## Collection Structure

Each folder is **self-contained** and can be run independently (assuming a test user exists):

| Folder | Prerequisites | Description |
|--------|---------------|-------------|
| **Health Check** | None | Basic server health (2 tests) |
| **Authentication** | Test user | Login, session management, logout (6 tests) |
| **Wallet** | Test user | Login then wallet operations (7 tests) |
| **Games** | Test user | Login, deposit, then gameplay (10 tests) |
| **Complete Player Journey** | Test user | Full E2E flow (11 tests) |

Each folder starts with a login step and includes all other setup needed (deposit where applicable).

## Running Tests

### Run Entire Collection

1. Click the **RGS - Remote Gaming Server** collection
2. Click **Run** (or right-click → Run collection)
3. Select **Run RGS - Remote Gaming Server**

### Run Individual Folders

Each folder is self-contained - just select any folder and run it independently:
- Right-click folder → **Run folder**
- All tests will pass without running other folders first (assuming a test user exists)

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
| Authentication | 6 |
| Wallet | 7 |
| Games | 10 |
| Complete Player Journey | 11 |
| **Total** | **36** |

## Test Scenarios by Folder

### Health Check (2 tests)
1. Health endpoint - verify server is healthy
2. Server info - verify server name and version

### Authentication (6 tests)
1. Login successful
2. Login invalid password (401 expected)
3. Login non-existent user (401 expected)
4. Get session
5. Get session unauthorized (401 expected)
6. Logout

### Wallet (7 tests)
1. Login
2. Check initial balance
3. Deposit $100
4. Check balance after deposit
5. Withdraw $25
6. Withdraw insufficient funds (400 expected)
7. Transaction history

### Games (10 tests)
1. Login
2. Deposit $100
3. List games
4. Get game details (Fortune Slots)
5. Start game session
6. Play game ($1.00 wager)
7. Play game ($0.50 wager)
8. Play game insufficient balance (400 expected)
9. Play game below minimum (400 expected)
10. Game history

### Complete Player Journey (11 tests)
Full end-to-end flow simulating a real player:
1. Login
2. Check initial balance
3. Deposit $500
4. Browse available games
5. Start game session
6. Play 3 rounds at $5 each
7. Check game history
8. Check final balance
9. Withdraw winnings
10. View transaction history
11. Logout

## Notes

- **Test users must be pre-created** - the collection no longer includes registration tests
- Authentication tokens are automatically stored and reused within each folder
- Each folder uses shared collection variables (`test_username`, `token`, etc.)
- Test scripts validate responses and log progress to console
- Console output shows step-by-step progress with results
