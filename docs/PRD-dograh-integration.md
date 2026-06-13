# PRD: Dograh Surgical Integration — Visual Workflow Builder & Voice Architecture Patterns

**Status:** Draft  
**Author:** AI-Architect (Kimi)  
**Date:** 2026-06-10  
**Branch Target:** `golang-2`  
**License Basis:** BSD 2-Clause (github.com/dograh-hq/dograh)

---

## 1. Executive Summary

This PRD defines a **surgical extraction** of high-ROI components from the open-source voice AI platform **Dograh** into our existing `globussoft-ai-dailer` (Callified) stack. We will **not** replace our Go backend, FreeSWITCH infrastructure, or CRM integrations. Instead, we will extract and adapt Dograh's proven frontend and architectural patterns to accelerate feature delivery and reduce engineering bottlenecks.

**Core Thesis:** Our Go backend with 90+ CRM providers, billing, TRAI compliance, and multi-language support (Hindi, Bengali, Marathi) is a competitive moat. Dograh's value to us is its **React workflow builder** and **telephony abstraction patterns** — not its Python backend.

---

## 2. Goals & Objectives

| # | Goal | Success Metric |
|---|------|----------------|
| 1 | Enable non-engineers (ops/sales) to design call campaigns visually | Campaign creation time drops from 2 days (engineering) to 30 minutes (ops) |
| 2 | Support Twilio alongside Exotel without custom spaghetti code | New telephony provider integration time: < 1 day |
| 3 | Allow rapid campaign testing without real phone calls | Prompt iteration cycle: hours → minutes |
| 4 | Improve call personalization via pre-call CRM data injection | Greeting personalization rate: 0% → 80% |
| 5 | Maintain zero platform licensing costs | Monthly Dograh licensing cost: $0 |

---

## 3. Scope

### 3.1 In Scope ✅

1. **Visual Workflow Builder (React)**
   - Drag-and-drop canvas for call flow design
   - Nodes: `StartCall`, `Greeting`, `Qualify`, `Branch`, `Webhook`, `Transfer`, `EndCall`
   - Edge transitions with natural-language conditions
   - Integration with existing campaign API

2. **Telephony Abstraction Layer (Go)**
   - Refactor `backend/internal/dial/` into provider-agnostic interface
   - Exotel adapter (migrate existing)
   - Twilio adapter (new)
   - Future-proof for Vonage, Telnyx

3. **In-Dashboard WebRTC Test Calls**
   - Browser-based voice testing without telephony setup
   - Mock STT/LLM/TTS pipeline for rapid iteration

4. **Pre-Call CRM Data Injection**
   - Fetch lead data before call starts
   - Variable injection into prompts (`{{customer_name}}`, `{{account_status}}`)
   - 10-second timeout with graceful fallback

5. **Architecture Patterns (Reference Only)**
   - Barge-in handling strategies
   - Audio stream buffering patterns
   - Call trace/observability structure

### 3.2 Out of Scope ❌

| Item | Reason |
|------|--------|
| Dograh Python backend replacement | Our Go backend is mature and custom-built |
| PostgreSQL / MinIO migration | We use MongoDB + MySQL + Redis successfully |
| Full Dograh Docker deployment | No need to run Dograh as a separate service |
| Langfuse integration (Phase 1) | Nice-to-have; defer to Phase 2 |
| MCP server | Overkill for current roadmap |
| Speech-to-Speech (Gemini Flash Live) | Experimental; evaluate separately |

---

## 4. Technical Architecture

### 4.1 Current Stack

```
Frontend (React)
  ├── CampaignsTab.jsx
  ├── CampaignDetail.jsx
  └── CampaignModals.jsx

Backend (Go)
  ├── internal/api/campaigns.go
  ├── internal/dial/exotel.go
  ├── internal/dial/initiator.go
  ├── internal/wshandler/
  └── internal/receptionist/

Infrastructure
  ├── FreeSWITCH (Docker)
  ├── MongoDB + MySQL + Redis
  └── Exotel (primary telephony)
```

### 4.2 Target Architecture

