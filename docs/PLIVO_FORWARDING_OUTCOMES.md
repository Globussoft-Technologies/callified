# Plivo Forwarding Outcomes

Forwarding XML now configures two POST endpoints automatically:

- `/webhook/plivo/inbound/dial-action`: final recipient status; returns Hangup XML.
- `/webhook/plivo/inbound/dial-event`: answer, connection and hangup notifications.

Both endpoints require Plivo V3 signatures covering the complete URL and form.
The signed account, parent A-leg UUID, saved routing snapshot and (when supplied)
forwarded destination must agree. There is no unsigned development bypass.

Incoming call status and recipient outcome are independent. The incoming history
API retains `status` for the parent incoming call and adds `forwarded_outcome`:
`answered`, `busy`, `no_answer`, `failed`, `cancelled`, or `unknown`.
Timeouts map to `no_answer`. Legacy calls without callbacks remain `unknown`;
the parent ending successfully is not treated as evidence of an answer.

Outcomes are persisted in `incoming_forwarding_outcomes`. Repeated callbacks
update the same organization-scoped row, not the incoming call log. A final
action takes precedence over provisional hangup notifications, and explicit
answer evidence is preserved against late failure/cancellation notifications.
The first final action is retained on retries. Callback handlers do not charge
credits; the existing signed incoming hangup callback remains the sole billing
path for forwarding, avoiding additional deductions for the same call.

The backend startup schema check creates the table, or apply
`backend/scripts/migrations/20261116_incoming_forwarding_outcomes.sql` before
deployment. Existing Plivo Answer/Hangup URLs remain unchanged. Both new paths
must reach the backend through the reverse proxy's `/webhook/` route.

## Verification Before Production

On a staging provider account, verify answered, busy, unanswered, failed and
caller-cancelled forwarding. Check callback delivery in both orders, duplicate
delivery, invalid signatures, destination mismatches and history output. Confirm
one charge per incoming call and an independent recipient outcome. Real provider
calls are not made by the automated suite.

An opt-in MySQL ordering test is available with `CALLIFIED_TEST_MYSQL_DSN` set
to a disposable server: `go test ./internal/db -run TestForwardingOutcomeOrderingMySQL -count=1`.
It creates and drops its own schema, so the test account needs those privileges.

Provider references:
- https://www.plivo.com/docs/voice/xml/routing
- https://www.plivo.com/docs/voice/concepts/signature-validation
