# Authenticated Tata dynamic streaming

This change requires coordination with Tata dashboard configuration. It is not
backward compatible with static `/media-stream/tata?...org_id=...` URLs.
Do not deploy it to production until the customer has Dynamic endpoint support.

## Setup

1. Apply `backend/scripts/migrations/20261029_tata_stream_authentication.sql`
   and `backend/scripts/migrations/20261029_tata_stream_verification.sql`
   once, or let the backend startup schema check add the columns.
2. Redis must be available. Ticket authentication deliberately has no in-memory
   fallback. Redis 6.2 or later is required for atomic GETDEL.
3. Set PUBLIC_SERVER_URL to the public HTTPS origin.
4. In Provider Accounts, choose the relevant organization-level Tata account and
   click **Generate stream secret**. Copy the secret immediately; only its SHA-256
   hash is stored. Generating a replacement invalidates the previous secret and resets outbound
   readiness. Save the new header in Tata and run its endpoint test. The first
   authenticated dynamic request verifies header possession even if the dashboard
   supplies sample call details that fail routing validation. Use **Check
   verification** in Callified to see the status. This cannot detect a later
   change made only in the Tata dashboard; it is not a full routing check.
5. In Smartflo: Settings → Channels → Voice Bot → Add an Endpoint. Select Dynamic,
   POST, and the URL displayed by Callified (`/webhook/tata/stream-url`). Configure
   Content-Type: application/json and X-Callified-Secret with that account's secret.
6. Map the JSON body:
   ```json
   {"callId":"$callId","fromNumber":"$fromNumber","toNumber":"$toNumber","status":"$status"}
   ```
7. Select this endpoint in the relevant Tata call routing. Keep the streaming
   codec at 8 kHz mu-law. Do not configure org_id or direction in a static URL.

The endpoint returns exactly `success` and `wss_url`. Its database lookup and
Redis operations use a 1.5-second context budget; Tata's external deadline is
2 seconds, so measure end-to-end timing in the customer environment.

## Ownership and lifecycle

The secret selects a saved organization-level account. For inbound calls the
called number must match that account's DID. For outbound calls the calling
number must match the DID and the destination must match a pending dial reserved
by that same account and organization. Account DID comparisons strip formatting without guessing country codes.
Pending outbound destination keys use the same Indian domestic normalization
as the dialer, so a stored 10-digit lead matches Tata's +91 number. Configure
account DIDs in full international form.

Outbound reservations expire in 2 minutes. Simultaneous dials to the same phone
through the same account are rejected instead of ambiguously sharing context.
Different destinations and accounts can proceed concurrently. Reservations carry unique IDs. Late updates and failure cleanup cannot modify a
newer reservation. Ticket issuance binds the reservation atomically, without
deleting it; the reservation is released when its stream ticket is consumed.

Each 256-bit opaque ticket expires after 45 seconds and is atomically consumed
before WebSocket upgrade. Invalid, expired and reused tickets are rejected.
Repeated dynamic requests for the same account and callId return the identical
unconsumed ticket during its 45-second lifetime. Once consumed or expired,
requests cannot mint another ticket for that call for five minutes. This supports
HTTP response retries without weakening WebSocket single-use protection.
The verified context supplies organization, campaign, lead and direction. URL
parameters and start-event metadata cannot override ownership. The start event
must arrive within 10 seconds and its carrier call identity must equal callId.
The expected start field is callSid/call_id/callId (not the REST ref_id).
Optional from/to fields must also match. Confirm these exact variable semantics
with a live test; a mismatch intentionally fails closed.

Secrets are never returned by account-list APIs. Previously issued tickets may
remain valid for their remaining 45-second lifetime after secret rotation.
Do not log or share the one-time secret or ticket-bearing URLs.

## Rollout checks

Automated tests cover Redis expiry, single use under concurrent requests,
account isolation, unavailable Redis, missing authentication, start identity
mismatches and rejection of legacy Tata URLs. Before production, verify inbound
and outbound live calls, dynamic request variable mappings, call cancellation,
concurrent calls, secret rotation, latency, codec and carrier retry behavior on
testgo1. No live carrier calls are placed by the test suite.
