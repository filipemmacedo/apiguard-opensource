# APIGuard

A self-hostable LLM proxy with a built-in dashboard. Route API calls through APIGuard to get usage tracking, cost estimation, PII detection, NSFW filtering, and rate limiting — without changing your existing OpenAI-compatible client code.

```
Your App  →  APIGuard  →  OpenAI / Anthropic / Mistral / ...
                ↓
           Dashboard (localhost:3000)
```

## Features

- **Drop-in proxy** — accepts `POST /v1/chat/completions` with an OpenAI-compatible API
- **Multi-tenant** — issue separate API keys per user or service; usage tracked independently
- **Usage dashboard** — logs, token counts, and estimated cost per request
- **Cost estimation** — static price table for common models (GPT-4o, Claude, Gemini, Mistral, etc.)
- **Guardrails** — PII detection, NSFW term blocking, rate limiting with quarantine
- **API key management** — create, revoke, and delete tenant keys from the dashboard
- **Provider management** — store encrypted LLM provider credentials; sync available models
- **Cost limits** — optional monthly EUR spend cap per tenant key

## Quick Start

**Prerequisites:** Docker and Docker Compose.

```bash
git clone https://github.com/your-org/apiguard.git
cd apiguard

# Copy the example env file and set a master key
cp .env.compose.example .env.compose
# Edit .env.compose and set SECRET_MASTER_KEY:
#   openssl rand -hex 32

docker compose --env-file .env.compose up --build
```

| Service   | URL                   |
|-----------|-----------------------|
| Dashboard | http://localhost:3000 |
| Proxy API | http://localhost:8080 |

Open the dashboard, go to **API Management**, add an LLM provider (your OpenAI/Anthropic key), then create a tenant API key. Use that key to call the proxy:

```bash
curl http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer <your-tenant-key>" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"Hello!"}]}'
```

## Configuration

### `.env.compose` (Docker Compose)

| Variable | Default | Description |
|---|---|---|
| `SECRET_MASTER_KEY` | *(required)* | Hex key for encrypting stored provider credentials. Generate with `openssl rand -hex 32`. |
| `DATABASE_DSN` | `postgres://apiguard:apiguard@postgres:5432/apiguard?sslmode=disable` | PostgreSQL connection string. |
| `LISTEN_ADDR` | `:8080` | Proxy listen address. |
| `UPSTREAM_TIMEOUT` | `30s` | Timeout for upstream LLM requests. |
| `ENABLE_TEST_INTERFACE` | `false` | Expose a minimal HTML test form at `/internal/test-interface`. |
| `OPENAI_BASE_URL` | `https://api.openai.com` | Default base URL for OpenAI-compatible providers. |

### `.env` (local `go run`)

Same variables as above. SQLite is used by default when `DATABASE_DSN` is not set:

| Variable | Default | Description |
|---|---|---|
| `USAGE_DB_PATH` | `usage.db` | SQLite file path (SQLite mode only). |

### Optional legacy bootstrap

Set these to seed a provider and tenant keys on first start (they are imported into the database and can be removed afterward):

```bash
UPSTREAM_BASE_URL=https://api.openai.com
UPSTREAM_API_KEY=sk-...
TENANT_API_KEYS=alice:key-alice,bob:key-bob
```

## Architecture

```
┌─────────────────────────────────────────────────┐
│                   Docker Compose                │
│                                                 │
│  ┌──────────┐    ┌──────────────┐    ┌───────┐  │
│  │Dashboard │───▶│  api-guard   │───▶│  LLM  │  │
│  │ :3000    │    │  :8080       │    │ APIs  │  │
│  └──────────┘    └──────┬───────┘    └───────┘  │
│                         │                       │
│                  ┌──────▼───────┐               │
│                  │  PostgreSQL  │               │
│                  └──────────────┘               │
└─────────────────────────────────────────────────┘
```

**`api-guard`** (Go) — the proxy core:
- Authenticates tenant API keys
- Forwards requests to the configured LLM provider
- Records usage, PII findings, and guardrail outcomes
- Serves dashboard and admin APIs at `/internal/*`

**`dashboard`** (React / TanStack Start) — the frontend:
- Logs and usage overview
- API key and provider management
- Guardrails configuration (PII, NSFW, rate limiting)
- Playground for testing prompts

**`postgres`** — persistent storage for keys, usage logs, guardrail config, and audit events.

## Local Development (without Docker)

**Backend:**

```bash
# Requires Go 1.22+
go run ./cmd/apiguard
# Starts on :8080 using SQLite (usage.db)
```

**Dashboard:**

```bash
cd dashboard
npm install
npm run dev
# Starts on :3000, proxies /internal/* to :8080
```

**Postgres + full stack:**

```bash
cp .env.compose.example .env.compose
# Edit .env.compose — set SECRET_MASTER_KEY
docker compose up --build
```

## Proxy API

**`POST /v1/chat/completions`**

Requires `Authorization: Bearer <tenant-api-key>`. Request and response format follow the OpenAI Chat Completions spec.

```json
{
  "model": "gpt-4o-mini",
  "messages": [{"role": "user", "content": "Hello"}]
}
```

The model must be enabled in the provider's model catalog (managed from the dashboard).

## Cost Estimation

Model costs are estimated from a static price table in [`internal/proxy/static_pricing.go`](internal/proxy/static_pricing.go). Prices are approximate public rates in USD per 1M tokens, converted to EUR at a fixed rate.

To update prices for a model, edit the table and open a PR.

## Supported Models (price table)

OpenAI: `gpt-4o`, `gpt-4o-mini`, `gpt-4-turbo`, `gpt-3.5-turbo`, `o1`, `o1-mini`, `o3-mini`

Anthropic: `claude-3-5-sonnet-*`, `claude-3-5-haiku-*`, `claude-3-opus-*`, `claude-3-haiku-*`

Google: `gemini-1.5-pro`, `gemini-1.5-flash`, `gemini-2.0-flash`

Mistral: `mistral-large-latest`, `mistral-small-latest`, `codestral-latest`

Unknown models are proxied normally; cost is recorded as zero.

## Contributing

1. Fork the repository
2. Create a feature branch
3. Run `go test ./...` (backend) and `npm test` (dashboard) before opening a PR
4. Open a pull request against `main`

## License

Apache 2.0 — see [LICENSE](LICENSE).
