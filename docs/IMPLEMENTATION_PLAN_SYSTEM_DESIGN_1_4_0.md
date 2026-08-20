# Callified System Design & Implementation Plan v1.4.0

## Executive Summary

This document proposes a set of architectural and implementation changes for the Callified AI Dialer platform to resolve the most pressing live issues, improve reliability, and prepare the product for scale. The plan covers the **frontend**, **backend**, and **AI pipeline**, with concrete file references and a phased rollout.

## Implementation Status

- **Phase 1 (partially complete)** — merged in PR #97 to `dev_1.3.0`.
  - Done: provider abstraction, credential validation on save, clearer dial errors, `RecordingStorage` interface, `MYSQL_PASSWORD_FILE` support.
  - Pending: full account-priority resolution, pre-signed recording URLs, storage health check on startup, WebSocket authentication, MySQL password rotation.
- **Phase 2 (merged)** — PRs #98 and #99 merged to `dev_1.3.0`.
  - Done: `CallState` enum and `CallManager` package, call-state transitions, greeting-guard prompt patch, Redis-backed dial queue, retry logic, pause/resume/abort, progress panel, TRAI check removed.
  - Pending: locked language per call, full `ConversationState` struct.
- **Phase 3 (done)** — TanStack Query migration, central route config, lazy loading, bundle below 500 KB.
  - Done: `@tanstack/react-query` dependency, query hooks (`useCampaigns`, `useCampaign`, `useLeads`, `useCallLogs`, `useAgentReport`, `useOrganizations`, `useOrgProducts`), React Query provider wiring, `OrgContext` refactored, `EventContext` invalidates affected query keys, route table + `ProtectedRoute`, `App.jsx` refactored with lazy-loaded pages, `CampaignsPage`/`CampaignDetail`/`AgentReportPage` use query hooks, Vite chunk splitting, initial bundle 357 kB (gzipped 85 kB).
  - Pending: extend React Query to remaining ad-hoc fetches (team, executives, products, billing, etc.), agent-presence events.
- **Phase 4 (done)** — Prompt registry, AI guardrails, post-call reports, agent-specific access, lead deduplication.
  - Done: `prompt_templates` table with versioning (`20250816_add_prompt_templates.sql`), `internal/prompt/registry.go`, `internal/prompt/builder.go` resolves campaign → product → global default, Panora script seeding including `panora_v4_curiosity`, `panora_wholesale`, `panora_hotel_spa_towels`, `panora_retail`, template REST API (`/api/templates`), script selection dropdown in `ProductsTab.jsx` and `ScriptsPage.jsx`, `ApplyGuardrails` strips markdown/URLs/phones, disallowed bracket tags, repeated greetings (when `GreetingDone` is true), and enforces language-specific length limits (240 chars Indic, 200 chars English) while preserving `[HANGUP]`, `ConversationState` tracks greeting/turn/question and is injected into the LLM request, `GET /api/calls/{call_id}/report` with async analysis, `GET /api/campaigns/{id}/leads` deduplicates by phone, agents see only their own leads/calls.
  - Pending: provider fallback chains (TTS/STT/LLM) are out of scope for this branch.
- **Phase 5 (complete)** — Scale & observability.
  - Done: per-provider dial outcome metrics, call-state transition metrics, LLM token-usage/response-latency metrics, Redis dial/retry queue-depth gauges, WebSocket connection count/duration metrics, Phase 5 composite DB indexes (`leads`, `call_logs`, `users`), per-provider circuit breakers, DLQs for webhooks and recording uploads with background retry workers.
  - Done: trace IDs across API → WebSocket → provider webhook → LLM (`internal/trace`, `X-Trace-ID` header, context propagation).
  - Done: collation audit (`20250818_collation_audit.sql`), env-configurable MySQL pool (`DB_MAX_OPEN_CONNS`, `DB_MAX_IDLE_CONNS`, `DB_CONN_MAX_LIFETIME`), service-split evaluation with Redis pub/sub event bus design.

## Current Pain Points Observed

Based on recent testgo1, testgo2, and app.callified.ai issues:

1. **AI Dial failures**: Exotel 401/502, "no campaign Exotel credentials set".
2. **Manual/AI account confusion**: Route guards and menu items scattered; provider accounts duplicated in UI.
3. **Repeated greetings and language switching**: AI greets twice or changes language mid-call.
4. **Auto-dial interruption**: Manual Save/Next popups break batch calling.
5. **Stale dashboards**: Lead status updates do not reflect in real time on dashboards/reports.
6. **Agent-level access gaps**: Leads assigned to agents are still visible/callable by admins.
7. **Duplicate leads in campaigns**: Campaign lists show duplicate phone numbers.
8. **Recording link issues**: Oracle bucket URLs fail or are not generated correctly.
9. **Bulk upload failures**: CSV imports fail due to missing columns or format mismatches.
10. **Security debt**: Hardcoded DB password in `backend/.env`, unauthenticated WebSockets, unsigned webhooks.

