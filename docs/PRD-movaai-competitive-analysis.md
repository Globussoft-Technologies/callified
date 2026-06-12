# PRD: MovaAI Competitive Analysis & Feature Parity

**Status:** Draft  
**Author:** AI-Architect (Kimi)  
**Date:** 2026-06-11  
**Context:** Callified / globussoft-ai-dailer vs. MovaAI  
**License Context:** MovaAI is CLOSED SOURCE — this PRD focuses on competitive parity & API integration, not code extraction.

---

## 1. Executive Summary

**MovaAI** (`movaai.in`) is an Indian commercial CPaaS + AI Voice platform that directly competes with our Callified stack. Unlike Dograh (open source, code-borrowable), MovaAI is a **closed-source SaaS** with no public repository. This PRD analyzes MovaAI's feature set, maps it against our current capabilities, and defines a roadmap to achieve **feature parity** where strategically valuable.

**Core Insight:** MovaAI's value proposition is "all communication channels in one platform" (Voice + WhatsApp + SMS + RCS). Our project is currently Voice + WhatsApp + CRM-heavy. The gap is **unified CPaaS orchestration** and **agentic multi-agent workflows**.

---

## 2. MovaAI Feature Breakdown

### 2.1 Product Stack

| MovaAI Layer | Description | Our Equivalent | Gap? |
|--------------|-------------|----------------|------|
| **Agentic AI Workflows** | Autonomous multi-agent orchestration for customer journeys | Campaigns (manual) + Receptionist | ⚠️ Medium Gap |
| **AI Voice Agents** | Inbound IVR, outbound dialer, voice receptionist | ✅ Callified dialer + receptionist | ✅ Parity |
| **WhatsApp Business API** | Official WABA automation, chatbots, templates | ✅ WhatsApp integration exists | ✅ Parity |
| **CPaaS APIs** | Programmable SMS, Voice, messaging with unified billing | ❌ No unified CPaaS layer | 🔴 High Gap |
| **RCS Business Messaging** | Rich interactive messages (carousels, buttons, images) | ❌ Not implemented | 🔴 High Gap |

### 2.2 MovaAI Architecture (Inferred)

```
MovaAI Platform (Closed Source)
├── Agentic Layer
│   ├── Multi-agent orchestration
│   ├── Workflow automation
│   └── 24/7 autonomous operation
├── Communication Layer
│   ├── Voice API (SIP/WebRTC)
│   ├── SMS API
│   ├── WhatsApp Business API
│   └── RCS Messaging API
├── Integration Layer
│   ├── CRM connectors
│   ├── Webhook system
│   └── Developer portal
└── Analytics Layer
    ├── Real-time monitoring
    ├── Call transcripts
    └── Usage analytics
```

---

## 3. Competitive Gap Analysis

### 3.1 Where We Lead Callified ✅

| Capability | Callified | MovaAI | Advantage |
|------------|-----------|--------|-----------|
| **CRM Integrations** | 90+ providers | Unknown (likely limited) | ✅ **We win** |
| **Multi-language AI** | Hindi, Bengali, Marathi + English | English (claimed expandable) | ✅ **We win** |
| **Self-hosted** | Full data control on our servers | SaaS only | ✅ **We win** |
| **Billing engine** | Custom credits + invoicing | Standard SaaS billing | ✅ **We win** |
| **Open-source flexibility** | Can modify anything | Locked platform | ✅ **We win** |
| **TRAI/DND compliance** | Built-in DND scrubbing | Unknown | ✅ **We win** |

### 3.2 Where MovaAI Leads 🔴

| Capability | MovaAI | Callified | Gap Severity |
|------------|--------|-----------|--------------|
| **Unified CPaaS API** | Single API for Voice + SMS + WhatsApp + RCS | Separate systems | 🔴 **High** |
| **RCS Messaging** | Native RCS support | Not implemented | 🔴 **High** |
| **Agentic multi-agent** | Swarm of cooperating AI agents | Single-agent per call | 🔴 **High** |
| **Unified dashboard** | One panel for all channels | Multiple tabs/views | 🟡 **Medium** |
| **Developer portal** | API docs, SDKs, sandbox | Basic API docs | 🟡 **Medium** |
| **Programmable SMS** | First-class SMS API | SMS via WhatsApp only | 🟡 **Medium** |
| **WebRTC inbound** | Browser-based calling | FreeSWITCH only | 🟡 **Medium** |

---

## 4. Strategic Recommendations

### 4.1 Do NOT Integrate with MovaAI

**Rationale:**
- MovaAI is a **competitor**, not a tool
- Closed source = vendor lock-in, exactly what we're trying to avoid
- No API documentation publicly available
- Our self-hosted model is a competitive advantage over SaaS

