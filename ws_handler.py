"""
ws_handler.py — WebSocket media stream handler for Callified AI Dialer.
Manages Deepgram STT, LLM pipeline, greeting, barge-in, and call recording.
"""
import os
import json
import base64
import asyncio
import logging
import uuid as _uuid
import time
import httpx
from fastapi import WebSocket
from deepgram import DeepgramClient, LiveTranscriptionEvents, LiveOptions
from google import genai
from google.genai import types
import call_logger
from database import (
    get_pronunciation_context, get_product_knowledge_context,
    get_org_custom_prompt, get_org_voice_settings, save_call_transcript,
    get_conn,
)
from tts import synthesize_and_send_audio, _tts_recording_buffers
import redis_store
from prompt_builder import build_call_context
from recording_service import save_call_recording_and_transcript
from billing import record_usage

# ─── Shared State ────────────────────────────────────────────────────────────
# Non-serializable state stays in-memory (asyncio.Task, WebSocket connections)
active_tts_tasks = {}
monitor_connections: dict[str, set[WebSocket]] = {}
twilio_websockets: dict[str, WebSocket] = {}

# ─── Language Detection ───────────────────────────────────────────────────────
def _detect_script_language(text: str):
    """Detect spoken language from Unicode script ranges."""
    counts = {
        'bn': sum(1 for c in text if '\u0980' <= c <= '\u09FF'),
        'hi': sum(1 for c in text if '\u0900' <= c <= '\u097F'),
        'en': sum(1 for c in text if c.isascii() and c.isalpha()),
        'ta': sum(1 for c in text if '\u0B80' <= c <= '\u0BFF'),
        'te': sum(1 for c in text if '\u0C00' <= c <= '\u0C7F'),
        'kn': sum(1 for c in text if '\u0C80' <= c <= '\u0CFF'),
        'ml': sum(1 for c in text if '\u0D00' <= c <= '\u0D7F'),
        'gu': sum(1 for c in text if '\u0A80' <= c <= '\u0AFF'),
        'pa': sum(1 for c in text if '\u0A00' <= c <= '\u0A7F'),
    }
    if sum(counts.values()) < 3:
        return None
    best = max(counts, key=counts.get)
    return best if counts[best] > 0 else None

# Words that appear across languages and should NOT trigger a language switch.
# e.g. "হ্যালো" is just "hello" in Bengali script — not a Bengali sentence.
_LANG_NEUTRAL_WORDS: frozenset = frozenset({
    # Latin / romanised
    'hello', 'hi', 'hey', 'helo', 'hallo',
    'ok', 'okay', 'okie',
    'hmm', 'hm', 'umm', 'um', 'uh', 'ah', 'oh',
    'yes', 'no', 'ya', 'yaa', 'yah',
    'bye', 'goodbye',
    'haan', 'han', 'ha', 'ji',
    'accha', 'achha', 'acha',
    'thanks', 'thank',
    # Bengali script
    'হ্যালো',   # hello
    'হ্যাঁ',    # yes
    'হাঁ',      # yes
    'ঠিক',      # right/okay
    'ওকে',      # okay
    # Devanagari
    'हाँ', 'हां', 'हा',
    'ओके', 'ओक',
    'जी',
    'अच्छा',
})

def _is_lang_neutral(text: str) -> bool:
    """Return True if the entire utterance is only language-neutral filler/greeting words."""
    stripped = text.strip().rstrip('.,!?।')
    if stripped in _LANG_NEUTRAL_WORDS:
        return True
    words = stripped.split()
    return bool(words) and all(
        w.strip('.,!?।') in _LANG_NEUTRAL_WORDS or w.strip('.,!?।').lower() in _LANG_NEUTRAL_WORDS
        for w in words
    )

# Explicit language-switch command keywords (Latin script).
# Detected BEFORE any other logic — even a single matching word triggers a switch.
# Handles cases like: "hindi me bolo", "speak in english", "hindi hello"
# where the STT may only partially capture the phrase but still includes the keyword.
_LANG_INTENT_MAP: dict[str, list[str]] = {
    'hi': ['hindi', 'hindi me', 'hindi mein', 'hindi bolo', 'hindi me baat'],
    'bn': ['bengali', 'bangla', 'bangla te', 'bangla bolo'],
    'en': ['english', 'speak english', 'in english', 'english me', 'english mein'],
    'mr': ['marathi', 'marathi me', 'marathi mein'],
    'ta': ['tamil', 'tamil la', 'tamil mein', 'tamizh'],
    'te': ['telugu', 'telugu lo', 'telugu mein'],
    'kn': ['kannada', 'kannada alli', 'kannada mein'],
    'ml': ['malayalam', 'malayalam il', 'malayalam mein'],
    'gu': ['gujarati', 'gujarati ma', 'gujarati mein'],
    'pa': ['punjabi', 'punjabi mein', 'punjabi vich'],
}

def _detect_lang_intent(text: str):
    """
    Scan STT text for an explicit language-switch command word.
    Returns language code ('hi', 'bn', 'en', 'mr') or None.
    Checked FIRST — overrides neutral-word guard and script detection.
    """
    lower = text.lower()
    for lang, keywords in _LANG_INTENT_MAP.items():
        for kw in keywords:
            if kw in lower:
                return lang
    return None

_LANG_LABEL = {
    'hi': 'Hindi', 'en': 'English', 'bn': 'Bengali', 'mr': 'Marathi',
    'ta': 'Tamil', 'te': 'Telugu', 'kn': 'Kannada', 'ml': 'Malayalam',
    'gu': 'Gujarati', 'pa': 'Punjabi',
}

# ─── Voicemail Detection ──────────────────────────────────────────────────────
# Only phrases that appear in automated voicemail systems — NOT in human speech.
# "call back later" / "try again later" removed: customers commonly say these
# in normal conversation ("baad mein call karo") and trigger false positives.
_VOICEMAIL_PHRASES = (
    "not available", "unavailable", "unable to take your call",
    "please leave a message", "leave a message", "leave your message",
    "please record your message", "record your message",
    "after the beep", "after the tone", "at the tone",
    "you have reached", "you've reached",
    "the number you have dialed", "the person you are trying",
    "this mailbox", "voicemail", "voice mail",
    # Hindi voicemail phrases
    "abhi uplabdh nahi", "sandesh chhodein", "beep ke baad",
    "is samay upalabdh nahi",
)

def _is_voicemail(text: str, call_age_s: float = 0) -> bool:
    # Ignore the first 8 seconds — early STT noise / echo can contain false phrases.
    if call_age_s < 8:
        return False
    lower = text.lower()
    return any(phrase in lower for phrase in _VOICEMAIL_PHRASES)

# Serializable state backed by Redis (falls back to in-memory if Redis unavailable)
# Access via redis_store.get_pending_call(), redis_store.get_takeover(), etc.
# Legacy aliases kept for backward-compat in main.py dial functions:
pending_call_info = {}

# SDK clients (initialized lazily)
dg_client = None
llm_client = None

# ─── WebSocket Handler ──────────────────────────────────────────────────────