---

## 1. Frontend Architecture

### 1.1 Replace Context-based server state with React Query

**Current state**
- `AuthContext.jsx`, `CallContext.jsx`, `OrgContext` manage server state via `useEffect` + `fetch`.
- Components poll independently, causing race conditions and stale UI.

**Target state**
- Introduce **TanStack Query (React Query)** for all server state.
- Define query keys per entity: `['org']`, `['campaigns']`, `['campaign', id]`, `['campaign', id, 'leads']`, `['callLogs']`, `['agentReport']`.

**Benefits**
- Automatic background refetch, cache invalidation, deduped requests.
- Single source of truth for lead/campaign/call data.
- Removes ~60% of ad-hoc `useEffect` fetches.

**Files to change**
- `frontend/src/contexts/AuthContext.jsx`
- `frontend/src/contexts/CallContext.jsx`
- `frontend/src/components/campaigns/CampaignDetail.jsx`
- `frontend/src/components/tabs/CrmTab.jsx`
- `frontend/src/pages/AgentReportPage.jsx`
- `frontend/src/pages/CampaignsPage.jsx`

**Migration steps**
1. Add `@tanstack/react-query` to `frontend/package.json`.
2. Wrap `App` in `QueryClientProvider`.
3. Create query hooks:
   - `useCampaigns()`
   - `useCampaign(id)`
   - `useLeads(campaignId, filters)`
   - `useCallLogs(filters)`
   - `useAgentReport(filters)`
4. Replace direct `apiFetch` in components with these hooks.
5. Mutations invalidate related query keys.

### 1.2 Real-time updates via Server-Sent Events (SSE)

**Current state**
- Dashboards and campaign pages require manual refresh.

**Target state**
- Single SSE connection `/api/events` per browser session.
- Backend pushes domain events; frontend invalidates affected React Query keys.

**Event types**
```json
{ "type": "LEAD_STATUS_CHANGED", "campaignId": 46, "leadId": 123, "status": "Qualified", "executiveId": 5 }
{ "type": "CALL_COMPLETED", "campaignId": 46, "leadId": 123, "outcome": "Interested", "duration": 42 }
{ "type": "AGENT_PRESENCE_CHANGED", "agentId": 5, "status": "on_call" }
```

**Files to change**
- Frontend: `frontend/src/contexts/EventContext.jsx` (new)
- Backend: `backend/internal/api/events.go` (new), `backend/internal/redis/store.go`

### 1.3 Centralize route permissions

**Current state**
- `App.jsx` has `hideAiFeatures ? <Navigate to="/crm" replace />` repeated per route.

**Target state**
- Define route table with `permission`, `requiresAiFeatures`, `allowedRoles`.
- Single `<ProtectedRoute />` component handles gating.

**Example route table**
```js
const routes = [
  { path: '/crm', element: CrmPage, roles: ['Admin','TeamLeader','Agent','Executive'] },
  { path: '/products', element: ProductsPage, roles: ['Admin'] },
  { path: '/analytics', element: AnalyticsPage, roles: ['Admin','TeamLeader'], permission: 'reports.view' },
  { path: '/exotel-accounts', element: ExotelAccountsPage, roles: ['Admin'], permission: 'provider_accounts.global', aiFeatures: false },
  { path: '/monitor', element: MonitorPage, aiFeatures: true },
];
```

**Files to change**
- `frontend/src/App.jsx`
- `frontend/src/utils/routeConfig.js` (new)
- `frontend/src/components/ProtectedRoute.jsx` (new)

### 1.4 Bundle splitting

**Current state**
- Single 862 KB JS chunk.

**Target state**
- Lazy-load heavy pages: Analytics, Agent Report, Team, User Management, Receptionist.

**Files to change**
- `frontend/src/App.jsx`
- `frontend/src/pages/*.jsx` imports

### 1.5 Persist call-action preferences server-side

**Current state**
- `SettingsTab.jsx` stores `callified_call_actions` in `localStorage`.

**Target state**
- Store per-user preferred call actions in `users` table (`preferred_call_mode`, `default_browser_account_id`).
- Store per-campaign default provider account in `campaigns.default_provider_account_id`.

**Files to change**
- `frontend/src/components/tabs/SettingsTab.jsx`
- `frontend/src/components/campaigns/CampaignDetail.jsx`
- `backend/internal/db/users.go`
- `backend/internal/db/campaigns.go`

