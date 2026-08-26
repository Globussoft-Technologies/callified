# Callified AI Dialer — Agent Guide

This file is a single source of truth for AI coding agents working on the Callified AI Dialer repository. It describes the actual current layout, build/test commands, conventions, and security posture. Some older docs (including `README.md` and `docs/architecture.md`) still describe a Python/FastAPI architecture; the production codebase has been migrated to a Go-based backend, with Python only remaining in the RAG microservice and utility scripts.

---

## 1. Project Overview

Callified AI Dialer is an AI-native outbound CRM and voice dialer targeting Indian languages (Hindi, Marathi, Tamil, Telugu, Bengali, Gujarati, Kannada, Malayalam, Punjabi, English). It automates telecom sales calls, captures transcripts, updates CRM records, and supports follow-up via WhatsApp and email.

Current runtime architecture:

- **Go backend** (`backend/`): WebSocket audio pipeline, REST API, AI orchestration, billing, WhatsApp, webhooks, background workers.
- **RAG microservice** (`rag_service.py` + `rag.py`): Python/FastAPI + FAISS + sentence-transformers for knowledge-base retrieval.
- **React SPA** (`frontend/`): Vite + React 19 dashboard.
- **Mobile clients**: Expo React Native (`mobile-app/`), native Android Kotlin (`native-android/`), native iOS SwiftUI (`native-ios/`).
- **Data stores**: MySQL 8 (primary persistence), Redis 7 (call state, sessions), FAISS indexes (knowledge retrieval).
- **Reverse proxy**: Nginx routes `/media-stream`, `/ws/*`, `/api/*`, `/wa/webhook/*`, and telephony webhooks to the Go backend; static SPA files are served for all other paths.

---

## 2. Repository Structure

```
callified-ai-dailer/
├── backend/                      # Go monolithic backend
│   ├── cmd/audiod/main.go        # Entry point: HTTP server + worker wiring
│   ├── cmd/wsprobe/              # WebSocket probing utility
│   ├── cmd/setbucketpolicy/      # OCI/S3 bucket policy utility
│   ├── internal/                 # Application packages
│   ├── docs/                     # Swagger/OpenAPI docs (generated + hand-written)
│   ├── examples/                 # Manual-call and WebSocket demos
│   ├── scripts/                  # SQL migrations
│   ├── proto/                    # Protobuf definitions (legacy; currently unused)
│   ├── _archive/                 # Old code / reference artifacts
│   ├── Dockerfile                # Multi-stage Go build
│   ├── Makefile                  # Build, test, proto generation
│   ├── go.mod / go.sum           # Go module: github.com/globussoft/callified-backend
│   └── .env.example              # Backend environment template
├── docker-compose.yml            # Full-stack local deployment
├── docker-compose.override.yml   # Dev-mode hot reload override
├── Dockerfile.rag                # RAG microservice image
├── requirements.rag.txt          # RAG-only Python dependencies
├── rag.py                        # FAISS ingestion/retrieval core
├── rag_service.py                # FastAPI wrapper for rag.py
├── frontend/                     # React + Vite SPA
│   ├── src/                      # Components, pages, contexts, hooks, utils
│   ├── Dockerfile / Dockerfile.dev
│   ├── package.json              # React 19, Vite, ESLint
│   └── vite.config.js            # Dev proxy to Go backend
├── mobile-app/                   # Expo + React Native
├── native-android/               # Kotlin + Jetpack Compose Android app
├── native-ios/                   # SwiftUI iOS app
├── nginx/                        # Nginx configs
│   ├── callified.conf            # Production (TLS) config
│   ├── callified-docker.conf     # Docker Compose config
│   └── go_ramp.conf              # Legacy shadow-mode split config
├── scripts/                      # Deployment helpers
│   ├── deploy-go.sh              # Legacy blue/green binary deploy script
│   └── set-ramp.sh               # Legacy Nginx traffic split script
├── tests/                        # Test suites
│   ├── e2e/                      # Python API integration tests
│   └── ui_e2e/                   # Playwright browser tests
├── docs/                         # Architecture, API flow, schema docs
├── website/                      # Static marketing site (HTML/CSS)
└── *.py                          # Root-level utility / diagnostic scripts
```