async def handle_media_stream(websocket: WebSocket):
    global dg_client, llm_client
    await websocket.accept()

    # Try query params first, then fall back to pending_call_info from dial
    lead_name = websocket.query_params.get("name", "") or ""
    interest = websocket.query_params.get("interest", "") or ""
    lead_phone = websocket.query_params.get("phone", "") or ""
    _call_lead_id = None
    _campaign_id = None
    _qp_lead_id = websocket.query_params.get("lead_id", "")
    if _qp_lead_id and _qp_lead_id.isdigit():
        _call_lead_id = int(_qp_lead_id)
    _tts_provider_override = websocket.query_params.get("tts_provider", None) or None
    _tts_voice_override = websocket.query_params.get("voice", None) or None
    _tts_language_override = websocket.query_params.get("tts_language", None) or None
    _lang_explicitly_set = bool(_tts_language_override)

    # Check Redis lead-voice cache — ensures same agent voice for repeat calls to same lead
    _redis_voice_key = f"lead_voice:{_call_lead_id}" if _call_lead_id else None
    if _redis_voice_key and not _tts_voice_override:
        try:
            _cached_voice = redis_store.get_raw(_redis_voice_key)
            if _cached_voice:
                _cv = json.loads(_cached_voice)
                _tts_voice_override = _cv.get("voice_id")
                _tts_provider_override = _cv.get("provider") or _tts_provider_override
                _tts_language_override = _tts_language_override or _cv.get("language")
        except Exception:
            pass

    # Check Redis pending call for campaign voice overrides (Exotel dial flow).
    # Always read pending call — campaign language must win over the lead-voice cache.
    _pending_voice = redis_store.get_pending_call("latest")
    if not _tts_voice_override:
        if _pending_voice.get("tts_provider"):
            _tts_provider_override = _pending_voice["tts_provider"]
        if _pending_voice.get("tts_voice_id"):
            _tts_voice_override = _pending_voice["tts_voice_id"]
    # Language from pending call wins over lead-voice cache but NOT over an explicit query param.
    # Sandbox (and any direct WebSocket caller) sets tts_language via query param — preserve it.
    if _pending_voice.get("tts_language") and not _lang_explicitly_set:
        _tts_language_override = _pending_voice["tts_language"]
        _lang_explicitly_set = True

    # If still no voice override, look up org voice settings from DB
    if not _tts_voice_override:
        try:
            _vc = get_conn()
            _vcur = _vc.cursor()
            _org_for_voice = None
            if _call_lead_id:
                _vcur.execute("SELECT org_id FROM leads WHERE id = %s", (_call_lead_id,))
                _lr = _vcur.fetchone()
                if _lr and _lr.get('org_id'):
                    _org_for_voice = _lr['org_id']
            if not _org_for_voice:
                _vcur.execute("SELECT org_id FROM users LIMIT 1")
                _ur = _vcur.fetchone()
                if _ur:
                    _org_for_voice = _ur.get('org_id')
            _vc.close()
            if _org_for_voice:
                _vs = get_org_voice_settings(_org_for_voice)
                if _vs.get('tts_voice_id'):
                    _tts_voice_override = _vs['tts_voice_id']
                    _tts_provider_override = _vs.get('tts_provider', 'elevenlabs')
                if not _tts_language_override and _vs.get('tts_language'):
                    _tts_language_override = _vs['tts_language']
                    _lang_explicitly_set = True
                elif not _tts_language_override:
                    _tts_language_override = 'hi'
        except Exception:
            pass

    # Write resolved voice to Redis so next call to this lead gets same agent (TTL 90 days)
    if _redis_voice_key and _tts_voice_override:
        try:
            if not redis_store.get_raw(_redis_voice_key):
                redis_store.set_raw(
                    _redis_voice_key,
                    json.dumps({"voice_id": _tts_voice_override, "provider": _tts_provider_override, "language": _tts_language_override}),
                    ex=60 * 60 * 24 * 90,
                )
        except Exception:
            pass

    # Mutable active language — updated dynamically when customer switches language mid-call
    _active_lang = [_tts_language_override or 'hi']
    # Pre-lock language when explicitly configured (query param or campaign setting) so that
    # first-utterance auto-detection cannot override the configured language.
    _lang_locked = [_lang_explicitly_set]

    if not lead_name or lead_name == "Customer":
        # Try to look up by phone number first (supports concurrent dialing)
        info = {}
        if lead_phone:
            _phone_clean = lead_phone.lstrip("+").strip()
            info = redis_store.get_pending_call(f"phone:{_phone_clean}")
            if not info and len(_phone_clean) > 10:
                info = redis_store.get_pending_call(f"phone:{_phone_clean[-10:]}")
        if not info:
            info = redis_store.get_pending_call("latest")
        lead_name = info.get("name", "Customer")
        interest = info.get("interest", "our platform") if not interest else interest
        lead_phone = info.get("phone", "") if not lead_phone else lead_phone
        if not _call_lead_id:
            _call_lead_id = info.get("lead_id")
        # Also pick up campaign/voice from phone-matched pending call
        if info.get("campaign_id"):
            _campaign_id = info["campaign_id"]

    EXOTEL_API_KEY = (os.getenv("EXOTEL_API_KEY") or "").strip()
    EXOTEL_API_TOKEN = (os.getenv("EXOTEL_API_TOKEN") or "").strip()
    EXOTEL_ACCOUNT_SID = (os.getenv("EXOTEL_ACCOUNT_SID") or "").strip()

    _exotel_call_sid = ""
    if lead_phone:
        _phone_info = redis_store.get_pending_call(f"phone:{lead_phone.lstrip('+').strip()}")
        _exotel_call_sid = _phone_info.get("exotel_call_sid", "")
    if not _exotel_call_sid:
        _exotel_call_sid = (redis_store.get_pending_call("latest").get("exotel_call_sid") or "")
    _call_start_time = time.time()
    stream_sid = None
    is_exotel_stream = False
    chat_history = []
    _llm_lock = asyncio.Lock()
    _hangup_requested = [False]  # mutable flag to block new transcripts after [HANGUP]
    _last_transcript_time = [0.0]
    _debounce_delay = 0.15  # 150ms debounce — near-instant response
    _last_tts_end_time = [0.0]  # Track when TTS last finished, to give user breathing room
    _tts_playing = [False]  # True while TTS is actively sending audio — suppress STT echo
    _recording_mic_chunks = []
    _recording_tts_chunks = []

    # ── Dual-STT merge state ──────────────────────────────────────────────────
    # Parallel Deepgram connections: primary (language=multi) + secondary (language=hi)
    # to catch Hindi speech that "multi" misidentifies as Spanish/other languages.
    _pending_stt = [None]       # (sentence, confidence, source, result, arrival_time) or None
    _pending_stt_task = [None]  # asyncio.Task for the 300ms merge-window flush
    _MERGE_WINDOW_S = 0.30      # Wait up to 300ms for the other connection's result
    _voicemail_detected = [False]  # Set True on first voicemail phrase — blocks all further STT

    # Load pronunciation guide (populated after org_id is known — see below)
    pronunciation_ctx = ""

    # Check for campaign context (from query params or Redis pending call)
    _qp_campaign = websocket.query_params.get("campaign_id", "")
    if _qp_campaign and _qp_campaign.isdigit():
        _campaign_id = int(_qp_campaign)
    if not _campaign_id:
        _pending_info = redis_store.get_pending_call("latest")
        _campaign_id = _pending_info.get("campaign_id")

    # Load product knowledge + per-product persona/call flow for system prompt
    product_ctx = ""
    _product_persona = ""
    _product_call_flow = ""
    _product_name = ""
    try:
        _user_conn = get_conn()
        _user_cursor = _user_conn.cursor()
        _user_cursor.execute("SELECT org_id FROM leads WHERE id = %s LIMIT 1", (_call_lead_id,))
        _user_row = _user_cursor.fetchone()
        _call_org_id = _user_row.get('org_id') if _user_row else 1
        _user_conn.close()

        # Load pronunciation guide scoped to this org
        if _call_org_id:
            pronunciation_ctx = get_pronunciation_context(_call_org_id)

        if _campaign_id:
            # Campaign-specific: load ONLY that campaign's product + persona + call flow
            from database import get_product_context_for_campaign
            _camp_product = get_product_context_for_campaign(_campaign_id)
            product_ctx = _camp_product.get("product_ctx", "")
            _product_persona = _camp_product.get("agent_persona", "")
            _product_call_flow = _camp_product.get("call_flow_instructions", "")
            _product_name = _camp_product.get("product_name", "")
            if not product_ctx:
                product_ctx = get_product_knowledge_context(org_id=_call_org_id)
        elif _call_org_id:
            custom = get_org_custom_prompt(_call_org_id)
            if custom.strip():
                product_ctx = "\n\n[PRODUCT KNOWLEDGE]:\n" + custom
            else:
                product_ctx = get_product_knowledge_context(org_id=_call_org_id)
        else:
            product_ctx = get_product_knowledge_context()
    except Exception:
        product_ctx = get_product_knowledge_context()

    # Build call context (voice identity, company name, lead source, system prompt)
    _ctx = build_call_context(
        lead_name=lead_name,
        lead_phone=lead_phone,
        interest=interest,
        _call_lead_id=_call_lead_id,
        _campaign_id=_campaign_id,
        _call_org_id=_call_org_id,
        _tts_voice_override=_tts_voice_override,
        product_ctx=product_ctx,
        _product_persona=_product_persona,
        _product_call_flow=_product_call_flow,
        pronunciation_ctx=pronunciation_ctx,
        _product_name=_product_name,
        _language=(_tts_language_override or "hi"),
    )
    dynamic_context = _ctx["dynamic_context"]
    _agent_name = _ctx["_agent_name"]
    _lead_first = _ctx["_lead_first"]
    _company_name = _ctx["_company_name"]
    _bol = _ctx["_bol"]
    _source_context = _ctx["_source_context"]
    greeting_text = _ctx["greeting_text"]

    if not dg_client:
        dg_client = DeepgramClient(os.getenv("DEEPGRAM_API_KEY", "dummy"))
    if not llm_client:
        llm_client = genai.Client(api_key=(os.getenv("GEMINI_API_KEY") or "dummy").strip())

    dg_connection = dg_client.listen.websocket.v("1")
    dg_connection_hi = dg_client.listen.websocket.v("1")  # secondary: explicit Hindi
    loop = asyncio.get_event_loop()

    def on_error(self, error, **kwargs):
        logging.getLogger("uvicorn.error").error(f"[STT ERROR] Deepgram fired an error: {error}")

    def on_speech_started(self, **kwargs):
        if stream_sid:
            asyncio.run_coroutine_threadsafe(
                websocket.send_text(json.dumps({"event": "clear", "streamSid": stream_sid})),
                loop,
            )
        if stream_sid in active_tts_tasks and not active_tts_tasks[stream_sid].done():
            loop.call_soon_threadsafe(active_tts_tasks[stream_sid].cancel)

    # ── Dual-STT: shared processing after merge picks the best transcript ──
    def _do_process_stt(sentence, result):
        """Run language detection + enqueue for LLM. Called once per utterance after merge."""
        conv_logger = logging.getLogger("uvicorn.error")
        conv_logger.info(f"[STT] USER SAID: {sentence}")
        if stream_sid:
            call_logger.call_event(stream_sid, "STT_TRANSCRIPT", sentence[:100])
        try:
            asyncio.run_coroutine_threadsafe(websocket.send_json({"event": "user_speech", "text": sentence}), loop)
        except Exception:
            pass

        # ── Language detection (three-pass, with lock) ───────────────────
        if _lang_locked[0]:
            _intent_lang = _detect_lang_intent(sentence)
            if _intent_lang and _intent_lang != _active_lang[0]:
                conv_logger.info(f"[LANG INTENT-LOCKED] {_active_lang[0]} → {_intent_lang} (keyword: '{sentence[:60]}')")
                _active_lang[0] = _intent_lang
            else:
                conv_logger.info(f"[LANG LOCKED] Keeping {_active_lang[0]} — ignoring detection on: '{sentence[:60]}'")
        else:
            _intent_lang = _detect_lang_intent(sentence)
            if _intent_lang and _intent_lang != _active_lang[0]:
                conv_logger.info(f"[LANG INTENT] {_active_lang[0]} → {_intent_lang} (keyword: '{sentence[:60]}')")
                _active_lang[0] = _intent_lang
                _lang_locked[0] = True
            else:
                _dg_lang = getattr(result.channel, 'detected_language', None) if result else None
                _lang_map = {
                    'hi': 'hi', 'hi-IN': 'hi', 'hi-latn': 'hi',
                    'bn': 'bn', 'bn-IN': 'bn',
                    'en': 'en', 'en-US': 'en', 'en-IN': 'en', 'en-AU': 'en', 'en-GB': 'en',
                    'mr': 'mr', 'mr-IN': 'mr',
                    'ta': 'ta', 'ta-IN': 'ta',
                    'te': 'te', 'te-IN': 'te',
                    'kn': 'kn', 'kn-IN': 'kn',
                    'ml': 'ml', 'ml-IN': 'ml',
                    'gu': 'gu', 'gu-IN': 'gu',
                    'pa': 'pa', 'pa-IN': 'pa',
                }
                _resolved = _lang_map.get(_dg_lang, None) if _dg_lang else None
                if not _resolved:
                    _resolved = _detect_script_language(sentence)
                if _resolved and _resolved != _active_lang[0] and not _is_lang_neutral(sentence):
                    conv_logger.info(f"[LANG SWITCH] {_active_lang[0]} → {_resolved} (dg={_dg_lang}, text={sentence[:60]})")
                    _active_lang[0] = _resolved
                    _lang_locked[0] = True
                elif _resolved and _resolved != _active_lang[0]:
                    conv_logger.info(f"[LANG NEUTRAL] Skipping switch {_active_lang[0]} → {_resolved} for: '{sentence[:60]}'")
        # ─────────────────────────────────────────────────────────────────

        chat_history.append({"role": "user", "parts": [{"text": sentence}]})

    # ── Dual-STT: merge callback — buffers transcripts, picks best ─────
    def _on_stt_result(result, source):
        """Called by both primary (multi) and secondary (hi) Deepgram callbacks."""
        sentence = result.channel.alternatives[0].transcript
        if not sentence or not result.is_final:
            return
        conv_logger = logging.getLogger("uvicorn.error")

        # ── Voicemail detection — highest priority, before all other checks ──
        if not _voicemail_detected[0] and _is_voicemail(sentence, time.time() - _call_start_time):
            _voicemail_detected[0] = True
            _hangup_requested[0] = True
            conv_logger.info(f"[VOICEMAIL] Detected: '{sentence[:80]}' — leaving pitch and hanging up")

            async def _leave_voicemail_and_hangup():
                # Cancel any active TTS (greeting may still be playing)
                if stream_sid and stream_sid in active_tts_tasks:
                    t = active_tts_tasks[stream_sid]
                    if not t.done():
                        t.cancel()
                        try:
                            await t
                        except (asyncio.CancelledError, Exception):
                            pass

                # Build a one-sentence pitch in the campaign language
                lang = _active_lang[0]
                name = _lead_first or lead_name.split()[0] if lead_name else "जी"
                if lang == 'bn':
                    pitch = (f"নমস্কার {name} জি, আমি {_agent_name}, {_company_name} থেকে বলছিলাম — "
                             f"আপনার enquiry সম্পর্কে কথা বলতে চেয়েছিলাম, সময় পেলে এই নম্বরে call back করুন।")
                elif lang == 'en':
                    pitch = (f"Hi {name}, this is {_agent_name} from {_company_name} — "
                             f"I was calling regarding your enquiry, please call us back at your convenience.")
                else:  # default Hindi
                    pitch = (f"नमस्ते {name} जी, मैं {_agent_name}, {_company_name} से बोल रहा था — "
                             f"आपकी enquiry के बारे में बात करनी थी, समय मिले तो इस नंबर पर call back करें।")

                _tts_playing[0] = True
                await synthesize_and_send_audio(
                    text=pitch,
                    stream_sid=stream_sid,
                    websocket=websocket,
                    tts_provider_override=_tts_provider_override,
                    tts_voice_override=_tts_voice_override,
                    tts_language_override=lang,
                )
                _tts_playing[0] = False

                # Grace period for audio to finish playing before closing
                await asyncio.sleep(4)
                try:
                    await websocket.close()
                except Exception:
                    pass

            asyncio.run_coroutine_threadsafe(_leave_voicemail_and_hangup(), loop)
            return
        # ────────────────────────────────────────────────────────────────────

        if _hangup_requested[0]:
            conv_logger.info(f"[STT-{source}] IGNORED (hangup pending): {sentence}")
            return
        if _voicemail_detected[0]:
            return
        if _tts_playing[0] or (time.time() - _last_tts_end_time[0] < 1.0):
            conv_logger.info(f"[STT-{source}] IGNORED (TTS echo suppression): {sentence}")
            return

        confidence = getattr(result.channel.alternatives[0], 'confidence', 0.0) or 0.0
        arrival = time.time()
        conv_logger.info(f"[STT-{source}] conf={confidence:.2f}: {sentence}")

        if not _use_secondary_stt:
            # Single-connection mode (English campaigns) — process directly
            _do_process_stt(sentence, result)
            asyncio.run_coroutine_threadsafe(_process_transcript(sentence), loop)
            return

        # Dual mode: merge with pending transcript from the other connection
        async def _merge_then_process():
            if _pending_stt[0] is not None:
                # Second to arrive — compare and pick best
                prev_sent, prev_conf, prev_src, prev_result = _pending_stt[0]
                _pending_stt[0] = None
                # Cancel the pending flush timer
                if _pending_stt_task[0] and not _pending_stt_task[0].done():
                    _pending_stt_task[0].cancel()
                    _pending_stt_task[0] = None

                if confidence >= prev_conf:
                    winner_sent, winner_result, winner_src = sentence, result, source
                    loser_src, loser_conf = prev_src, prev_conf
                else:
                    winner_sent, winner_result, winner_src = prev_sent, prev_result, prev_src
                    loser_src, loser_conf = source, confidence
                conv_logger.info(f"[STT MERGE] Winner={winner_src} ({confidence if winner_src == source else prev_conf:.2f}), "
                                 f"loser={loser_src} ({loser_conf:.2f})")
                _do_process_stt(winner_sent, winner_result)
                await _process_transcript(winner_sent)
            else:
                # First to arrive — buffer and wait for the other
                _pending_stt[0] = (sentence, confidence, source, result)

                async def _flush():
                    await asyncio.sleep(_MERGE_WINDOW_S)
                    if _pending_stt[0] is not None:
                        flushed_sent, flushed_conf, flushed_src, flushed_result = _pending_stt[0]
                        _pending_stt[0] = None
                        conv_logger.info(f"[STT MERGE] Flush: only {flushed_src} arrived (conf={flushed_conf:.2f})")
                        _do_process_stt(flushed_sent, flushed_result)
                        await _process_transcript(flushed_sent)

                _pending_stt_task[0] = asyncio.ensure_future(_flush())

        asyncio.run_coroutine_threadsafe(_merge_then_process(), loop)

    def on_message_primary(self, result, **kwargs):
        _on_stt_result(result, source="multi")

    def on_message_secondary(self, result, **kwargs):
        _on_stt_result(result, source="hi")

    async def _process_transcript(sentence):
        conv_logger = logging.getLogger("uvicorn.error")
        try:
            t_start = time.time()
            _last_transcript_time[0] = t_start

            # Brief cooldown after TTS ends so user can start speaking
            time_since_tts = t_start - _last_tts_end_time[0]
            if time_since_tts < 0.2:
                await asyncio.sleep(0.2 - time_since_tts)

            await asyncio.sleep(_debounce_delay)
            if _last_transcript_time[0] != t_start:
                conv_logger.info("[DEBOUNCE] Skipping older transcript — newer one pending.")
                return
            if _llm_lock.locked():
                conv_logger.info("[TURN_GUARD] Skipping — LLM already processing.")
                return

            async with _llm_lock:
                if stream_sid:
                    for monitor in monitor_connections.get(stream_sid, set()):
                        try:
                            await monitor.send_json({"type": "transcript", "role": "user", "text": sentence})
                        except Exception:
                            pass
                    if redis_store.get_takeover(stream_sid):
                        return
                    pending = redis_store.pop_all_whispers(stream_sid)
                    if pending:
                        for whisper in pending:
                            chat_history.append({"role": "user", "parts": [{"text": f"Manager Whisper: {whisper}. Acknowledge this implicitly in your next response."}]})

                # RAG via Local FAISS
                rag_context = ""
                if _call_org_id:
                    try:
                        import rag
                        context = rag.retrieve_context(sentence, _call_org_id, top_k=2)
                        if context:
                            rag_context = "\n\n[COMPANY KNOWLEDGE - Check if this has facts relevant to the discussion and explicitly use it]:\n" + context
                    except Exception as e:
                        conv_logger.error(f"RAG FAISS lookup error: {e}")

                t_pre_llm = time.time()
                _lang_label = _LANG_LABEL.get(_active_lang[0], _active_lang[0])
                _lang_directive = (
                    f"\n\n⚠️ LANGUAGE OVERRIDE — MANDATORY: "
                    f"The customer is speaking in {_lang_label}. "
                    f"Your ENTIRE response MUST be in {_lang_label} ONLY. "
                    f"Do NOT use any other language. Not even one word."
                )
                final_system_instruction = dynamic_context + rag_context + _lang_directive

                # Start TTS Worker Queue for Streaming Pipeline
                tts_queue = asyncio.Queue()

                # [Phase 2: Conversational Backchanneling]
                if len(sentence.split()) > 2:
                    import random
                    if random.random() < 0.6:
                        fillers = ["Hmm...", "Achha...", "Okay...", "Haan..."]
                        await tts_queue.put(random.choice(fillers))

                async def tts_worker():
                    try:
                        while True:
                            tts_text = await tts_queue.get()
                            if tts_text is None:
                                break
                            _tts_playing[0] = True
                            await synthesize_and_send_audio(
                                text=tts_text,
                                stream_sid=stream_sid,
                                websocket=websocket,
                                tts_provider_override=_tts_provider_override,
                                tts_voice_override=_tts_voice_override,
                                tts_language_override=_active_lang[0]
                            )
                            _tts_playing[0] = False
                            _last_tts_end_time[0] = time.time()
                            tts_queue.task_done()
                    except asyncio.CancelledError:
                        _tts_playing[0] = False
                        _last_tts_end_time[0] = time.time()
                    except Exception as e:
                        conv_logger.error(f"TTS Worker Error: {e}")

                if stream_sid in active_tts_tasks and not active_tts_tasks[stream_sid].done():
                    active_tts_tasks[stream_sid].cancel()
                    try:
                        await active_tts_tasks[stream_sid]
                    except (asyncio.CancelledError, Exception):
                        pass

                worker_task = asyncio.create_task(tts_worker())
                if stream_sid:
                    active_tts_tasks[stream_sid] = worker_task

                try:
                    import llm_provider
                    import re

                    sentence_separators = re.compile(r'([.!?|\n]+)')
                    full_response = ""
                    current_sentence = ""
                    first_token_time = None

                    _lang = (_active_lang[0] or "hi")
                    _llm_max_tokens = 300 if _lang == "mr" else 80 if _lang in ("hi", "bn") else 400
                    async for chunk in llm_provider.generate_response_stream(
                        chat_history=chat_history,
                        system_instruction=final_system_instruction,
                        max_tokens=_llm_max_tokens,
                    ):
                        if first_token_time is None:
                            first_token_time = time.time()
                            conv_logger.info(f"TIMING: TTFB LLM = {first_token_time - t_pre_llm:.2f}s")

                        full_response += chunk
                        current_sentence += chunk

                        parts = sentence_separators.split(current_sentence)
                        if len(parts) > 1:
                            complete_text = "".join(parts[:-1]).strip()
                            remaining_text = parts[-1]

                            if complete_text:
                                clean_text = re.sub(r'[\*\_\#\`\~\>\|]', '', complete_text)
                                clean_text = re.sub(r'\[([^\]]+)\]\([^)]+\)', r'\1', clean_text)
                                clean_text = clean_text.strip()
                                if clean_text:
                                    await tts_queue.put(clean_text)

                            current_sentence = remaining_text

                    if current_sentence.strip():
                        clean_text = re.sub(r'[\*\_\#\`\~\>\|]', '', current_sentence.strip())
                        clean_text = re.sub(r'\[([^\]]+)\]\([^)]+\)', r'\1', clean_text)
                        clean_text = clean_text.strip()
                        if clean_text and "[HANGUP]" not in clean_text:
                            await tts_queue.put(clean_text)

                    await tts_queue.put(None)

                    t_post_llm = time.time()
                    chat_history.append({"role": "model", "parts": [{"text": full_response}]})
                    conv_logger.info(f"[LLM] AI STREAM RESPONSE FULL: {full_response[:200]}")
                    conv_logger.info(f"TIMING: pre_llm={t_pre_llm - t_start:.2f}s, first_token={first_token_time - t_pre_llm if first_token_time else 0:.2f}s, total_gen={t_post_llm - t_pre_llm:.2f}s")

                    try:
                        await websocket.send_json({"event": "llm_response", "text": full_response.replace("[HANGUP]", "")})
                    except Exception:
                        pass
                    if stream_sid:
                        call_logger.call_event(stream_sid, "LLM_RESPONSE", full_response[:100], llm_time_s=round(t_post_llm - t_pre_llm, 3))
                        for monitor in monitor_connections.get(stream_sid, set()):
                            try:
                                await monitor.send_json({"type": "transcript", "role": "agent", "text": full_response.replace("[HANGUP]", "")})
                            except Exception:
                                pass

                    # AI Physical Disconnect Command Handler
                    if "[HANGUP]" in full_response:
                        _hangup_requested[0] = True
                        conv_logger.info("[COMMAND] LLM explicitly commanded a websocket disconnect.")
                        if stream_sid:
                            call_logger.call_event(stream_sid, "LLM_HANGUP", "AI explicitly ended the call block.")
                        try:
                            await asyncio.wait_for(worker_task, timeout=15)
                            conv_logger.info("[HANGUP] TTS drain complete, closing connection.")
                        except (asyncio.TimeoutError, Exception):
                            conv_logger.info("[HANGUP] TTS drain timed out, forcing close.")
                        await asyncio.sleep(7)
                        try:
                            await websocket.close()
                        except Exception:
                            pass
                        return
                except Exception as e:
                    import traceback
                    conv_logger.error(f"Error streaming LLM response: {e}")
                    conv_logger.error(traceback.format_exc())
                    await tts_queue.put(None)
                    return
        except Exception as _crash:
            import traceback
            logging.getLogger("uvicorn.error").error(f"[SYSTEM FATAL] _process_transcript SILENT CRASH: {_crash}")
            logging.getLogger("uvicorn.error").error(traceback.format_exc())

    dg_connection.on(LiveTranscriptionEvents.SpeechStarted, on_speech_started)
    dg_connection.on(LiveTranscriptionEvents.Transcript, on_message_primary)
    dg_connection.on(LiveTranscriptionEvents.Error, on_error)

    _stt_lang = _tts_language_override or "hi"
    # Use Deepgram's multi-language model for non-English campaigns so it can
    # recognise mid-call language switches (Bengali ↔ Hindi ↔ English).
    # "multi" is supported in nova-2 streaming and doesn't require detect_language.
    _use_multi = _stt_lang not in ("en", "en-US", "en-IN")
    _stt_effective_lang = "multi" if _use_multi else _stt_lang
    # Dual-STT: run a secondary Hindi connection alongside "multi" to catch
    # Hindi speech that the multi-language model misidentifies as Spanish/other.
    _use_secondary_stt = _use_multi
    _secondary_stt_lang = "hi"
    dg_connection.start(
        LiveOptions(
            model="nova-2",
            language=_stt_effective_lang,
            encoding="linear16",
            sample_rate=8000,
            channels=1,
            endpointing=300,          # 300ms VAD — fast response while still catching natural pauses
            utterance_end_ms="1000",  # Force utterance generation
            interim_results=True,     # Fix HTTP 400 deepgram crash
        )
    )

    if _use_secondary_stt:
        dg_connection_hi.on(LiveTranscriptionEvents.SpeechStarted, on_speech_started)
        dg_connection_hi.on(LiveTranscriptionEvents.Transcript, on_message_secondary)
        dg_connection_hi.on(LiveTranscriptionEvents.Error, on_error)
        dg_connection_hi.start(
            LiveOptions(
                model="nova-2",
                language=_secondary_stt_lang,
                encoding="linear16",
                sample_rate=8000,
                channels=1,
                endpointing=300,
                utterance_end_ms="1000",
                interim_results=True,
            )
        )

    ws_logger = logging.getLogger("uvicorn.error")
    ws_logger.info(f"Media stream handler started for {lead_name} (dual_stt={_use_secondary_stt})")
    greeting_sent = False
    _dg_alive = True

    # Deepgram keepalive: send periodic keepalive to prevent STT timeout during TTS playback
    async def _deepgram_keepalive():
        while _dg_alive:
            await asyncio.sleep(5)
            try:
                dg_connection.keep_alive()
            except Exception:
                pass
            if _use_secondary_stt:
                try:
                    dg_connection_hi.keep_alive()
                except Exception:
                    pass
    _keepalive_task = asyncio.create_task(_deepgram_keepalive())

    # CRITICAL: Send greeting immediately on WebSocket connect
    # Exotel VoiceBot has a 10-second timeout — if we don't send audio, it kills the session
    # Detect web sim vs Exotel from query params — web sim passes lead_id + name in URL
    _is_web_sim = bool(_qp_lead_id and lead_name)
    stream_sid = f"web_sim_{_call_lead_id}_{int(time.time()*1000)}" if _is_web_sim else f"exotel-{_uuid.uuid4().hex[:12]}"
    is_exotel_stream = not _is_web_sim
    twilio_websockets[stream_sid] = websocket
    monitor_connections[stream_sid] = set()
    redis_store.delete_whispers(stream_sid)
    redis_store.set_takeover(stream_sid, False)
    _tts_recording_buffers[stream_sid] = []
    ws_logger.info(f"[WS] Immediate stream init, sid={stream_sid}")

    # Only send greeting immediately for web simulator — Exotel's real stream SID
    # isn't known yet (arrives in the "start" JSON event). Starting TTS with a fake
    # SID means Exotel ignores every audio packet → silence → immediate disconnect.
    if _is_web_sim:
        chat_history.append({"role": "model", "parts": [{"text": greeting_text}]})
        ws_logger.info(f"[GREETING] Immediate greeting for {lead_name}, sid={stream_sid}")
        call_logger.call_event(stream_sid, "GREETING_SENT", f"to={lead_name}")
        greeting_sent = True
        try:
            await websocket.send_json({"event": "llm_response", "text": greeting_text})
        except Exception:
            pass
        active_tts_tasks[stream_sid] = asyncio.create_task(
            synthesize_and_send_audio(greeting_text, stream_sid, websocket, _tts_provider_override, _tts_voice_override, _tts_language_override)
        )
    else:
        ws_logger.info(f"[GREETING] Exotel stream — deferring greeting until real stream SID arrives, sid={stream_sid}")

    try:
        while True:
            try:
                msg = await websocket.receive()
            except Exception as e:
                ws_logger.error(f"[WS RECV ERROR] Connection lost: {e}", exc_info=True)
                break

            if msg.get("type") == "websocket.disconnect":
                ws_logger.info(f"[WS DISCONNECT] Client sent disconnect frame, sid={stream_sid}")
                break

            # Handle binary frames (Exotel sends raw audio bytes)
            if "bytes" in msg and msg["bytes"]:
                audio_data = msg["bytes"]
                if len(_recording_mic_chunks) % 100 == 0:
                    ws_logger.info(f"[DEBUG-REC] Binary frame: {len(audio_data)} bytes, is_exotel={is_exotel_stream}, mic_chunks={len(_recording_mic_chunks)}")
                if not stream_sid:
                    stream_sid = f"exotel-{_uuid.uuid4().hex[:12]}"
                    twilio_websockets[stream_sid] = websocket
                    monitor_connections[stream_sid] = set()
                    redis_store.delete_whispers(stream_sid)
                    redis_store.set_takeover(stream_sid, False)
                    ws_logger.info(f"[WS] Exotel binary stream started, sid={stream_sid}")
                    call_logger.call_event(stream_sid, "WS_CONNECTED", f"name={lead_name}, phone={lead_phone}")
                    _tts_recording_buffers[stream_sid] = []

                if not greeting_sent:
                    greeting_sent = True
                    chat_history.append({"role": "model", "parts": [{"text": greeting_text}]})
                    ws_logger.info(f"[GREETING] Sending greeting for {lead_name}")
                    call_logger.call_event(stream_sid, "GREETING_SENT", f"to={lead_name}")
                    active_tts_tasks[stream_sid] = asyncio.create_task(
                        synthesize_and_send_audio(greeting_text, stream_sid, websocket, _tts_provider_override, _tts_voice_override, _tts_language_override)
                    )

                # Forward audio to Deepgram — Exotel sends mulaw, Deepgram expects PCM linear16
                if is_exotel_stream:
                    import audioop as _ao_dg
                    try:
                        _dg_pcm = _ao_dg.ulaw2lin(audio_data, 2)
                    except Exception:
                        _dg_pcm = audio_data
                else:
                    _dg_pcm = audio_data
                dg_connection.send(_dg_pcm)
                if _use_secondary_stt:
                    dg_connection_hi.send(_dg_pcm)
                # Capture mic audio for recording
                if is_exotel_stream:
                    import audioop as _ao
                    try:
                        pcm = _ao.ulaw2lin(audio_data, 2)
                        _recording_mic_chunks.append((time.time(), pcm))
                    except Exception:
                        pass
                else:
                    # Web sim sends PCM directly via binary frames
                    _recording_mic_chunks.append((time.time(), audio_data))

            # Handle text frames (Twilio/Exotel JSON)
            elif "text" in msg and msg["text"]:
                try:
                    data = json.loads(msg["text"])
                except Exception as e:
                    ws_logger.error(f"[WS JSON ERROR] Failed to parse msg: {e}", exc_info=True)
                    ws_logger.warning(f"Failed to parse WS text: {e}")
                    continue

                if data.get("event") != "media":
                    ws_logger.info(f"WS text message received: {str(data)[:200]}")

                if data.get("event") == "connected":
                    ws_logger.info("Exotel WebSocket connected event received")
                    continue
                elif data.get("event") == "start":
                    _prev_stream_sid = stream_sid  # remember before reassignment
                    stream_sid = (
                        data.get("stream_sid")
                        or data.get("start", {}).get("streamSid")
                        or f"exotel-{_uuid.uuid4().hex[:12]}"
                    )
                    if stream_sid.startswith("web_sim_"):
                        is_exotel_stream = False
                        ws_logger.info(f"[BROWSER SIM] Detected web simulator stream, sid={stream_sid}")
                    else:
                        is_exotel_stream = True
                        ws_logger.info(f"[EXOTEL] Detected Exotel stream, sid={stream_sid}")
                    ws_logger.info(f"[DEBUG-REC] Stream started: sid={stream_sid}, exotel={is_exotel_stream}, start_data_keys={list(data.keys())}")

                    # [RACE CONDITION FIX] Map strict CallSid from Exotel payload
                    call_sid = data.get("start", {}).get("callSid") or data.get("call_sid") or data.get("CallSid")
                    info = redis_store.get_pending_call(call_sid) if call_sid else {}
                    if info:
                        true_name = info.get("name", "Customer")
                        old_name = lead_name
                        lead_name = true_name
                        _call_lead_id = info.get("lead_id", _call_lead_id)
                        _exotel_call_sid = call_sid
                        dynamic_context = dynamic_context.replace(f"Tum {old_name} ko call kar rahe ho", f"Tum {true_name} ko call kar rahe ho")
                        ws_logger.info(f"[CONTEXT FIX] Successfully mapped CallSid {call_sid} to {true_name}, LeadID: {_call_lead_id}")

                    twilio_websockets[stream_sid] = websocket
                    monitor_connections[stream_sid] = set()
                    redis_store.delete_whispers(stream_sid)
                    redis_store.set_takeover(stream_sid, False)
                    # Migrate recording buffer and active TTS task from old sid to new sid
                    if _prev_stream_sid and _prev_stream_sid != stream_sid:
                        existing_buf = _tts_recording_buffers.pop(_prev_stream_sid, [])
                        _tts_recording_buffers[stream_sid] = existing_buf
                        if _prev_stream_sid in active_tts_tasks:
                            active_tts_tasks[stream_sid] = active_tts_tasks.pop(_prev_stream_sid)
                        twilio_websockets.pop(_prev_stream_sid, None)
                        monitor_connections.pop(_prev_stream_sid, None)
                    else:
                        _tts_recording_buffers[stream_sid] = []

                    if _campaign_id:
                        from live_logs import emit_campaign_event
                        if not is_exotel_stream:
                            # Web sim: no prior DIALING event from dial_routes, emit it now
                            emit_campaign_event(_campaign_id, lead_name, lead_phone, "dialing", "via web-sim")
                        emit_campaign_event(_campaign_id, lead_name, lead_phone, "connected", "audio stream opened")

                    if not greeting_sent:
                        greeting_sent = True
                        ws_logger.info(f"GREETING: Triggering TTS greeting for stream {stream_sid}")
                        active_tts_tasks[stream_sid] = asyncio.create_task(
                            synthesize_and_send_audio(
                                f"नमस्ते {_lead_first} जी, मैं {_agent_name}, {_company_name} से {_bol}। आपने {_source_context} क्या?",
                                stream_sid, websocket, _tts_provider_override, _tts_voice_override, _tts_language_override,
                            )
                        )
                elif data.get("event") == "media":
                    raw_audio = base64.b64decode(data["media"]["payload"])
                    if is_exotel_stream:
                        import audioop as _ao_dg2
                        try:
                            _dg_pcm2 = _ao_dg2.ulaw2lin(raw_audio, 2)
                        except Exception:
                            _dg_pcm2 = raw_audio
                    else:
                        _dg_pcm2 = raw_audio
                    dg_connection.send(_dg_pcm2)
                    if _use_secondary_stt:
                        dg_connection_hi.send(_dg_pcm2)
                    if len(_recording_mic_chunks) % 100 == 0:
                        ws_logger.info(f"[DEBUG-REC] Media event: {len(raw_audio)} bytes, is_exotel={is_exotel_stream}, mic_chunks={len(_recording_mic_chunks)}")
                    if is_exotel_stream:
                        import audioop as _ao2
                        try:
                            pcm = _ao2.ulaw2lin(raw_audio, 2)
                            _recording_mic_chunks.append((time.time(), pcm))
                        except Exception as _mic_err:
                            ws_logger.error(f"[DEBUG-REC] Mic capture error: {_mic_err}")
                    else:
                        # Capture anyway for non-exotel streams (browser sim sends PCM directly)
                        _recording_mic_chunks.append((time.time(), raw_audio))
                elif data.get("event") == "stop":
                    print("Media stream stopped.")
                    break
                else:
                    if not stream_sid:
                        stream_sid = f"exotel-{_uuid.uuid4().hex[:12]}"
                        twilio_websockets[stream_sid] = websocket
                        monitor_connections[stream_sid] = set()
                        redis_store.delete_whispers(stream_sid)
                        redis_store.set_takeover(stream_sid, False)
                        ws_logger.info(f"Exotel text stream started, sid={stream_sid}")
                    _tts_recording_buffers.setdefault(stream_sid, [])
                    if not greeting_sent:
                        greeting_sent = True
                        active_tts_tasks[stream_sid] = asyncio.create_task(
                            synthesize_and_send_audio(
                                f"नमस्ते {_lead_first} जी, मैं {_agent_name}, {_company_name} से {_bol}। आपने {_source_context} क्या?",
                                stream_sid, websocket, _tts_provider_override, _tts_voice_override, _tts_language_override,
                            )
                        )
    except Exception as e:
        logging.getLogger("uvicorn.error").error(f"[WS] Error in media stream: {e}")
        if stream_sid:
            call_logger.call_event(stream_sid, "WS_ERROR", str(e))
    finally:
        logging.getLogger("uvicorn.error").info(f"[WS CLOSED] sid={stream_sid}, turns={len(chat_history)}, exotel={is_exotel_stream}")

        if _campaign_id and stream_sid:
            from live_logs import emit_campaign_event
            duration_s = int(time.time() - _call_start_time)
            via = "via web-sim" if not is_exotel_stream else "via exotel"
            if greeting_sent:
                evt = "completed"
                detail = f"{duration_s}s call | {via}"
            else:
                evt = "hangup"
                detail = f"no answer | {duration_s}s | {via}"
            emit_campaign_event(_campaign_id, lead_name, lead_phone, evt, detail)

        # Cleanup any active TTS tasks to prevent dangling background processes
        if stream_sid in active_tts_tasks:
            t = active_tts_tasks[stream_sid]
            if not t.done():
                t.cancel()
            del active_tts_tasks[stream_sid]
            
        if stream_sid:
            call_logger.call_event(stream_sid, "WS_DISCONNECTED", f"turns={len(chat_history)}")
            call_logger.end_call(stream_sid)
            # Save transcript and recording to DB
            if _call_lead_id and chat_history:
                try:
                    await save_call_recording_and_transcript(
                        stream_sid=stream_sid,
                        _call_lead_id=_call_lead_id,
                        _exotel_call_sid=_exotel_call_sid,
                        chat_history=chat_history,
                        _recording_mic_chunks=_recording_mic_chunks,
                        _tts_recording_buffers=_tts_recording_buffers,
                        _call_start_time=_call_start_time,
                        EXOTEL_API_KEY=EXOTEL_API_KEY,
                        EXOTEL_API_TOKEN=EXOTEL_API_TOKEN,
                        EXOTEL_ACCOUNT_SID=EXOTEL_ACCOUNT_SID,
                        _campaign_id=_campaign_id,
                        call_source='sim_web_call' if not is_exotel_stream else 'exotel',
                    )
                except Exception as _te:
                    import traceback
                    ws_logger.error(f"[TRANSCRIPT] Error saving: {_te}\n{traceback.format_exc()}")

            # Record billing usage
            if _call_org_id and _call_start_time:
                try:
                    # For sim web calls use audio chunk timestamps for accuracy;
                    # WS session includes setup/teardown time that isn't real call time.
                    if not is_exotel_stream:
                        _all_t = [t for t, _ in _recording_mic_chunks] + [t for t, _ in _tts_recording_buffers.get(stream_sid, [])]
                        call_duration_s = (max(_all_t) - min(_all_t) + 0.5) if _all_t else (time.time() - _call_start_time)
                    else:
                        call_duration_s = time.time() - _call_start_time
                    call_minutes = round(call_duration_s / 60, 2)
                    record_usage(org_id=_call_org_id, minutes=call_minutes, call_id=_call_lead_id)
                    ws_logger.info(f"[BILLING] Recorded {call_minutes} min for org {_call_org_id}")
                except Exception as _billing_err:
                    ws_logger.error(f"[BILLING] Failed to record usage: {_billing_err}")

        # Cleanup
        if stream_sid:
            redis_store.cleanup_call(stream_sid)
        if stream_sid and stream_sid in _tts_recording_buffers:
            del _tts_recording_buffers[stream_sid]
        if stream_sid and stream_sid in twilio_websockets:
            del twilio_websockets[stream_sid]
        _dg_alive = False
        _keepalive_task.cancel()
        try:
            dg_connection.finish()
        except Exception:
            pass
        if _use_secondary_stt:
            try:
                dg_connection_hi.finish()
            except Exception:
                pass
        try:
            await websocket.close()
        except Exception:
            pass

        # Omnichannel Summary & WhatsApp Trigger
        if len(chat_history) > 2:
            try:
                transcript_text = "\n".join([f"{m['role']}: {m['parts'][0]['text']}" for m in chat_history if isinstance(m, dict) and 'parts' in m])
                summary_prompt = "You are a sales evaluator. Analyze the transcript. Return strictly a valid JSON object with: {'sentiment': 'Cold/Warm/Hot', 'requires_brochure': true/false, 'note': 'short summary of next steps'}. If the lead asks for details, pricing, or a brochure, set requires_brochure to true."
                res = await llm_client.aio.models.generate_content(
                    model="gemini-2.5-flash",
                    contents=transcript_text,
                    config=types.GenerateContentConfig(system_instruction=summary_prompt)
                )
                text = res.text.replace("```json", "").replace("```", "").strip()
                outcome = json.loads(text)
                if lead_phone:
                    from database import update_call_note
                    update_call_note("ws_" + str(stream_sid), outcome.get("note", "Call completed via Dialer."), lead_phone)
            except Exception as e:
                print(f"Omnichannel intent trigger error: {e}")


