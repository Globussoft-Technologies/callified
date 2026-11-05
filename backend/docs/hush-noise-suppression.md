# Hush input audio processing

Hush is implemented as an optional private model service for Gemini Live. It is
**disabled by default** (`HUSH_ENABLED=false`) so installing this change cannot
silently change existing customer calls. Deployment and activation are separate
steps from installing this code.

## Audio path and voice preservation

All Gemini Live carrier input reaches the same client after the existing PCM /
mu-law decoding and original recording. Both normal and prewarmed calls pass
Hush settings to that client. One Hush WebSocket and one native model session
are used per call. Model state never crosses calls.

The upstream Weya native runtime accepts the actual 8 kHz PCM16LE telephone
stream and resamples internally to its 16 kHz model. The service returns 8 kHz
PCM16LE. The model/runtime are pinned by commit and SHA-256 in
`hush_service/download.py`; downloads occur only while building the image.
The license is bundled at `/opt/hush/LICENSE`.

The starting suppression limit is **6 dB**, rather than upstream's unlimited
100 dB setting. `HUSH_ATTENUATION_DB` permits 0–18 dB; higher suppression can
remove more background speech but increases the risk of altering a quiet caller.
There is no extra volume normalization, noise gate, speaker enrollment, or VAD
threshold. Every complete processed frame is forwarded, including quiet speech
and silence. Existing Gemini turn detection and barge-in settings are unchanged.
The model introduces its own small streaming delay; it is not a zero-delay path.

No single-channel model can promise the customer's voice will be unchanged or
that every competing speaker will be removed. Hush can mistake a loud nearby
speaker for the intended speaker. Preserving quiet signal energy in tests does
not prove unchanged words or identity. Evaluate actual recordings before enabling.

## Fail-open behavior

A missing/unready model service leaves the caller's original audio flowing.
An invalid response, socket failure or 500 ms inference timeout switches that
call to raw passthrough. The backend retains original bytes until their processed
response is validated, so even a buffered partial frame is replayed on failure.
The fallback is sticky for the remainder of that call and logged once.

An independent socket reader answers ping/pong while a prewarmed call rings.
There is no fixed call-duration limit or local silence cutoff. Partial model
frames are buffered across arbitrary carrier chunks. A final incomplete PCM
frame is sent raw when the input channel closes normally. Native analysis /
synthesis tail is not flushed after a hangup; live speech normally includes
following silence that drains it.

## Run locally with Docker Compose

The released native runtime supports Linux x86_64. The Hush service is pinned
to `linux/amd64`; ARM hosts need Docker emulation. The Go backend keeps its
existing pure-Go build and architecture support.

```sh
docker compose --profile hush up -d --build --wait hush
```

Then set these entries in `backend/.env`:

```dotenv
HUSH_ENABLED=true
HUSH_URL=ws://hush:8003/process
```

Recreate the backend only after the model is healthy:

```sh
docker compose up -d --no-deps --build go-audio
```

To bypass Hush, set `HUSH_ENABLED=false` and restart only the backend. To adjust
strength, set `HUSH_ATTENUATION_DB` in the Compose environment (default 6), then
recreate the Hush service during an idle period. For example:

```sh
HUSH_ATTENUATION_DB=6 docker compose --profile hush up -d --no-deps --build --wait hush
```

The model service publishes no host port and requires no Nginx change. It runs
non-root with a read-only filesystem, bounded memory/CPU, and at most 16 calls.
Load-test expected concurrency on the target CPU before increasing this cap.

For a native/systemd backend, expose the model on a dedicated **localhost-only**
port and set `HUSH_URL` to that address. Do not expose raw-audio processing to
the public Internet. On testgo1, Hush was deployed separately and enabled for the
target backend. The older RNNoise/Silero services remain available for rollback
and are not part of the active Hush input path.

## Validation

`go test -race ./... -timeout 60s` covers raw-audio preservation across partial
chunks, invalid output, disconnection, idle ping/pong, and a real two-socket
Hush-to-Gemini protocol test. The Gemini endpoint is mocked; no paid call occurs.

The Hush Docker build runs actual native inference tests for quiet telephone-band
signals, streaming length/chunk consistency, silence, noise attenuation, and
concurrent-call isolation. It also runs the actual ASGI HTTP/WebSocket service.
These are integration checks, not speech-recognition accuracy guarantees.

Before enabling for users, compare raw and enhanced recordings containing quiet
customer speech, brief answers, nearby voices, fan/traffic noise and overlapping
speakers. Check word preservation, intelligibility, false interruptions, added
latency, and sustained calls longer than three minutes. Keep the originals for
comparison and use the bypass if the customer's speech is degraded.

Upstream references:
- https://huggingface.co/weya-ai/hush
- https://github.com/pulp-vision/Hush/tree/main/deployment
