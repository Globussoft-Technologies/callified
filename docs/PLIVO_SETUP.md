# Plivo Outbound AI Calling

## Configure

1. In Provider Accounts, add an outbound Plivo account.
2. Enter the Plivo Auth ID, Auth Token, and purchased voice-enabled caller number.
3. Select this provider account on a voice campaign and configure the AI voice as usual.
4. Place a test AI call to a permitted destination before starting a campaign.

Use international phone numbers including the country code. Account permissions,
trial restrictions, balance, and destination permissions must permit the call.

The server's `PUBLIC_SERVER_URL` must be its externally reachable HTTPS origin.
The reverse proxy must forward these paths to the Go service:

- `POST /webhook/plivo/answer`
- `POST /webhook/plivo/status`
- WebSocket `/media-stream/plivo`

The outbound API supplies answer and status URLs automatically. No inbound Plivo
application routing is implemented by this change.

## Behavior

An application call ID and its log/context are persisted before dialing. This
same ID links status updates, recordings, reviews, billing and monitoring.
Plivo's request UUID is retained for canceling unanswered calls; its actual
call UUID is retained for controlling answered calls. The answer webhook loads
the pre-existing context and marks the call in progress before returning
bidirectional 8 kHz mu-law streaming XML. Audio uses the existing AI pipeline.
Playback interruption uses Plivo's `clearAudio` event. Hangup supports both
pending requests and answered calls.

Webhooks require Plivo V3 signatures using the selected account's Auth Token.
Media connections require a short-lived signed ticket and a one-use nonce.
Provider credentials are not placed in callback URLs.

Credentials reuse the provider-account schema: `account_sid` stores Auth ID,
`api_token` stores Auth Token, and `caller_id` stores the purchased number.

Inbound calls and human/browser bridging are not supported for Plivo yet.
Automated tests mock provider requests; a real carrier call still needs to be
verified with the purchased number after deployment.