```
Frontend (React)
  ├── NEW: WorkflowBuilder/ (extracted from Dograh)
  │     ├── Canvas.jsx
  │     ├── nodes/ (StartCall, Greeting, Qualify, Branch, etc.)
  │     ├── edges/ (TransitionEdge)
  │     └── PropertyPanel.jsx
  ├── CampaignsTab.jsx (integrates WorkflowBuilder)
  └── NEW: TestCall/WebRTCTester.jsx

Backend (Go)
  ├── internal/api/campaigns.go (extends for workflow_definition JSON)
  ├── internal/dial/
  │     ├── provider.go (NEW: TelephonyProvider interface)
  │     ├── exotel.go (refactored: implements TelephonyProvider)
  │     ├── twilio.go (NEW: implements TelephonyProvider)
  │     └── initiator.go (adds pre-call CRM fetch)
  ├── internal/wshandler/ (reference Dograh patterns for barge-in)
  └── internal/campaign/
        └── workflow.go (NEW: workflow execution engine)

Infrastructure
  ├── FreeSWITCH (Docker) — unchanged
  ├── MongoDB + MySQL + Redis — unchanged
  └── Exotel + Twilio (via abstraction)
```

---

## 5. Detailed Feature Specifications

### 5.1 Feature: Visual Workflow Builder

**User Story:** As an ops manager, I want to drag-and-drop call flow nodes so that I can create campaigns without engineering help.

**Acceptance Criteria:**
- [ ] Canvas supports pan, zoom, and snap-to-grid
- [ ] Node types available: StartCall, Greeting, Agent, Qualify, Branch, Webhook, Transfer, EndCall
- [ ] Each node has a properties panel (prompt text, voice settings, timeout)
- [ ] Edges have condition labels (e.g., "interested == true", "language == 'hi'")
- [ ] Workflow serializes to JSON (`workflow_definition`) stored in MongoDB
- [ ] Existing campaigns without workflow_definition render in legacy mode
- [ ] Export/import workflow JSON

**API Changes:**
```go
// backend/internal/campaign/workflow.go
type WorkflowNode struct {
    ID       string                 `json:"id" bson:"_id"`
    Type     string                 `json:"type" bson:"type"` // startCall, greeting, agent, branch, etc.
    Position Position               `json:"position" bson:"position"`
    Data     map[string]interface{} `json:"data" bson:"data"`
}

type WorkflowEdge struct {
    ID        string `json:"id" bson:"_id"`
    Source    string `json:"source" bson:"source"`
    Target    string `json:"target" bson:"target"`
    Condition string `json:"condition" bson:"condition"`
}

type WorkflowDefinition struct {
    Nodes []WorkflowNode `json:"nodes" bson:"nodes"`
    Edges []WorkflowEdge `json:"edges" bson:"edges"`
}
```

**UI Reference:** Dograh's `workflow_definition` schema with nodes and edges.

---

### 5.2 Feature: Telephony Abstraction

**User Story:** As a developer, I want to add a new telephony provider by implementing an interface, not rewriting call logic.

**Acceptance Criteria:**
- [ ] `TelephonyProvider` interface defined in `internal/dial/provider.go`
- [ ] Exotel implementation passes all existing call flows
- [ ] Twilio implementation supports outbound calls
- [ ] Provider selection per-campaign in UI
- [ ] Call initiation, status callbacks, and hangup events abstracted

**Interface Design:**
```go
// backend/internal/dial/provider.go
type TelephonyProvider interface {
    Name() string
    InitiateCall(ctx context.Context, req CallRequest) (*CallResponse, error)
    HandleWebhook(w http.ResponseWriter, r *http.Request) error
    TransferCall(ctx context.Context, callID string, to string) error
    HangupCall(ctx context.Context, callID string) error
    GetCallStatus(ctx context.Context, callID string) (CallStatus, error)
}
```

**Migration Path:**
1. Extract Exotel logic from `exotel.go` into `ExotelProvider` struct
2. Update `initiator.go` to use `TelephonyProvider` interface
3. Add `TwilioProvider` implementing same interface
4. Campaign model gets `telephony_provider` field

---

### 5.3 Feature: WebRTC Test Mode

**User Story:** As a campaign manager, I want to test my voice bot in the browser so that I can iterate prompts without making real phone calls.

**Acceptance Criteria:**
- [ ] "Test Campaign" button in campaign detail page
- [ ] Opens WebRTC audio session in browser
- [ ] Uses mock STT (typed text input) or real Deepgram stream
- [ ] Uses real LLM (Gemini) but with test flag
- [ ] Uses real TTS but caches responses
- [ ] Full transcript visible in real-time
- [ ] No telephony charges incurred during test

**Architecture:**
```
Browser (React)
  → WebSocket → wshandler/ (test mode flag)
  → Mock/Real STT → Gemini LLM → TTS
  → Audio streamed back via WebRTC data channel
```

**Backend Changes:**
```go
// internal/wshandler/handler.go
const (
    ModeProduction = "production"
    ModeTest       = "test"
)

type SessionConfig struct {
    Mode        string `json:"mode"` // production or test
    CampaignID  string `json:"campaign_id"`
    UseRealSTT  bool   `json:"use_real_stt"` // false = typed input
    UseRealTTS  bool   `json:"use_real_tts"` // true for voice testing
}
```