### Important path notes
- The Go module path is `github.com/globussoft/callified-backend`.
- Legacy scripts and the README refer to a directory called `go-audio-service/`; the actual directory is `backend/`.
- Legacy protobuf and Makefile references use `github.com/globussoft/callified-audio`; this is out of sync with the real module name. The `.proto` stubs are not currently generated or used in the Go build.

---

## 3. Technology Stack

| Layer | Technology | Notes |
|-------|------------|-------|
| Backend runtime | Go 1.25 | Module `github.com/globussoft/callified-backend` |
| Web framework | `net/http` + Go 1.22 method/path mux | No external router |
| WebSocket | `github.com/gorilla/websocket` | Audio pipeline, sandbox, monitor, agent bridge |
| Auth | JWT (HS256) via `github.com/golang-jwt/jwt/v5` | Shared secret `JWT_SECRET_KEY` |
| DB driver | `github.com/go-sql-driver/mysql` | MySQL 8, `database/sql` |
| Redis | `github.com/redis/go-redis/v9` | Call state, whispers, takeover, pending calls |
| Observability | Prometheus (`client_golang`) + Zap structured logs | `/metrics` endpoint |
| STT | Deepgram WebSocket API | `internal/stt/deepgram.go` |
| TTS | ElevenLabs, Sarvam AI, SmallestAI | `internal/tts/` |
| LLM | Gemini, Groq, Anthropic | `internal/llm/`, `internal/receptionist/llm/` |
| Object storage | AWS S3 v2 SDK or OCI S3-compatible | `internal/storage/` |
| Email | SMTP | `internal/email/` |
| Billing | Razorpay | `internal/billing/` |
| PDF | `github.com/go-pdf/fpdf` | Invoice generation |
| Excel | `github.com/xuri/excelize/v2` | Exports/imports |
| Frontend | React 19, React Router DOM 7, Vite 8 | ESLint 9 flat config |
| Voice/WebRTC | `@twilio/voice-sdk`, `jssip` | Browser calling |
| RAG | Python 3.11, FastAPI, FAISS, sentence-transformers | `rag.py` |
| CI/CD | GitHub Actions | `.github/workflows/ci.yml`, `deploy-go.yml` |

---

## 4. Build and Test Commands

### Go backend (`backend/`)

```bash
cd backend

# Build the production binary (writes bin/audiod)
make build

# Run tests with race detector
make test

# CI tests (no race detector, no cache)
make test-ci

# Build Docker image
make docker

# Generate protobuf stubs (requires protoc + Go plugins; currently legacy/unused)
make proto

# Run locally from the built binary (sources ../.env if present)
make run
```

Verified commands (as of this writing):
- `go test ./... -timeout 60s` passes.
- `go build -ldflags="-s -w" -o bin/audiod ./cmd/audiod` succeeds.

### Frontend (`frontend/`)

```bash
cd frontend
npm install
npm run dev        # Vite HMR dev server on http://localhost:5173
npm run build      # Production build → dist/
npm run lint       # ESLint
npm run preview    # Preview production build
```

Verified command: `npm run build` succeeds.

### RAG microservice (root)

```bash
# Docker build
docker build -f Dockerfile.rag -t callified-rag .

# Local run (requires Python deps and a running MySQL/Redis)
python -m venv .venv
source .venv/bin/activate
pip install -r requirements.rag.txt
uvicorn rag_service:app --host 0.0.0.0 --port 8002
```

### Full-stack local run (Docker Compose)