---

## 2. Backend Architecture

### 2.1 Provider abstraction with credential validation

**Current state**
- Exotel/Twilio/Tata logic is spread across `internal/dial/` and `internal/wshandler/`.
- Campaigns reference credentials indirectly; validation happens late.

**Target state**
- A `Provider` interface and a registry.

```go
package dial

type Provider interface {
    Name() string
    ValidateCredentials(ctx context.Context, creds ProviderCreds) error
    InitiateCall(ctx context.Context, req InitiateCallRequest) (CallSession, error)
    BuildConnectStream(ctx context.Context, call CallSession) (string, error)
    ParseWebhook(r *http.Request) (WebhookEvent, error)
}
```

Implementations:
- `exotelProvider`
- `twilioProvider`
- `tataProvider`
- `browserProvider` (no-op outbound, local media)
- `simWebProvider`

**Files to change**
- `backend/internal/dial/provider.go` (new)
- `backend/internal/dial/exotel.go`
- `backend/internal/dial/twilio.go`
- `backend/internal/dial/tata.go` (existing)
- `backend/internal/db/exotel_accounts.go`

**Validation flow**
1. When provider account is saved, call `ValidateCredentials`.
2. When campaign is saved, ensure `default_provider_account_id` belongs to the org and is valid.
3. When dial is requested, resolve provider by priority:
   - Lead/executive assigned account
   - Campaign default account
   - User default account
   - Org fallback account
4. If no valid account, return HTTP 400 with clear message instead of 502.

### 2.2 Call state machine

**Current state**
- State is implicit in Redis + `wshandler` logic.

**Target state**
- Explicit `CallManager` with states and transitions.

```go
type CallState string
const (
    StatePending     CallState = "pending"
    StateDialing     CallState = "dialing"
    StateConnected   CallState = "connected"
    StateSpeaking    CallState = "speaking"
    StateListening   CallState = "listening"
    StateCompleted   CallState = "completed"
    StateFailed      CallState = "failed"
    StateNoAnswer    CallState = "no_answer"
    StateBusy        CallState = "busy"
)
```

**Files to change**
- `backend/internal/callmanager/` (new package)
- `backend/internal/wshandler/handler.go`
- `backend/internal/wshandler/pipeline.go`
- `backend/internal/wshandler/session.go`

### 2.3 Queue-based auto-dialer

**Current state**
- Auto Dial is driven by frontend/backend loops without a durable queue.

**Target state**
- Redis-backed queue per campaign.
- Worker pool consumes queue, respecting:
  - Provider rate limits
  - TRAI call-hour rules (`internal/callguard`)
  - Agent availability
  - Retry budget

**Queue keys**
```
callified:campaign:{id}:dial_queue          # leads to dial now
callified:campaign:{id}:retry_queue         # failed leads with backoff
callified:campaign:{id}:completed           # terminal outcomes
```

**Files to change**
- `backend/internal/workers/dialer_worker.go` (new)
- `backend/internal/workers/scheduler.go`
- `backend/internal/redis/store.go`
- `backend/internal/dial/initiator.go`

**Auto-dial UX**
- Remove forced Save/Next popup.
- Frontend subscribes to SSE and shows progress (called/remaining/qualified/appointments).
- Allow pause/resume/abort.

### 2.4 WebSocket authentication

**Current state**
- `/media-stream`, `/ws/monitor/{key}`, `/ws/agent`, `/ws/sandbox` are unauthenticated (per security audit).

**Target state**
- Validate JWT or ticket **before** `upgrader.Upgrade`.
- Ticket endpoint generates short-lived WebSocket tickets.
- Reject unknown `stream_sid`/`call_sid`.

**Files to change**
- `backend/internal/api/server.go`
- `backend/internal/wshandler/handler.go`
- `backend/internal/api/auth.go`

### 2.5 Storage abstraction for recordings

**Current state**
- Recording URLs directly reference Oracle bucket paths.
- No health check or signed URLs.

**Target state**
- `RecordingStorage` interface:
  ```go
  type RecordingStorage interface {
      Store(ctx, key string, data io.Reader) (StoredObject, error)
      GetURL(ctx, key string, expiry time.Duration) (string, error)
      HealthCheck(ctx) error
  }
  ```
- Implementations: OCI, S3, local.
- On read, generate pre-signed URL.
- Health-check on startup.

**Files to change**
- `backend/internal/storage/recording.go` (new)
- `backend/internal/storage/oci.go`
- `backend/internal/storage/s3.go`
- `backend/internal/recording/service.go`

### 2.6 Database hardening

**Actions**
- Add composite indexes:
  - `(org_id, campaign_id, status)` on leads
  - `(org_id, executive_id, status)` on leads
  - `(campaign_id, created_at)` on call_logs
  - `(org_id, role)` on users