---

### 5.4 Feature: Pre-Call CRM Data Injection

**User Story:** As a sales rep, I want the AI to greet leads by name and reference their account status so that calls feel personalized.

**Acceptance Criteria:**
- [ ] Campaign config accepts `pre_call_webhook_url`
- [ ] HTTP POST fired before call initiation with lead context
- [ ] 10-second timeout; failure doesn't block call
- [ ] Response JSON variables injected into campaign prompts
- [ ] Variables accessible via `{{variable_name}}` syntax in prompt templates
- [ ] Support for auth headers (Bearer, API key, Basic)

**Data Flow:**
```
Dial Initiator
  → POST /crm-pre-call-webhook (lead_id, phone, campaign_id)
  → CRM Response: {"customer_name": "Rahul", "plan": "Premium"}
  → Prompt: "Hello {{customer_name}}, I see you're on the {{plan}} plan..."
  → Call proceeds with injected variables
```

**Schema:**
```go
// internal/dial/initiator.go
type PreCallFetchConfig struct {
    URL         string            `json:"url" bson:"url"`
    Method      string            `json:"method" bson:"method"`
    Headers     map[string]string `json:"headers" bson:"headers"`
    TimeoutSec  int               `json:"timeout_sec" bson:"timeout_sec"`
    FallbackVars map[string]string `json:"fallback_vars" bson:"fallback_vars"`
}

type CallContext struct {
    LeadID      string                 `json:"lead_id"`
    Phone       string                 `json:"phone"`
    CampaignID  string                 `json:"campaign_id"`
    CRMData     map[string]interface{} `json:"crm_data"`
    Variables   map[string]string      `json:"variables"` // merged CRM + fallback
}
```

---

## 6. Implementation Phases

### Phase 1: Foundation (Week 1)
- [ ] Clone Dograh repo locally for code reference
- [ ] Set up new `frontend/src/components/WorkflowBuilder/` directory
- [ ] Extract and port Dograh's canvas, node, and edge React components
- [ ] Strip Dograh-specific API calls; adapt to our campaign API
- [ ] Design `workflow_definition` JSON schema

### Phase 2: Workflow Builder Integration (Week 2)
- [ ] Build node types: StartCall, Greeting, Qualify, Branch, EndCall
- [ ] Build property panels for each node
- [ ] Integrate builder into `CampaignDetail.jsx`
- [ ] Save/load workflow_definition to/from MongoDB
- [ ] Legacy campaign fallback (non-workflow campaigns still work)

### Phase 3: Telephony Abstraction (Week 3)
- [ ] Define `TelephonyProvider` interface
- [ ] Refactor Exotel into `ExotelProvider`
- [ ] Build `TwilioProvider`
- [ ] Update campaign model with `telephony_provider` field
- [ ] Test both providers with existing call flows

### Phase 4: WebRTC Test Mode (Week 4)
- [ ] Add WebRTC client in React
- [ ] Add `ModeTest` to WebSocket handler
- [ ] Build mock STT input (typed text) option
- [ ] Add "Test Campaign" button to UI
- [ ] Log test transcripts separately from production

### Phase 5: Pre-Call CRM Injection (Week 5)
- [ ] Add `pre_call_webhook_url` to campaign config
- [ ] Build HTTP fetcher with timeout and fallback
- [ ] Implement `{{variable}}` substitution in prompt builder
- [ ] Integrate with existing CRM providers (ActiveCampaign, Zoho, etc.)
- [ ] Test with real lead data

### Phase 6: Polish & QA (Week 6)
- [ ] End-to-end testing of full workflow builder → test mode → production call
- [ ] Performance testing (canvas with 50+ nodes)
- [ ] Documentation for ops team
- [ ] Training session for non-technical users

---

## 7. Database Schema Changes

### MongoDB: `campaigns` collection

```json
{
  "_id": "...",
  "name": "Insurance Lead Qualification",
  "workflow_definition": {
    "nodes": [
      {
        "id": "node-1",
        "type": "startCall",
        "position": { "x": 100, "y": 100 },
        "data": {
          "pre_call_webhook": {
            "url": "https://api.ourcrm.com/lead-data",
            "headers": { "Authorization": "Bearer xxx" },
            "timeout_sec": 10
          }
        }
      },
      {
        "id": "node-2",
        "type": "greeting",
        "position": { "x": 100, "y": 250 },
        "data": {
          "prompt": "Hello {{customer_name}}, this is an AI assistant from Globussoft. How are you today?",
          "voice_id": "en-US-Neural2-F",
          "language": "en"
        }
      }
    ],
    "edges": [
      {
        "id": "edge-1",
        "source": "node-1",
        "target": "node-2",
        "condition": ""
      }
    ]
  },
  "telephony_provider": "exotel",
  "mode": "production",
  "created_at": "...",
  "updated_at": "..."
}
```