```bash
# Copy backend env template and fill in real API keys
cp backend/.env.example backend/.env

# Production build (frontend + backend + RAG + MySQL + Redis)
docker compose up --build

# Dev mode (Vite HMR on :5173, RAG hot reload)
docker compose up --build
# open http://localhost:5173

# Rebuild only the Go service after code changes
docker compose up -d --no-deps --build go-audio
```

Port map in Docker:

| Service | Container | Internal port | Exposed locally |
|---------|-----------|---------------|-----------------|
| Go backend | `go-audio` | 8001 | **8001** |
| RAG service | `rag-service` | 8002 | **8002** (127.0.0.1) |
| Frontend | `frontend` | 80 | **80** |
| MySQL | `mysql` | 3306 | **3307** |
| Redis | `redis` | 6379 | **6380** |

---

## 5. Code Organization

### Go backend (`backend/internal/`)

| Package | Responsibility |
|---------|----------------|
| `api` | REST handlers, auth middleware, RBAC, rate limiting, route registration (`server.go`) |
| `audio` | PCM/μ-law codec, resampling, stereo WAV recording |
| `billing` | Razorpay subscriptions, prepaid credits, invoices |
| `callguard` | TRAI calling-hour regulation enforcement |
| `config` | Environment variable parsing (`backend/.env`) |
| `db` | MySQL data layer; mirrors legacy `database.py` semantics |
| `dial` | Twilio / Exotel outbound call initiation |
| `email` | SMTP transactional email |
| `llm` | LLM client, sentence splitting, prompt helpers |
| `metrics` | Prometheus instrumentations |
| `prompt` | System prompt builder for voice + WhatsApp agents |
| `rag` | HTTP client to the RAG microservice |
| `recording` | End-of-call WAV save, Gemini analysis, webhook dispatch |
| `redis` | Redis store porting legacy `redis_store.py` key scheme |
| `receptionist/...` | Embedded IVR/receptionist sub-system: conversation manager, intent detection, emergency handling, appointment booking |
| `storage` | S3 / OCI object storage clients for recordings |
| `stt` | Deepgram streaming speech-to-text |
| `tts` | Streaming TTS providers (ElevenLabs, Sarvam, SmallestAI) |
| `wa` | WhatsApp multi-provider parsing and sending |
| `webhook` | Outbound webhook dispatch with HMAC-SHA256 signing |
| `workers` | Background goroutines: scheduler, retry worker, CRM poller |
| `wshandler` | Per-call WebSocket state, pipeline orchestration, monitor, barge-in |

### Entry point wiring (`backend/cmd/audiod/main.go`)

1. Load config from environment.
2. Initialize Redis store (with in-memory fallback).
3. Open MySQL pool (REST API and recording analysis are disabled if DB is unavailable).
4. Create LLM provider, prompt builder, webhook dispatcher, recording service.
5. Create WebSocket handler and register `/media-stream`, `/ws/sandbox`, `/ws/monitor/`, `/ws/agent`.
6. Create dial initiator and wire it into the WebSocket handler.
7. Create RAG client and WhatsApp agent.
8. Register REST API routes (`api.Server.RegisterRoutes`).
9. Register Swagger UI, `/metrics`, `/health`.
10. Start background workers.
11. Graceful shutdown on SIGTERM/SIGINT with a 60-second drain window.

### Frontend (`frontend/src/`)

| Directory | Contents |
|-----------|----------|
| `components/` | Reusable UI: auth, header, onboarding wizard, notification bell, role guard, plus `campaigns/`, `common/`, `modals/`, `tabs/` |
| `pages/` | Route-level pages: CRM, campaigns, analytics, WhatsApp, billing, settings, team, manual dial, agent presence |
| `contexts/` | React providers: Auth, Org, Voice, Call, UI, Toast |
| `hooks/` | `useHideAiFeatures.js` |
| `utils/` | Phone formatting, date formatting, password policy, campaign names, roles |
| `constants/` | API base path, voice constants, lead statuses, campaign templates |
| `assets/` | Static images |

---