### 4.2 DO Build Parity Features

Focus on the **3 high-severity gaps** that would make us competitive with MovaAI:

#### Gap 1: Unified CPaaS API Layer
**What:** A single API abstraction for Voice + SMS + WhatsApp

**Implementation:**
```go
// internal/cpaas/cpaas.go
type MessageChannel string

const (
    ChannelVoice    MessageChannel = "voice"
    ChannelSMS      MessageChannel = "sms"
    ChannelWhatsApp MessageChannel = "whatsapp"
    ChannelRCS      MessageChannel = "rcs"
)

type CPaaSMessage struct {
    Channel   MessageChannel    `json:"channel"`
    To        string            `json:"to"`
    From      string            `json:"from"`
    Content   string            `json:"content"`
    Metadata  map[string]string `json:"metadata"`
    CampaignID string           `json:"campaign_id,omitempty"`
}

type CPaaSProvider interface {
    SendMessage(ctx context.Context, msg CPaaSMessage) (*MessageResponse, error)
    GetStatus(ctx context.Context, messageID string) (MessageStatus, error)
    HandleWebhook(w http.ResponseWriter, r *http.Request) error
}
```

**Value:** Customers can orchestrate omnichannel campaigns from one API.

---

#### Gap 2: RCS Business Messaging
**What:** Rich messaging with images, carousels, buttons via RCS (Android's iMessage equivalent)

**Implementation:**
- Integrate with Google RCS Business Messaging API
- Add RCS templates to our WhatsApp template system
- Support rich cards: product carousels, quick-reply buttons, images

**Value:** 2x higher engagement than SMS; required for enterprise BFSI clients.

---

#### Gap 3: Agentic Multi-Agent Workflows
**What:** Multiple AI agents cooperate on a customer journey, not just one voice call

**Implementation:**
```go
// internal/agentic/workflow.go
type AgentRole string

const (
    AgentLeadQualifier AgentRole = "lead_qualifier"
    AgentScheduler     AgentRole = "scheduler"
    AgentCloser        AgentRole = "closer"
    AgentFollowUp      AgentRole = "follow_up"
)

type AgenticWorkflow struct {
    ID          string      `json:"id" bson:"_id"`
    Name        string      `json:"name"`
    Trigger     string      `json:"trigger"` // "lead_created", "appointment_missed"
    Agents      []AgentStep `json:"agents"`
    Conditions  []Condition `json:"conditions"`
    Channels    []string    `json:"channels"` // ["voice", "whatsapp", "sms"]
}

type AgentStep struct {
    Role      AgentRole `json:"role"`
    Channel   string    `json:"channel"`
    Prompt    string    `json:"prompt"`
    Timeout   int       `json:"timeout_sec"`
    HandoffTo string    `json:"handoff_to,omitempty"` // next agent
}
```

**Example Workflow:**
1. **Lead Qualifier Agent** (Voice) → Calls new lead, qualifies interest
2. If interested → **Scheduler Agent** (WhatsApp) → Sends booking link
3. If booked → **Closer Agent** (Voice) → Confirms details, collects payment info
4. If no-show → **FollowUp Agent** (SMS + WhatsApp) → Sends reminder + rebooking

**Value:** End-to-end automation without human handoff until necessary.

---

## 5. Implementation Roadmap

### Phase 1: CPaaS Unification (Weeks 1-3)
- [ ] Define `CPaaSProvider` interface
- [ ] Refactor WhatsApp sender to implement `CPaaSProvider`
- [ ] Add SMS provider adapter (Twilio SMS / Exotel SMS)
- [ ] Build unified `/api/v2/messages/send` endpoint
- [ ] Update campaign model to support multi-channel sequences

### Phase 2: RCS Messaging (Weeks 4-5)
- [ ] Apply for Google RCS Business Messaging partner account
- [ ] Build RCS adapter implementing `CPaaSProvider`
- [ ] Add RCS template builder UI
- [ ] Test with Android devices

### Phase 3: Agentic Workflows (Weeks 6-8)
- [ ] Design agentic workflow schema
- [ ] Build workflow engine (state machine)
- [ ] Add agent handoff logic between channels
- [ ] Build workflow designer UI (extends Dograh PRD workflow builder)
- [ ] Test end-to-end: Voice → WhatsApp → SMS → Voice

### Phase 4: Unified Dashboard (Weeks 9-10)
- [ ] Single analytics view across Voice + SMS + WhatsApp + RCS
- [ ] Cross-channel customer journey timeline
- [ ] Unified billing/credits system

---

## 6. API Design

### Unified Send API

```http
POST /api/v2/messages/send
Content-Type: application/json

{
  "channel": "whatsapp",
  "to": "+919876543210",
  "from": "+911234567890",
  "content": {
    "type": "template",
    "template_name": "appointment_reminder",
    "language": "hi",
    "params": {
      "customer_name": "Rahul",
      "appointment_time": "3:00 PM"
    }
  },
  "metadata": {
    "campaign_id": "camp_123",
    "lead_id": "lead_456"
  }
}
```

### Multi-Channel Campaign API

```http
POST /api/v2/campaigns/omnichannel
Content-Type: application/json

{
  "name": "Insurance Renewal Sequence",
  "steps": [
    {
      "order": 1,
      "channel": "voice",
      "delay_minutes": 0,
      "content": "Hello {{name}}, your policy expires in 7 days..."
    },
    {
      "order": 2,
      "channel": "whatsapp",
      "delay_minutes": 30,
      "condition": "not_interested",
      "content": "Hi {{name}}, here's a quick renewal link: {{link}}"
    },
    {
      "order": 3,
      "channel": "sms",
      "delay_minutes": 1440,
      "condition": "no_response",
      "content": "Last chance to renew! Call us at 1800-XXX. -Globussoft"
    }
  ]
}
```

---

## 7. Database Schema Changes

### MongoDB: New `omnichannel_campaigns` collection

```json
{
  "_id": "...",
  "name": "Insurance Renewal",
  "type": "omnichannel_sequence",
  "steps": [
    {
      "order": 1,
      "channel": "voice",
      "delay_minutes": 0,
      "template_id": "voice_reminder_hi",
      "condition": null
    },
    {
      "order": 2,
      "channel": "whatsapp",
      "delay_minutes": 30,
      "template_id": "wa_renewal_link",
      "condition": "voice_not_interested"
    },
    {
      "order": 3,
      "channel": "sms",
      "delay_minutes": 1440,
      "template_id": "sms_final_reminder",
      "condition": "whatsapp_no_response"
    }
  ],
  "status": "active",
  "created_at": "..."
}
```

### MongoDB: New `message_logs` collection (unified)

```json
{
  "_id": "...",
  "campaign_id": "...",
  "lead_id": "...",
  "channel": "whatsapp",
  "direction": "outbound",
  "status": "delivered",
  "content": "...",
  "provider": "twilio",
  "provider_message_id": "SMxxxxx",
  "cost_inr": 0.25,
  "created_at": "...",
  "delivered_at": "..."
}
```

---

## 8. Cost Impact

| Feature | Cost | Notes |
|---------|------|-------|
| CPaaS Unification | $0 | Internal refactoring |
| SMS API (Twilio/Exotel) | ₹0.15-0.30/SMS | Usage-based |
| RCS Messaging | ₹0.50-2.00/message | Google RCS partner pricing |
| Agentic Workflows | $0 | Internal engine |
| Engineering Time | ~8 weeks | 2 engineers |

**Total monthly cost increase: $0** (pay only for actual SMS/RCS usage)  
**Competitive impact: High** — matches MovaAI's core value prop

---

## 9. Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| RCS requires Google partner approval | Medium | Apply early; fallback to SMS if denied |
| Multi-channel complexity | Medium | Phase rollout: WhatsApp → SMS → RCS |
| Customer confusion (too many channels) | Low | Smart defaults; ops team training |
| Engineering bandwidth | High | Defer non-critical features; use Dograh PRD UI components |

---

## 10. Success Metrics

| Metric | Baseline | Target | Timeline |
|--------|----------|--------|----------|
| Channels supported | 2 (Voice, WhatsApp) | 4 (+SMS, +RCS) | 8 weeks |
| Omnichannel campaigns | 0 | 10+ live campaigns | 10 weeks |
| Customer retention | Current | +15% (more sticky) | 12 weeks |
| Enterprise deals closed | Current | +3 (RCS is enterprise requirement) | 16 weeks |

---

## 11. Conclusion

**MovaAI is a competitor, not a tool.** We cannot integrate with it (closed source) and should not (SaaS lock-in). Instead, we should **build parity** on the 3 features that make MovaAI attractive:

1. **Unified CPaaS API** — One API for Voice + SMS + WhatsApp + RCS
2. **RCS Messaging** — Required for enterprise BFSI/retail clients
3. **Agentic Workflows** — Multi-agent, multi-channel customer journeys

These features leverage our existing advantages (90+ CRMs, multi-language, self-hosted) while closing the gap on MovaAI's "unified platform" narrative.

---

**Next Step:** Approve this PRD → Begin Phase 1 (CPaaS Unification) in parallel with Dograh workflow builder work.