- Audit all `utf8mb4_unicode_ci` vs `utf8mb4_0900_ai_ci` joins.
- Tune MySQL connection pool in `internal/db/db.go`.

**Files to change**
- `backend/internal/db/db.go`
- `backend/scripts/migrations/` (new migration files)

---

## 3. AI Pipeline

### 3.1 Conversation state management

**Current state**
- Prompt builder has limited memory of what has already happened.
- Greeting can repeat.

**Target state**
- Maintain `ConversationState` per session:
  ```json
  {
    "greetingDone": true,
    "languageLocked": "hi-IN",
    "questionsAsked": ["company_age", "import_history"],
    "lastSpeaker": "ai",
    "turnCount": 3,
    "buyerProfile": "wholesale_distributor"
  }
  ```
- Inject this state into every LLM call.
- Guard against repeating greetings.

**Files to change**
- `backend/internal/prompt/builder.go`
- `backend/internal/wshandler/session.go`
- `backend/internal/llm/provider.go`

### 3.2 Lock language per call

**Current state**
- Language changes dynamically during calls.

**Target state**
- Determine language from campaign settings at call start.
- Lock STT, LLM, TTS to that language.
- Only allow language switching if explicitly enabled for the campaign.

**Files to change**
- `backend/internal/wshandler/handler.go`
- `backend/internal/stt/deepgram.go`
- `backend/internal/tts/` providers

### 3.3 Prompt registry and versioning

**Current state**
- Prompts are hard-coded in `internal/prompt/builder.go`.

**Target state**
- Store prompt templates in DB: `prompt_templates(id, name, version, content, variables, language, product_type)`.
- Campaigns reference `opening_script_id`.
- Support variants like the Panora scripts provided by the user:
  - `panora_v4_curiosity`
  - `panora_wholesale`
  - `panora_hotel_procurement`
  - `panora_retail`

**Files to change**
- `backend/internal/prompt/registry.go` (new)
- `backend/internal/prompt/builder.go`
- `backend/scripts/migrations/20250814_add_prompt_templates.sql`
- Frontend: product creation page should allow selecting/previewing scripts.

### 3.4 LLM output guardrails

**Target state**
- Post-process every LLM output before TTS:
  - Strip repeated greetings if `greetingDone`.
  - Remove hallucinated URLs/phone numbers.
  - Limit response length.
  - Map to allowed intents.

**Files to change**
- `backend/internal/llm/provider.go`
- `backend/internal/wshandler/pipeline.go`

### 3.5 Provider fallback chains

**Target state**
- Configurable fallback per language and campaign.
- TTS: Sarvam (Hindi/Marathi) → ElevenLabs → SmallestAI.
- STT: Deepgram → Groq Whisper.
- LLM: Gemini → Groq → Anthropic.

**Files to change**
- `backend/internal/llm/fallback.go` (new)
- `backend/internal/stt/fallback.go` (new)
- `backend/internal/tts/fallback.go` (new)

### 3.6 Async post-call analysis

**Current state**
- Recording analysis runs synchronously or inline.

**Target state**
- Queue transcript/recording for async analysis.
- Extract: outcome, sentiment, summary, cost, recording URL, qualified, appointment.
- Provide single endpoint: `GET /api/calls/{call_id}/report`.

**Files to change**
- `backend/internal/recording/service.go`
- `backend/internal/workers/analysis_worker.go` (new)
- `backend/internal/llm/analysis.go` (new)

---

## 4. Observability & Reliability

### 4.1 Metrics to add

- [ ] Per-provider dial success/failure rate.
- [ ] Call state transition counts.
- [ ] LLM token usage and latency.
- [ ] STT/TTS latency.
- [ ] Queue depth per campaign.
- [ ] WebSocket connection count and duration.

### 4.2 Circuit breakers

- [ ] If provider returns 401/5xx repeatedly, pause dialing and alert.
- [ ] Auto-recovery after cooldown.

### 4.3 Dead-letter queues

- [ ] Failed webhooks → retry 3 times → DLQ.
- [ ] Failed recording uploads → retry → DLQ.

### 4.4 Tracing

- [x] Add trace IDs across API → WebSocket → provider webhook → LLM.

---

## 5. Security

### 5.1 Secret management

- [ ] Remove plaintext DB password from `backend/.env`.
- [ ] Use systemd `EnvironmentFile` split or file-based secrets.
- [ ] Rotate MySQL password.

### 5.2 Webhook verification

- [ ] Verify Twilio and Exotel webhook signatures.
- [ ] Remove dev-mode bypasses.

### 5.3 WebSocket auth