## 6. Development Conventions

### Go

- Go version: **1.25** (CI uses 1.25; local tests pass with 1.25.1).
- Module: `github.com/globussoft/callified-backend`.
- Formatting: `gofmt` (standard). Run `gofmt -w ./...` before committing.
- Linting: `go vet ./...` is used in the security audit workflow. There is no golangci-lint config in the repo.
- Error handling: wrap errors with `fmt.Errorf("...: %w", err)`; structured logs use `go.uber.org/zap`.
- Context: pass `context.Context` for cancellation and timeouts; worker goroutines cancel on SIGTERM.
- DB access: `internal/db` wraps `*sql.DB`; use `nullString`/`nullInt64` helpers for optional fields.
- HTTP routing: uses Go 1.22 method+path patterns (`mux.HandleFunc("GET /api/leads/{id}", ...)`). Register exact literal paths before wildcard paths.
- Tests: `*_test.go` files co-located with production code; use `github.com/stretchr/testify`.
- Environment: all configuration lives in `backend/.env` (or root `.env` for legacy scripts). Never commit secrets.

### Frontend

- Node 18+ required; repo currently builds with Node 22.
- React 19 functional components; contexts for global state.
- Vite dev server proxies `/api`, `/ws`, `/media-stream`, `/ping`, `/recordings`, `/wa/webhook` to the Go backend container.
- ESLint 9 flat config in `eslint.config.js`.
- Production build outputs to `frontend/dist/` (ignored by Git).

### Python (RAG + scripts)

- RAG microservice uses `requirements.rag.txt` intentionally minimal dependencies.
- Root `.py` files are mostly diagnostic/deployment utilities; many are `.gitignore`d because they contain hardcoded credentials or are environment-specific.

### Database

- MySQL 8, database name `callified_ai`, UTF-8 mb4.
- The Go backend auto-creates/ensures tables on startup (`internal/db/db.go`).
- New schema changes should be added as migrations under `backend/scripts/migrations/` with a dated prefix, then applied to the live database manually or via a deployment script.
- Key migrations present:
  - `migrate_app_db_for_go.sql` + `part2` / `part3` — additive schema to make legacy Python DB compatible with Go backend.
  - `20250629_add_lead_pagination_indexes.sql`
  - `20250720_add_lead_follow_up_at.sql`
  - `20250717_rbac_campaign_assignments.sql` — RBAC + campaign assignments + notifications.

---

## 7. Testing Strategy

| Suite | Location | How to run | Notes |
|-------|------------|------------|-------|
| Go unit tests | `backend/internal/**` | `cd backend && make test` | Race detector enabled |
| Go CI tests | `backend/internal/**` | `cd backend && make test-ci` | Used in GitHub Actions |
| Python API E2E | `tests/e2e/` | `python -m pytest tests/e2e/test_api_v1.py -v` | Needs running server; `E2E_BASE_URL` defaults to `http://localhost:8000` |
| Playwright UI E2E | `tests/ui_e2e/` | `python -m pytest tests/ui_e2e/ -v` | Needs `E2E_BASE_URL`, Chromium, Playwright deps |
| RAG service | `rag_service.py` + `rag.py` | Manual curl / FastAPI test client | No automated test suite currently |

### CI pipeline (`.github/workflows/ci.yml`)

1. Go tests (`make test-ci`) + Go binary build (`make build`).
2. Go Docker image build check.
3. Python API E2E tests (best-effort, continues on failure).
4. Frontend build + bundle-size check.

### Key Go test files

- `internal/api/auth_rate_limit_test.go`
- `internal/api/phone_test.go`
- `internal/wshandler/conformance_test.go`
- `internal/audio/codec_test.go`, `internal/audio/recorder_test.go`
- `internal/llm/sentence_splitter_test.go`
- `internal/receptionist/intent/detection_test.go`
- `internal/receptionist/emergency/handler_test.go`
- `internal/receptionist/conversation/manager_test.go`

