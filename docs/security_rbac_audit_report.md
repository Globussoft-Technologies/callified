# Security / RBAC Audit Report

> Generated: 2026-07-20  
> Status: code changes applied locally; **not deployed** to `testgo2` or `app`.

Build/test status at the time of this report:

```bash
go build ./cmd/audiod
go vet ./...
go test ./...
```

All green.

---

## What was hardened in this pass

| Fix | Location |
|---|---|
| Single-lead dial (`/api/dial/{lead_id}`) scoped by `canAccessLead` | `backend/internal/api/dial.go:49` |
| Bulk dial-all / redial-failed now require `requireCampaignView` | `backend/internal/api/dial.go:192,318` |
| `requireCampaignView` enforces `campaign.OrgID == ac.OrgID` for every role, closing the Admin/SuperAdmin bypass | `backend/internal/api/campaigns.go:1424` |
| `assignCampaignUsers` deduplicates IDs and verifies every `user_id` belongs to the org | `backend/internal/api/campaigns.go:1568–1617` |
| `setCampaignExecutives` verifies campaign ownership and executive IDs belong to the org | `backend/internal/api/executives.go:107–144` |
| `createScheduledCall` uses `requireLeadAccess` and validates `executive_id` by org | `backend/internal/api/scheduled_calls.go:90–140` |
| WaSender session proxy validates session `id` against `^[A-Za-z0-9_-]+$` | `backend/internal/api/wa_session.go:18,126,153,180` |
| Signup ignores client-supplied `role` and forces `Admin` | `backend/internal/api/auth.go:77–82` |
| `requireAuth` rejects `kind="sse"` tokens as Bearer tokens and re-checks `users.is_active` | `backend/internal/api/middleware.go:62–73` |
| CRM integrations list loads only the caller’s org | `backend/internal/api/integrations.go:23` |
| External transcript export restricted to Admin JWT or org-scoped API keys | `backend/internal/api/external_transcripts.go:59–68` |
| Knowledge download re-bases filename with `filepath.Base` | `backend/internal/api/knowledge.go:174` |
| SSO org resolution requires the explicit `org_id` claim to exist before JIT-creating a user | `backend/internal/api/sso.go:371–380` |
| WhatsApp messages in interaction timeline scoped by `c.org_id` | `backend/internal/db/interactions.go:168` |
| Transcript review / conclusion endpoints use lead-level isolation | `backend/internal/api/campaigns.go:1528–1549`, `backend/internal/api/leads.go:1256–1294` |
| WA campaign blast `send-one` scoped by campaign org and lead access | `backend/internal/api/wa_blast.go:239–299` |

---

## Remaining potential issues

### 🔴 High

1. **WebSocket endpoints are unauthenticated**
   - `/media-stream`, `/ws/sandbox`, `/ws/monitor/{key}`, `/ws/agent`
   - `backend/cmd/audiod/main.go:76–88`, `backend/internal/wshandler/handler.go:86`, `backend/internal/wshandler/monitor.go:148`, `backend/internal/wshandler/bridge.go:124`
   - Anyone who obtains a `stream_sid`/`call_sid` can listen to live audio, inject whispers, or take over a call.

2. **Inbound WhatsApp webhooks accept unsigned requests**
   - `POST /wa/webhook/gupshup`, `/wa/webhook/wati`, `/wa/webhook/aisensei`, `/wa/webhook/interakt`
   - `backend/internal/api/wa_webhooks.go:25–60`, `handleWAWebhook:157–167`
   - No signature/token validation; an attacker can inject arbitrary inbound messages/calls.

3. **`POST /api/billing/subscribe` activates paid plans without payment verification**
   - `backend/internal/api/billing.go:94–119`
   - In production this lets any Admin activate a paid plan for free. It should be removed or guarded by an explicit dev/test env flag.

4. **`POST /api/billing/webhook` skips HMAC when Razorpay secret is empty**
   - `backend/internal/api/billing.go:345–359`
   - If `RazorpayWebhookSecret` is unset, forged payment events are accepted.

5. **`GET /api/debug/last-dial` is global, not org-scoped**
   - `backend/internal/api/misc.go:656–670`
   - Returns the most recent dial metadata across all tenants.

6. **`GET /api/demo-requests` returns all demo requests globally**
   - `backend/internal/api/misc.go:564–574`
   - Any authenticated user can read every tenant’s demo-request PII.