- [ ] Authenticate all WebSocket upgrades.

### 5.4 File uploads

- [ ] Store recordings/docs under `org_id/` subdirectories with UUID filenames.
- [ ] Validate upload MIME types and size.

---

## 6. Phased Implementation Roadmap

### Phase 1 — Stability (Weeks 1–2)

#### Backend
- [x] 1.1 Create `dial.Provider` interface with Exotel, Twilio, Tata implementations.
- [x] 1.2 Add credential validation on provider account save.
- [ ] 1.3 Resolve provider account by priority (lead → campaign → user → org fallback).
- [x] 1.4 Return clear 4xx errors instead of 502 for missing/invalid credentials.

#### Storage
- [x] 1.5 Create `storage.RecordingStorage` interface (OCI/S3/local).
- [ ] 1.6 Generate pre-signed recording URLs at read time.
- [ ] 1.7 Add storage health check on startup.

#### Security
- [ ] 1.8 Authenticate WebSocket upgrades before `upgrader.Upgrade`.
- [x] 1.9 Remove plaintext DB password from `backend/.env`; move to file-based secrets.
- [ ] 1.10 Rotate MySQL password.

---

### Phase 2 — Dialer & State (Weeks 3–4)

#### State Management
- [x] 2.1 Define explicit `CallState` enum and `CallManager` package.
- [~] 2.2 Track all call transitions and emit events. (Transitions tracked; event emission pending.)

#### Auto-Dialer
- [x] 2.3 Implement Redis-backed queue per campaign (`campaign:{id}:dial_queue` via global `dial_queue` + per-campaign state).
- [x] 2.4 Add retry queue with exponential backoff.
- [~] 2.5 Respect TRAI call-hour rules (`internal/callguard`). **Disabled by request — callguard always allows.**
- [x] 2.6 Remove forced Save/Next popup from AI auto-dial UX (queue runs uninterrupted; browser auto-dial uninterrupted mode defaults to on).
- [x] 2.7 Add pause/resume/abort controls.

#### Language Stability
- [ ] 2.8 Lock language from campaign settings at call start.
- [ ] 2.9 Enforce locked language across STT/LLM/TTS.

#### Conversation State
- [~] 2.10 Add `ConversationState` per session (`greetingDone`, `questionsAsked`, etc.). (`greetingDone` guard implemented; full struct pending.)
- [x] 2.11 Guard against repeated greetings.

---

### Phase 3 — Frontend Modernization (Weeks 5–6)

#### State Layer
- [x] 3.1 Add `@tanstack/react-query` dependency.
- [x] 3.2 Create query hooks: `useCampaigns`, `useCampaign`, `useLeads`, `useCallLogs`, `useAgentReport`.
- [x] 3.3 Replace ad-hoc `useEffect` fetches in key pages.
- [x] 3.4 Mutations invalidate related query keys.

#### Real-Time
- [x] 3.5 Implement SSE endpoint `/api/events` in backend.
- [x] 3.6 Create `EventContext` in frontend.
- [x] 3.7 Push events for lead status, call completion, agent presence.
- [x] 3.8 Extend events to agent presence and other mutations (CRM, products, billing).

#### Routing
- [x] 3.9 Create central `routeConfig.js` with permissions and AI-feature flags.
- [x] 3.10 Create `ProtectedRoute` wrapper.
- [x] 3.11 Replace scattered `hideAiFeatures ? <Navigate to="/crm" />` in `App.jsx`.

#### Performance
- [x] 3.12 Lazy-load Analytics, Agent Report, Team, User Management, Receptionist.
- [x] 3.13 Reduce initial bundle below 500 KB.

---

### Phase 4 — AI & Analytics (Weeks 7–8)

#### Prompt Registry
- [x] 4.1 Add `prompt_templates` table with versioning.
- [x] 4.2 Migrate existing hard-coded prompts into registry.
- [x] 4.3 Add Panora script variants (`panora_v4_curiosity`, `panora_wholesale`, `panora_hotel_spa_towels`, `panora_retail`).
- [x] 4.4 Allow script selection per campaign/product.

#### AI Guardrails
- [x] 4.5 Post-process LLM output before TTS with `ApplyGuardrailsWithState`.
- [x] 4.6 Strip repeated greetings when `GreetingDone` is true and remove hallucinated URLs/phone numbers.
- [x] 4.7 Strip disallowed bracket tags (e.g. `[HOLD]`), enforce response length limits (240 chars Indic / 200 chars English), and map outputs to allowed intents.

#### Provider Fallbacks
- [ ] 4.8 Implement TTS fallback chain (Sarvam → ElevenLabs → SmallestAI).
- [ ] 4.9 Implement STT fallback chain (Deepgram → Groq Whisper).
- [ ] 4.10 Implement LLM fallback chain (Gemini → Groq → Anthropic).