### MySQL: No schema changes required (campaigns in MongoDB)

---

## 8. API Changes

### New Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/campaigns/:id/workflow` | Save workflow definition |
| `GET` | `/api/campaigns/:id/workflow` | Load workflow definition |
| `POST` | `/api/campaigns/:id/test` | Start WebRTC test session |
| `POST` | `/api/calls/test-mode` | WebSocket handshake for test mode |

### Modified Endpoints

| Method | Endpoint | Change |
|--------|----------|--------|
| `POST` | `/api/campaigns` | Accept `telephony_provider` and `workflow_definition` |
| `POST` | `/api/dial/initiate` | Uses `TelephonyProvider` interface; injects CRM data if configured |

---

## 9. Risks & Mitigations

| Risk | Impact | Likelihood | Mitigation |
|------|--------|------------|------------|
| Dograh React components don't cleanly separate from their API | High | Medium | Fork and refactor components before integration; maintain abstraction layer |
| Canvas performance degrades with complex workflows (>50 nodes) | Medium | Low | Implement virtualization; lazy-load node properties |
| WebRTC test mode leaks into production calls | High | Low | Strict `ModeTest` flag checks; separate test transcript DB collection |
| Exotel refactor breaks existing call flows | High | Medium | Comprehensive regression testing; feature flag for new abstraction |
| Ops team resists visual builder | Medium | Low | Parallel support for legacy campaign creation during transition |
| Pre-call CRM fetch latency impacts call initiation | Medium | Medium | 10s timeout with async background fetch; fallback variables pre-loaded |

---

## 10. Success Metrics

| Metric | Baseline | Target | Measurement |
|--------|----------|--------|-------------|
| Campaign creation time | 2 days (engineering) | 30 min (ops self-serve) | Time from request to live campaign |
| Telephony provider integration time | N/A (Exotel hardcoded) | < 1 day per provider | Time to add Vonage/Telnyx |
| Prompt iteration cycle | Hours (requires real call) | Minutes (WebRTC test) | Time to test 3 prompt variants |
| Call personalization rate | 0% | 80% | % of calls using CRM-injected variables |
| Engineering support tickets for campaigns | 5/week | < 1/week | Zendesk/Slack ticket count |
| Platform licensing cost | $0 | $0 | Invoice review |

---

## 11. Dependencies

### External
- Dograh source code (BSD 2-Clause): `github.com/dograh-hq/dograh`
- React Flow library (likely used by Dograh): `reactflow.dev`
- WebRTC adapter: `webrtc-adapter`

### Internal
- `backend/internal/api/campaigns.go`
- `backend/internal/dial/exotel.go`
- `backend/internal/wshandler/handler.go`
- `frontend/src/components/campaigns/CampaignDetail.jsx`
- MongoDB campaigns collection

---

## 12. Rollback Plan

1. **Feature Flags:** All new features gated by feature flags (`workflow_builder_enabled`, `test_mode_enabled`)
2. **Database:** `workflow_definition` is additive; legacy campaigns without it continue working
3. **Code:** Telephony abstraction coexists with old Exotel code via adapter pattern
4. **Deployment:** Canary release on staging environment (`globussoft-crm-staging`)

---

## 13. Open Questions

1. Should we support voice cloning (ElevenLabs) in the workflow builder, or stick to pre-defined voices?
2. Do we need A/B testing capability within the workflow builder (e.g., branch 50/50)?
3. Should the WebRTC test mode support real-time voice (microphone) or typed input only for Phase 1?
4. What's the priority: Twilio integration or visual builder first?
5. Do we want to extract Dograh's QA node for prompt quality scoring?

---

## 14. Appendix: Dograh Reference Materials

- **GitHub:** `github.com/dograh-hq/dograh`
- **Docs:** `docs.dograh.com`
- **License:** BSD 2-Clause (free to modify, embed commercially)
- **Key Files to Study:**
  - `frontend/src/components/WorkflowBuilder/` (canvas, nodes, edges)
  - `backend/internal/telephony/` (provider abstraction)
  - `backend/internal/workflow/` (workflow execution engine)
  - `docs/workflow-schema.md` (workflow_definition JSON spec)

---

**Next Step:** Approve this PRD → Clone Dograh repo → Begin Phase 1 (Week 1).