7. **`/api/pronunciation` is global (no `org_id`)**
   - `backend/internal/api/misc.go:119–219`, `backend/internal/db/orgs.go:536–589`
   - Any Admin can view/edit/delete pronunciation rules for all tenants.

### 🟡 Medium

8. **`POST /wa/webhook/wasender` falls back to “accept anything”**
   - `backend/internal/api/wa_webhooks.go:89–97`
   - When no webhook secret is stored, it accepts any request and uses a non-constant-time comparison.

9. **`POST /wa/webhook/meta` skips signature verification when `MetaAppSecret` is empty**
   - `backend/internal/api/wa_webhooks.go:131–138`
   - Same “dev mode bypass” risk as above.

10. **`POST /webhook/twilio/voice` builds TwiML from unvalidated form params without verifying Twilio signature**
    - `backend/internal/api/twilio_browser.go:122–155`
    - An attacker can make the endpoint dial arbitrary numbers on your Twilio account.

11. **API key appears in URL query strings**
    - `GET /api/auth/sso/api-key?api_key=...` and `GET /api/auth/token?api_key=...`
    - `backend/internal/api/sso.go:255–288`
    - Keys end up in access logs, browser history, and Referer headers.

12. **SSO re-syncs existing user’s role from external JWT**
    - `backend/internal/api/sso.go:168–183`
    - If the external SSO issuer is compromised, a user can be promoted to Admin.

13. **`POST /api/upload-recording` can create transcripts with `org_id = 0/NULL`**
    - `backend/internal/api/misc.go:309–484` (see `SaveCallTranscript` fallback at line 476)
    - When `lead_id` is missing, the fallback inserts `org_id=0`, which can bypass org-scoped recording authorization.

14. **Lead documents are stored in a shared `docs/` directory with original filenames**
    - `backend/internal/api/leads.go:876–920`
    - No org segregation, no UUID renaming, and the filename is used directly in the URL/path. Path-traversal sanitization is missing.

15. **DND check endpoints are `auth`-gated, not Admin-only**
    - `GET /api/dnd/check`, `GET /api/dnd/check/{phone}`
    - `backend/internal/api/server.go:317–318`, `backend/internal/api/dnd.go:224–267`
    - Swagger claims Admin-only, but any logged-in user can query them.

16. **Analytics exports are org-scoped but not lead-assignment scoped**
    - `/api/analytics/export`, `/api/analytics/report`, `/api/analytics/agent-report`
    - `backend/internal/api/analytics.go:69–418`
    - Admins see all leads; if Team Leaders/Agents should see only their assigned leads here, additional filtering is needed.

### 🟢 Low / design-level

17. **Public trial signup has no CAPTCHA/rate-limit and returns the plaintext password**
    - `backend/internal/api/misc.go:823–919`
    - Enables automated org/user provisioning; credentials appear in response bodies/logs.

18. **API keys are SHA-256 hashed without salt**
    - `backend/internal/db/api_keys.go:23–32`
    - A leaked DB hash can be attacked with rainbow tables.

19. **SSE tickets still travel in the URL (`?ticket=...`)**
    - Mitigated by rejecting SSE tickets as Bearer tokens (`backend/internal/api/middleware.go:62–64`), but proxies may still log them.

20. **Product images remain public and not org-scoped**
    - By design, but worth noting if tenants expect private product assets.

---

## Recommended next steps

1. **Authenticate WebSocket upgrades** — pass a short-lived JWT/ticket in the handshake query, validate it before `upgrader.Upgrade`, and reject cross-org stream/agent access.
2. **Sign all inbound WA webhooks** — require provider secrets in production, validate signatures, and use constant-time comparison.
3. **Remove or env-gate `POST /api/billing/subscribe`**; do not expose a no-payment plan-activation endpoint on production.
4. **Scope by org**: add `org_id` to `pronunciation_guide`, `debug_last_dial`, and `demo_requests`; update their handlers/DB queries.
5. **Move API keys out of URLs** — use `Authorization: Bearer <api_key>` or a POST body for SSO/token flows.
6. **Secure file uploads**: store lead docs under `org_id/` subdirectories with UUID filenames and enforce org-scoped download authorization.
7. **Add rate-limit/CAPTCHA to trial signup** and stop returning the generated password in the JSON response.
8. **Upgrade API key hashing** to bcrypt/Argon2 with a key-rotation workflow.
9. **Run end-to-end tests on `testgo2.callified.ai`** before promoting to `app.callified.ai`.