#### Post-Call Analysis
- [x] 4.11 Queue recording/transcript for async analysis.
- [x] 4.12 Extract outcome, sentiment, summary, cost, recording URL, qualified, appointment.
- [x] 4.13 Add `GET /api/calls/{call_id}/report`.

#### Access & Data Quality
- [x] 4.14 Implement agent-specific lead visibility and call ownership.
- [x] 4.15 Add duplicate phone filtering in campaign lead lists.

---

### Phase 5 — Scale & Observability (Weeks 9–10)

Status as of audit on current branch (`feat/phase-4-prompt-registry-guardrails`):

#### Metrics
- [x] 5.1 Add per-provider dial success/failure metrics.  
  **Status:** ✅ Implemented in `backend/internal/dial/initiator.go`. `callified_dial_attempts_total{provider,outcome}` is emitted on every dial attempt with outcomes: `success`, `dnd`, `call_hours`, `insufficient_credits`, `invalid_credentials`, `provider_error`, `unknown`.
- [x] 5.2 Track call state transitions.  
  **Status:** ✅ Implemented in `backend/internal/callmanager/state.go`. `callified_call_state_transitions_total{from,to}` is incremented on every valid transition.
- [x] 5.3 Track LLM token usage and latency.  
  **Status:** ✅ Implemented in `backend/internal/llm/provider.go`. `callified_llm_token_usage_total{provider,direction}` records approximate input/output tokens; `callified_llm_response_seconds{provider}` records full response latency.
- [x] 5.4 Track STT/TTS latency and queue depth.  
  **Status:** ✅ Implemented. STT/TTS first-byte latencies already existed; added `callified_queue_depth{queue}` for `dial` and `retry` queues (`backend/internal/redis/queue.go`); added `callified_websocket_connections{type}` and `callified_websocket_duration_seconds{type}` for `media`, `sandbox`, `monitor`, and `agent` endpoints.

#### Reliability
- [x] 5.5 Add circuit breakers for providers.  
  **Status:** ✅ Implemented in `backend/internal/dial/circuitbreaker.go`. Per-provider circuit breaker (`closed`/`open`/`half-open`) wraps `InitiateCall` in `backend/internal/dial/initiator.go`. Config: 5 failures to open, 2 successes to close, 30-second cooldown. Open circuits fast-fail with a clear error.
- [x] 5.6 Add dead-letter queues for failed webhooks and uploads.  
  **Status:** ✅ Implemented.
  - Webhooks: `backend/internal/webhook/dispatch.go` retries 3 times with backoff, then moves failures to `webhook_dlq`. `Dispatcher.RetryDLQ()` runs every 60 seconds via a background goroutine in `cmd/audiod/main.go`.
  - Recording uploads: `backend/internal/recording/service.go` retries OCI/S3 uploads 3 times; after exhaustion it writes a `recording_dlq` row and falls back to local disk. `Service.RetryRecordingDLQ()` runs every 5 minutes from `cmd/audiod/main.go`.
- [x] 5.7 Add trace IDs across API → WebSocket → webhook → LLM.  
  **Status:** ✅ Implemented.
  - `backend/internal/trace/trace.go` provides context-scoped trace IDs (`X-Trace-ID` header / `trace_id` query param), HTTP middleware, and helper functions.
  - `cmd/audiod/main.go` wraps the mux with `trace.Middleware`; all REST requests get a trace ID.
  - WebSocket handlers (`handler.go`, `bridge.go`, `monitor.go`) extract the trace ID from the upgrade request and add it to session loggers.
  - LLM provider (`internal/llm/provider.go`) logs `trace_id` in `ProcessTranscript`, `GenerateResponse`, and `GenerateText`.
  - Webhook dispatcher (`internal/webhook/dispatch.go`) includes `trace_id` in payloads and headers; DLQ retries preserve it.
  - Provider dial callbacks (`internal/dial/initiator.go`) append `trace_id` to status callback URLs; webhook handlers (`internal/api/dial_webhooks.go`) continue the trace on callback.

#### Database
- [x] 5.8 Add indexes for lead/campaign/agent filtering.  
  **Status:** ✅ Implemented. Migration `backend/scripts/migrations/20250817_phase5_indexes.sql` and auto-ensure in `backend/internal/db/db.go` create:
  - `idx_leads_org_campaign_status` on `leads(org_id, campaign_id, status)`
  - `idx_leads_org_executive_status` on `leads(org_id, executive_id, status)`
  - `idx_call_logs_campaign_created` on `call_logs(campaign_id, created_at)`
  - `idx_users_org_role` on `users(org_id, role)`