---

## 8. Deployment

### Production deployment path

The current production deployment is via GitHub Actions: `.github/workflows/deploy-go.yml`.

- Trigger: push to `main` that changes `backend/**`, `docker-compose.yml`, or the workflow itself; also manual `workflow_dispatch`.
- Action: SSH into the production VM, checkout the requested ref, run `docker compose up -d --no-deps --build go-audio`, then health-check `http://localhost:8001/api/debug/health` up to 10 times.

### Legacy scripts

- `scripts/deploy-go.sh`: blue/green binary deployment with Nginx upstream swap. Hardcodes `go-audio-service/` directory; it is outdated for the current `backend/` layout.
- `scripts/set-ramp.sh`: shadow-mode traffic split between Go and a legacy Python service. Also outdated; the current architecture routes 100% of traffic to the Go backend via `nginx/callified.conf` and `nginx/callified-docker.conf`.

### Docker image

- `backend/Dockerfile`: multi-stage build from `golang:1.25-alpine` → `alpine:3.20`, runs as non-root `audiod` user, exposes `8001`, includes health check.
- `frontend/Dockerfile`: multi-stage Node 20 → nginx build; serves SPA from `/app/frontend/dist`.
- `Dockerfile.rag`: Python 3.11 slim + FAISS + sentence-transformers; pre-bakes `all-MiniLM-L6-v2` at build time.

---

## 9. Security Considerations

### Authentication & Authorization

- JWT auth with `JWT_SECRET_KEY` (must be ≥ 32 characters). Tokens contain `sub` (email), `org_id`, `role`.
- RBAC roles: `Admin`, `TeamLeader`, `Agent`, `Viewer`. Super-admin bypass exists for `SUPER_ADMIN_EMAIL`.
- Middleware re-checks `users.is_active` and current role from DB on every request (not cached in the JWT), so role changes take effect immediately.
- API keys are stored as SHA-256 hashes; callers send `X-API-Key` and are treated as org-scoped Admin for the external transcript endpoint.
- SSE endpoints use short-lived `?ticket=...` tokens with `kind="sse"`; regular long-lived JWTs are rejected as SSE tickets, and SSE tickets are rejected as Bearer tokens.

### Known high-severity issues (from `docs/security_rbac_audit_report.md`)

The following are documented in the security audit report and have not necessarily been fixed in production:

1. **WebSocket endpoints are unauthenticated.** Anyone with a `stream_sid`/`call_sid` can listen to live calls, inject whispers, or take over a call (`/media-stream`, `/ws/sandbox`, `/ws/monitor/{key}`, `/ws/agent`).
2. **Inbound WhatsApp webhooks accept unsigned requests** for Gupshup, Wati, AiSensei, Interakt, and WaSender providers.
3. **`POST /api/billing/subscribe`** can activate paid plans without payment verification.
4. **`POST /api/billing/webhook`** skips HMAC verification when `RazorpayWebhookSecret` is unset.
5. **`GET /api/debug/last-dial`** is global, not org-scoped.
6. **`GET /api/demo-requests`** returns demo requests across all tenants.
7. **`/api/pronunciation`** is global (no `org_id`).
8. **`POST /webhook/twilio/voice`** builds TwiML without validating Twilio request signatures.
9. **API keys appear in URL query strings** for SSO/API-key token exchange endpoints.
10. **Lead documents** are stored in a shared directory with original filenames and no org segregation.
11. **Public trial signup** lacks CAPTCHA/rate-limit and returns the generated plaintext password.
12. **API keys are SHA-256 hashed without salt** (rainbow-table risk if DB leaks).

### Security-sensitive conventions for agents

