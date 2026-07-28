# agent-weave server

Go backend for agent-weave. Gin + GORM + Wire + Anthropic SDK.

## Prerequisites

- Go 1.25+
- MySQL 8.0+

## Setup

**1. Create the MySQL database** (tables are created automatically at startup, but the database itself must exist):

```bash
mysql -u root -p -e "CREATE DATABASE IF NOT EXISTS agentweave CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;"
```

**2. Configure the service:**

```bash
cp config.yaml.example config.yaml
```

Edit `config.yaml`:

```yaml
llm_model:
  anthropic:
    api_key: "sk-ant-..."
    base_url: ""          # leave empty to use the default Anthropic endpoint
    model: "claude-sonnet-4-6"

database:
  database_url: "mysql://root:password@tcp(localhost:3306)/agentweave"

server:
  port: "8080"
```

## Run

```bash
go run ./cmd/server/
```

The server auto-runs DB migrations on startup. On first run it creates the `conversations`, `messages`, and `threads` tables.

## Test

Tests use an in-memory SQLite database — no MySQL required.

```bash
go test ./test/... -race -v
```

To run a single test:

```bash
go test ./test/handler/... -run TestConversation_CreateAndList -v
```

## API

### Health

```bash
curl http://localhost:8080/health
```

### Conversations

```bash
# List
curl http://localhost:8080/api/conversations

# Create
curl -X POST http://localhost:8080/api/conversations \
  -H 'Content-Type: application/json' \
  -d '{"title": "my chat"}'

# Create with default title
curl -X POST http://localhost:8080/api/conversations \
  -H 'Content-Type: application/json' \
  -d '{}'
```

### Messages

```bash
# List messages (replace CONV_ID)
curl http://localhost:8080/api/conversations/CONV_ID/messages

# List with cursor (keyset pagination)
curl "http://localhost:8080/api/conversations/CONV_ID/messages?after_created_at=2024-01-01T00:00:00.000000000Z&after_id=MSG_ID"

# Send a message (triggers agent response)
curl -X POST http://localhost:8080/api/conversations/CONV_ID/messages \
  -H 'Content-Type: application/json' \
  -d '{"content": "Hello!"}'
```

### SSE stream

```bash
curl -N http://localhost:8080/api/conversations/CONV_ID/stream
```

Events are newline-delimited JSON. The stream closes after a `queue_drained` event.