- [x] 5.9 Audit collation mismatches.  
  **Status:** ✅ Implemented.
  - Migration `backend/scripts/migrations/20250818_collation_audit.sql` standardises all Go-managed and legacy call/lead/user tables to `utf8mb4_0900_ai_ci` and pins exact-match identifier columns (`leads.phone`, `call_logs.call_sid`, `call_logs.phone`, `call_transcripts.call_sid`, `whatsapp_conversations.phone`, `wa_channel_configs.phone_number`, `org_exotel_accounts.caller_id`, `org_exotel_accounts.account_sid`, `executives.phone`) to `utf8mb4_bin`.
  - `backend/scripts/migrate_app_db_for_go.sql` and `backend/internal/db/exotel_accounts.go`, `backend/internal/db/executives.go` create new tables with the same collations.
  - Explicit `COLLATE utf8mb4_0900_ai_ci` overrides in `backend/internal/db/agent_activities.go` and `backend/internal/db/campaigns.go` are replaced with `COLLATE utf8mb4_bin` so joins against JSON-extracted call_sids use the exact-match column collation.
- [x] 5.10 Tune MySQL connection pool.  
  **Status:** ✅ Implemented.
  - `backend/internal/config/config.go` adds `DB_MAX_OPEN_CONNS`, `DB_MAX_IDLE_CONNS`, and `DB_CONN_MAX_LIFETIME` with the previous hard-coded values as defaults.
  - `backend/internal/db/db.go` exposes `PoolConfig` and `New(dsn, poolConfig)` so the pool can be tuned per environment.
  - `backend/cmd/audiod/main.go` passes the configured values from `cfg` into `db.New`.
  - `backend/.env.example` documents the new variables.
  - Pending (out of scope for this slice): periodic health check and slow-query logging.

#### Architecture
- [x] 5.11 Evaluate splitting monolith into API / dialer / worker services.  
  **Status:** ✅ Evaluated. The current `audiod` binary remains a monolith for the short term; the recommended medium-term split is documented below and in the new Section 6 "Service-Split Evaluation".
- [x] 5.12 Design inter-service event bus if split proceeds.  
  **Status:** ✅ Designed. Redis pub/sub is the pragmatic first event bus; a persistent event store (Kafka / RabbitMQ / AWS EventBridge) is reserved for later scale.

---

## 6. Service-Split Evaluation

### 6.1 Current monolith (`audiod`)

Everything runs in a single Go binary today:

| Concern | Package | Scale pressure |
|---------|---------|----------------|
| REST API + auth | `internal/api` | Medium — correlates with dashboard users. |
| WebSocket media streams | `internal/wshandler` | High — one persistent connection per AI call; CPU-bound on audio resampling. |
| Outbound dialer | `internal/dial`, `internal/callmanager` | Bursty — traffic spikes during campaign auto-dial. |
| Background workers | `internal/workers` | Low/Medium — retry loops, schedulers, CRM pollers. |
| Webhook dispatch | `internal/webhook` | Medium — retries and DLQ can backlog during provider outages. |
| Recording upload | `internal/recording` | High — egress bandwidth to OCI/S3 during peak hours. |

### 6.2 Recommended split

A **three-service** split balances deployment independence with manageable operational overhead:

1. **`callified-api`** — REST API, auth, RBAC, campaign/leads CRUD, analytics queries, webhook reception from providers.
2. **`callified-dialer`** — WebSocket media streams, STT/TTS/LLM pipeline, outbound dial initiation, provider callbacks.
3. **`callified-workers`** — Scheduler, retry/DLQ workers, recording upload, CRM sync, post-call analysis.

### 6.3 Service boundaries and data ownership

| Function | Service | Owned state | Shared read state |
|----------|---------|-------------|-------------------|
| Login, JWT minting, RBAC | `callified-api` | `users`, `user_permissions`, `organizations` | — |
| Campaigns, leads, products | `callified-api` | `campaigns`, `leads`, `products` | `call_logs` (read-only for analytics) |
| Analytics / reports | `callified-api` | materialised report tables / cache | `call_logs`, `agent_activities`, `call_transcripts` |
| Live AI call pipeline | `callified-dialer` | in-memory `CallManager`, Redis call state | reads `campaigns`, `leads`, `org_exotel_accounts`; writes `call_logs`, `call_transcripts` |
| Provider callbacks | `callified-dialer` | transient Twilio/Exotel/Tata sessions | reads `call_logs` |
| Retry / DLQ | `callified-workers` | `webhook_dlq`, `recording_dlq` | reads `call_logs`, `webhooks` |
| Scheduled calls | `callified-workers` | `scheduled_calls` | reads `campaigns`, `leads` |