- **Never trust client-supplied `role` or `org_id` JSON fields.** Always resolve from the authenticated session and DB.
- **Always scope DB queries by `org_id`** unless the endpoint is explicitly global.
- **Validate inbound webhooks** with provider signatures/tokens; do not leave dev-mode bypasses in production.
- **Do not put secrets or API keys in query strings.** Use headers or POST bodies.
- **Store uploaded files under `org_id/` subdirectories with UUID filenames**, and enforce org-scoped download authorization.
- **Use bcrypt/Argon2 for new credential hashing**; avoid unsalted SHA-256 for new features.
- **Rate-limit public endpoints** (signup, login, trial signup) to prevent abuse.
- **Authenticate WebSocket upgrades** before `upgrader.Upgrade`; do not rely on stream IDs as the only authorization.

### Nginx security headers

Production and Docker configs set:

- `X-Frame-Options SAMEORIGIN`
- `X-Content-Type-Options nosniff`
- `Referrer-Policy strict-origin-when-cross-origin`
- `Permissions-Policy` restricting camera/mic/geolocation/payment/usb
- `Content-Security-Policy` (Docker config)
- `Strict-Transport-Security` (production TLS config)
- `/metrics` is restricted to private networks.

---

## 10. Common Tasks & Gotchas

### Running locally without Docker

```bash
# 1. MySQL 8 and Redis 7 must be running.
# 2. Backend
cd backend
cp .env.example .env
# edit .env: set MYSQL_HOST=127.0.0.1, REDIS_URL, all API keys
make build
./bin/audiod

# 3. Frontend (separate terminal)
cd frontend
npm install
npm run dev
# open http://localhost:5173
```

### Environment variables

- Backend config is loaded from `backend/.env` by Docker Compose (`env_file: backend/.env`).
- Root `.env` is used by some legacy root Python scripts only.
- Required secrets: `JWT_SECRET_KEY`, `MYSQL_PASSWORD`, `REDIS_PASSWORD`, `DEEPGRAM_API_KEY`, `GEMINI_API_KEY` (or `GROQ_API_KEY`), at least one TTS provider key, and Twilio or Exotel credentials.
- `PUBLIC_SERVER_URL` must be the public HTTPS URL used for telephony callbacks.

### Database schema drift

- If the Go backend warns about missing columns, run `docker compose restart go-audio` or apply the latest migration scripts manually.
- The Go backend auto-creates core tables but does not run all historical migrations automatically.

### Rebuild after Go changes

```bash
docker compose up -d --no-deps --build go-audio
```

The Go service rebuilds in ~10 seconds because dependencies are cached in the image layer.

### Rebuild after RAG changes

The dev override mounts `rag.py` and `rag_service.py` into the container and runs uvicorn with `--reload`, so saves trigger an automatic restart.

### Frontend bundle size

CI warns if `frontend/dist/assets/*.js` exceeds 1 MB. The current build produces a single ~774 kB JS chunk.

---

## 11. External Dependencies & API Keys

Required for full functionality:

- **Deepgram** — STT.
- **Google AI Studio / Gemini** — LLM (default).
- **Groq** — LLM fallback.
- **ElevenLabs** — TTS.
- **Sarvam AI** — TTS (best for Hindi/Marathi).
- **SmallestAI** — low-latency TTS.
- **Twilio** or **Exotel** — telephony.
- **Razorpay** — billing/payments.
- **SMTP** — transactional email.
- **Meta / Gupshup / Wati / Interakt / AiSensei** — WhatsApp providers (optional).

---

## 12. Where to Look for More Detail

- API flow: `docs/API_FLOW.md`
- Security/RBAC audit: `docs/security_rbac_audit_report.md`
- Database schema sketch: `docs/database_schema.md`
- Voice testing: `docs/VOICE_TESTING.md`
- WhatsApp guide: `docs/whatsapp-guide.md`
- Swagger UI: run the backend and open `/swagger/`
- OpenAPI JSON: `backend/docs/swagger.json` / `backend/docs/swagger.yaml`

---

*Last updated: 2026-07-24. This guide reflects the actual repository layout at the time of creation; always verify paths and module names against the current codebase before making large changes.*