# ─── Sandbox Stream ─────────────────────────────────────────────────────────

async def sandbox_stream(websocket: WebSocket):
    await websocket.accept()
    from deepgram import DeepgramClient, LiveTranscriptionEvents, LiveOptions
    import os, json, base64
    import llm_provider
    import httpx
    
    dg = DeepgramClient(os.getenv("DEEPGRAM_API_KEY", "dummy"))
    dg_conn = dg.listen.websocket.v("1")
    chat_hist = []

    async def on_message(self, result, **kwargs):
        sentence = result.channel.alternatives[0].transcript
        if sentence and result.is_final:
            chat_hist.append({"role": "user", "parts": [{"text": sentence}]})
            await websocket.send_json({"type": "transcript", "role": "user", "text": sentence})
            try:
                system_prompt = "You are in AI sandbox test mode. A sales manager is interacting with you. Be extremely aggressive answering sales objections, keeping answers to one line."
                response_text = await llm_provider.generate_response(
                    chat_history=chat_hist,
                    system_instruction=system_prompt,
                    max_tokens=150
                )
                
                chat_hist.append({"role": "model", "parts": [{"text": response_text}]})
                
                url = f"https://api.elevenlabs.io/v1/text-to-speech/{os.getenv('ELEVENLABS_VOICE_ID')}/stream?output_format=mp3_44100_128"
                headers = {"xi-api-key": os.getenv("ELEVENLABS_API_KEY")}
                payload = {"text": response_text, "model_id": "eleven_turbo_v2"}
                async with httpx.AsyncClient() as client:
                    async with client.stream("POST", url, json=payload, headers=headers) as resp:
                        async for chunk in resp.aiter_bytes(chunk_size=4000):
                            if chunk:
                                await websocket.send_json({"type": "audio", "payload": base64.b64encode(chunk).decode('utf-8')})
                                
                await websocket.send_json({"type": "transcript", "role": "agent", "text": response_text})
            except Exception as e:
                import logging
                logging.getLogger("uvicorn.error").error(f"[SANDBOX CRASH] LLM Provider Error: {e}", exc_info=True)
                print(f"Sandbox LLM Error: {e}")

    dg_conn.on(LiveTranscriptionEvents.Transcript, on_message)
    await dg_conn.start(LiveOptions(
        model="nova-2", language="en-US", encoding="linear16", sample_rate=16000, channels=1, endpointing=True
    ))

    try:
        while True:
            data = await websocket.receive_json()
            if data.get("type") == "audio_chunk":
                raw_bytes = base64.b64decode(data["payload"])
                await dg_conn.send(raw_bytes)
    except Exception:
        pass
    finally:
        await dg_conn.finish()
        await websocket.close()


# ─── Monitor / Whisper Stream ───────────────────────────────────────────────

async def monitor_call(websocket: WebSocket, stream_sid: str):
    await websocket.accept()
    if stream_sid not in monitor_connections:
        monitor_connections[stream_sid] = set()
    monitor_connections[stream_sid].add(websocket)

    try:
        while True:
            data = await websocket.receive_json()
            if data.get("action") == "whisper":
                redis_store.push_whisper(stream_sid, data.get("text", ""))
            elif data.get("action") == "takeover":
                redis_store.set_takeover(stream_sid, True)
                if stream_sid in active_tts_tasks and not active_tts_tasks[stream_sid].done():
                    active_tts_tasks[stream_sid].cancel()
            elif data.get("action") == "audio_chunk" and redis_store.get_takeover(stream_sid):
                target_ws = twilio_websockets.get(stream_sid)
                if target_ws:
                    await target_ws.send_text(json.dumps({
                        "event": "media",
                        "streamSid": stream_sid,
                        "media": {"payload": data.get("payload")}
                    }))
    except Exception:
        pass
    finally:
        if stream_sid in monitor_connections and websocket in monitor_connections[stream_sid]:
            monitor_connections[stream_sid].remove(websocket)