### 6.4 Inter-service communication

**Phase 1 — Redis pub/sub (recommended now)**
- `callified-dialer` publishes events: `call.started`, `call.ended`, `call.status_changed`.
- `callified-api` subscribes and invalidates caches / sends SSE to dashboards.
- `callified-workers` subscribes and triggers post-call analysis, recording upload, DLQ retries.
- **Pros:** Already required for call state; no new infrastructure; sub-millisecond latency.
- **Cons:** No persistence — messages are lost if a subscriber is offline; no ordering guarantees.

**Phase 2 — Persistent event bus (reserved for >10k concurrent calls)**
- Kafka / RabbitMQ / AWS EventBridge for durable, ordered call-event streams.
- Use Redis pub/sub only for transient real-time signals; persistent bus for audit/reconciliation.

### 6.5 API contract between services

| Caller | Endpoint / Channel | Purpose |
|--------|--------------------|---------|
| `callified-dialer` | `POST callified-api/internal/calls` | Persist call start/end, update lead status, allocate credits. |
| `callified-dialer` | `GET callified-api/internal/campaigns/{id}/config` | Fetch campaign + provider account at call start. |
| `callified-workers` | `POST callified-api/internal/calls/{id}/analysis` | Push post-call analysis results. |
| `callified-api` | Redis pub/sub `dialer:commands:{campaign_id}` | Trigger auto-dial batches, pause/resume. |

### 6.6 Deployment and migration path

1. **Extract a shared internal client package** (`internal/apiclient`) so services can call each other with mutual-TLS or signed JWTs.
2. **Run dialer + API as separate processes behind feature flag** (`SPLIT_DIALER=true`) while still sharing the same binary build; validates routing before true split.
3. **Move `internal/workers` to its own binary** (`cmd/workerd`) first — lowest risk, no WebSockets.
4. **Split `internal/wshandler` and `internal/dial` into `cmd/dialerd`** once worker split is stable.
5. **Keep MySQL + Redis shared** during transition; introduce per-service read replicas later if needed.

### 6.7 Trade-off summary

| Approach | Pros | Cons | Recommendation |
|----------|------|------|----------------|
| Keep monolith | Simplest ops; shared memory; no network partitions | Single deploy blast radius; WebSocket load crowds REST API; scaling is coarse | Short-term OK; do not scale beyond ~1k concurrent AI calls |
| 3-service split | Independent scale; API can deploy without dialer; worker retries isolated | More services to monitor; inter-service auth/latency; needs event bus | Recommended at 1k+ concurrent calls or when 99th-percentile API latency suffers |
| Microservices per provider | Finest scaling; isolated provider failures | Operational explosion; premature for current size | Defer |

**Decision:** Stay monolith for the next 1–2 releases; complete the shared internal client and Redis event contracts now so the split is low-risk when needed.

---

## 7. Success Metrics

| Metric | Current | Target |
|--------|---------|--------|
| AI dial failure rate | High (401/502) | < 2% |
| Auto-dial interruptions | Forced popups | Zero interruptions |
| Dashboard refresh delay | Manual refresh | < 3 seconds via SSE |
| Repeated greeting rate | Observable | Zero |
| Duplicate leads in campaign list | Present | Zero |
| WebSocket auth coverage | None | 100% |
| Bundle size | 862 KB | < 500 KB initial |
| MySQL query latency p95 | > 500 ms | < 100 ms |

---

## 8. Risk Mitigation

| Risk | Mitigation |
|------|------------|
| Large refactor breaks dialer | Incremental changes behind feature flags; A/B test on testgo1/testgo2 first. |
| Provider credential rotation causes outages | Validate new credentials before switching; keep old credentials as fallback during transition. |
| DB migrations lock tables | Run migrations during low-traffic windows; use `pt-online-schema-change` if needed. |
| SSE load on backend | Use Redis pub/sub to fan out events; one SSE connection per user. |
| Prompt registry confuses existing campaigns | Keep default prompt backward-compatible; migration sets `opening_script_id` for existing campaigns. |

---

## 9. Suggested Next Immediate Actions

- [ ] **1. Provider abstraction + credential validation** — highest ROI; fixes live Exotel failures.
- [ ] **2. Conversation state + locked language** — fixes greeting/language issues immediately.
- [ ] **3. Queue-based auto-dialer** — enables uninterrupted auto-dial.
- [ ] **4. React Query + SSE** — modernizes frontend and fixes stale dashboard.
- [ ] **5. Prompt registry + Panora scripts** — makes scripts configurable per campaign.

Would you like me to start implementing any of these phases, or create smaller PR-sized tickets for a specific phase?
