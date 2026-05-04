"""
tts.py — Text-to-Speech synthesis module for Callified AI Dialer.
Supports ElevenLabs, SmallestAI, and Sarvam AI (Bulbul v3) providers.
Handles audio format conversion (PCM, mulaw) and streaming to WebSocket.
"""
import os
import json
import base64
import asyncio
import httpx
from fastapi import WebSocket

# Module-level dict to collect TTS audio for recording per stream
_tts_recording_buffers: dict = {}


async def synthesize_and_send_audio(
    text: str, stream_sid: str, websocket: WebSocket,
    tts_provider_override: str = None, tts_voice_override: str = None, tts_language_override: str = None
):
    import logging
    tts_logger = logging.getLogger("uvicorn.error")
    tts_logger.info(f"TTS START: text='{text[:60]}...', sid={stream_sid}")
    is_browser_sim = stream_sid.startswith("web_sim_")
    is_exotel = not stream_sid.startswith("SM") and not is_browser_sim
    needs_raw_pcm = is_exotel or is_browser_sim

    tts_provider = (tts_provider_override or os.getenv("TTS_PROVIDER", "elevenlabs")).lower()

    if tts_provider == "sarvam":
        await _synthesize_sarvam(text, stream_sid, websocket, tts_voice_override, tts_language_override, needs_raw_pcm, tts_logger)
    elif tts_provider == "smallest":
        await _synthesize_smallest(text, stream_sid, websocket, tts_voice_override, tts_language_override, needs_raw_pcm, tts_logger)
    else:
        await _synthesize_elevenlabs(text, stream_sid, websocket, tts_voice_override, tts_language_override, needs_raw_pcm, is_exotel, is_browser_sim, tts_logger)


async def _synthesize_sarvam(text, stream_sid, websocket, tts_voice_override, tts_language_override, needs_raw_pcm, tts_logger):
    """Sarvam AI Bulbul v3 TTS via WebSocket streaming."""
    import websockets

    SARVAM_API_KEY = os.getenv("SARVAM_API_KEY", "")
    if not SARVAM_API_KEY:
        tts_logger.error("TTS Sarvam: SARVAM_API_KEY not set, falling back to ElevenLabs")
        await _synthesize_elevenlabs(text, stream_sid, websocket, tts_voice_override, tts_language_override, needs_raw_pcm, False, needs_raw_pcm, tts_logger)
        return

    voice = tts_voice_override or os.getenv("SARVAM_VOICE_ID", "aditya")
    lang = tts_language_override or "hi-IN"
    # Map short codes to Sarvam format
    lang_map = {"hi": "hi-IN", "en": "en-IN", "ta": "ta-IN", "te": "te-IN", "bn": "bn-IN",
                "gu": "gu-IN", "kn": "kn-IN", "ml": "ml-IN", "mr": "mr-IN", "pa": "pa-IN"}
    lang = lang_map.get(lang, lang)

    ws_url = "wss://api.sarvam.ai/text-to-speech/ws?model=bulbul:v3&send_completion_event=true"
    headers = {"Api-Subscription-Key": SARVAM_API_KEY}

    tts_logger.info(f"TTS: provider=Sarvam, voice={voice}, lang={lang}, needs_raw_pcm={needs_raw_pcm}")

    try:
        async with websockets.connect(ws_url, additional_headers=headers) as ws:
            # Send config
            config_msg = {
                "type": "config",
                "data": {
                    "model": "bulbul:v3",
                    "target_language_code": lang,
                    "speaker": voice,
                    "pace": 1.0,
                    "speech_sample_rate": "8000",
                    "output_audio_codec": "linear16",
                    "enable_preprocessing": True,
                    "min_buffer_size": 30,
                }
            }
            await ws.send(json.dumps(config_msg))

            # Send text
            await ws.send(json.dumps({"type": "text", "data": {"text": text}}))
            # Flush to ensure processing starts immediately
            await ws.send(json.dumps({"type": "flush"}))

            # Receive audio chunks
            chunk_count = 0
            async for msg in ws:
                data = json.loads(msg)
                if data.get("type") == "audio":
                    audio_b64 = data["data"]["audio"]
                    pcm_bytes = base64.b64decode(audio_b64)

                    if needs_raw_pcm:
                        _sarvam_is_exotel = not stream_sid.startswith("web_sim_") and not stream_sid.startswith("SM")
                        if _sarvam_is_exotel:
                            import audioop
                            send_bytes = audioop.lin2ulaw(pcm_bytes, 2)
                        else:
                            send_bytes = pcm_bytes
                        b64_chunk = base64.b64encode(send_bytes).decode('utf-8')
                        await websocket.send_text(json.dumps({
                            "event": "media",
                            "streamSid": stream_sid,
                            "media": {"payload": b64_chunk}
                        }))
                        if stream_sid in _tts_recording_buffers:
                            import time as _tts_t
                            _tts_recording_buffers[stream_sid].append((_tts_t.time(), pcm_bytes))
                    else:
                        import audioop
                        ulaw_chunk = audioop.lin2ulaw(pcm_bytes, 2)
                        b64_chunk = base64.b64encode(ulaw_chunk).decode('utf-8')
                        await websocket.send_text(json.dumps({
                            "event": "media",
                            "streamSid": stream_sid,
                            "media": {"payload": b64_chunk}
                        }))
                    chunk_count += 1

                elif data.get("type") == "event" and data.get("data", {}).get("event_type") == "final":
                    break
                elif data.get("type") == "error":
                    tts_logger.error(f"TTS Sarvam error: {data.get('data', {}).get('message', 'unknown')}")
                    break

            tts_logger.info(f"TTS Sarvam END: sent {chunk_count} chunks.")
    except asyncio.CancelledError:
        tts_logger.info("TTS Sarvam cancelled (barge-in)")
    except Exception as e:
        tts_logger.error(f"TTS Sarvam Exception: {e}")


async def _synthesize_smallest(text, stream_sid, websocket, tts_voice_override, tts_language_override, needs_raw_pcm, tts_logger):
    # SmallestAI language name map (lang code → API language name)
    _smallest_lang_map = {
        'hi': 'hindi', 'hi-IN': 'hindi',
        'bn': 'bengali', 'bn-IN': 'bengali',
        'en': 'english', 'en-IN': 'english', 'en-US': 'english',
        'mr': 'marathi', 'mr-IN': 'marathi',
        'ta': 'tamil', 'te': 'telugu', 'gu': 'gujarati',
        'kn': 'kannada', 'ml': 'malayalam', 'pa': 'punjabi',
    }
    url = "https://waves-api.smallest.ai/api/v1/lightning/get_speech"
    headers = {
        "Authorization": f"Bearer {os.getenv('SMALLEST_API_KEY')}",
        "Content-Type": "application/json"
    }
    payload = {
        "text": text,
        "voice_id": tts_voice_override or os.getenv("SMALLEST_VOICE_ID", "emily"),
        "sample_rate": 8000,
        "add_wav_header": False,
        "speed": 1.0,
    }
    if tts_language_override:
        lang_name = _smallest_lang_map.get(tts_language_override, tts_language_override)
        payload["language"] = lang_name
    tts_logger.info(f"TTS: provider=SmallestAI, lang={tts_language_override}, needs_raw_pcm={needs_raw_pcm}")
    try:
        async with httpx.AsyncClient(timeout=30.0) as client:
            async with client.stream("POST", url, json=payload, headers=headers) as response:
                if response.status_code != 200:
                    body = await response.aread()
                    tts_logger.error(f"TTS SmallestAI error: {body[:200]}")
                    return
                chunk_count = 0
                async for chunk in response.aiter_bytes(chunk_size=1024):
                    if chunk:
                        _sm_is_exotel = not stream_sid.startswith("web_sim_") and not stream_sid.startswith("SM")
                        if needs_raw_pcm:
                            if _sm_is_exotel:
                                import audioop
                                send_chunk = audioop.lin2ulaw(chunk, 2)
                            else:
                                send_chunk = chunk
                            b64_chunk = base64.b64encode(send_chunk).decode('utf-8')
                        else:
                            import audioop
                            ulaw_chunk = audioop.lin2ulaw(chunk, 2)
                            b64_chunk = base64.b64encode(ulaw_chunk).decode('utf-8')
                        payload_key = "streamSid"
                        await websocket.send_text(json.dumps({
                            "event": "media",
                            payload_key: stream_sid,
                            "media": {"payload": b64_chunk}
                        }))
                        chunk_count += 1
                tts_logger.info(f"TTS SmallestAI END: sent {chunk_count} chunks.")
    except Exception as e:
        tts_logger.error(f"TTS SmallestAI Exception: {e}")


def _downsample_pcm_2x(data: bytes) -> bytes:
    """Decimate 16-bit PCM by 2 (e.g. 16 kHz → 8 kHz) using no external libraries.
    Takes every other 16-bit little-endian sample — exact for a 2:1 integer ratio."""
    out = bytearray()
    for i in range(0, len(data) - 1, 4):
        out.extend(data[i:i + 2])
    return bytes(out)


async def _synthesize_elevenlabs(text, stream_sid, websocket, tts_voice_override, tts_language_override, needs_raw_pcm, is_exotel, is_browser_sim, tts_logger):
    if needs_raw_pcm:
        output_format = "pcm_16000"
    else:
        output_format = "ulaw_8000"

    url = (
        f"https://api.elevenlabs.io/v1/text-to-speech/"
        f"{tts_voice_override or os.getenv('ELEVENLABS_VOICE_ID')}/stream?output_format={output_format}"
    )
    # eleven_turbo_v2_5 only supports a limited set of language codes.
    # For unsupported Indian languages (te, kn, ml, gu, pa, bn, mr) use
    # eleven_multilingual_v2 without a language_code — it auto-detects from script.
    _EL_TURBO_LANGS = {'en', 'hi', 'ta', 'de', 'pl', 'es', 'it', 'fr', 'pt', 'ar',
                       'cs', 'ro', 'hu', 'el', 'ko', 'ms', 'id', 'ja', 'tr', 'zh',
                       'nl', 'fi', 'uk', 'bg', 'hr', 'sk', 'da', 'sv', 'tl'}
    _lang = tts_language_override or "hi"
    if _lang in _EL_TURBO_LANGS:
        _el_model = "eleven_turbo_v2_5"
        _el_lang_param = {"language_code": _lang}
    else:
        _el_model = "eleven_multilingual_v2"
        _el_lang_param = {}

    headers = {"xi-api-key": os.getenv("ELEVENLABS_API_KEY")}
    payload = {
        "text": text,
        "model_id": _el_model,
        **_el_lang_param,
        "voice_settings": {
            "stability": 0.35,
            "similarity_boost": 0.85,
            "style": 0.1,
            "use_speaker_boost": True
        },
    }
    tts_logger.info(f"TTS: provider=ElevenLabs, is_exotel={is_exotel}, is_browser_sim={is_browser_sim}, format={output_format}")

    try:
        async with httpx.AsyncClient(timeout=30.0) as client:
            async with client.stream("POST", url, json=payload, headers=headers) as response:
                if response.status_code != 200:
                    body = await response.aread()
                    tts_logger.error(f"TTS ElevenLabs HTTP {response.status_code} for voice={tts_voice_override}: {body[:300]}")
                    if response.status_code == 402 and stream_sid.startswith("web_sim_"):
                        # Library voice requires paid ElevenLabs plan — fall back to SmallestAI.
                        # Map each ElevenLabs voice to a distinct SmallestAI voice (hindi, english).
                        _EL_TO_SMALLEST = {
                            # (lowercase el id): (hindi_voice, english_voice)
                            'oh8ymzxjyezq5scgogn9': ('raj',     'arman'),    # Aakash
                            'x4exprixdkrwchdtgysh': ('priya',   'jasmine'),  # Anjura (F)
                            'sxukwbhkoioahklf6gt3': ('aravind', 'james'),    # Gaurav
                            'n09nfwyjjg9vssgdlqbt': ('raj',     'arman'),    # Ishan
                            'u9wnm2bnanqtbcawwlga': ('aravind', 'james'),    # Himanshu
                            'h061kgyotplydxcoi8e3': ('aravind', 'james'),    # Ravi
                            'ock0al5dbkvtudept4hm': ('raj',     'arman'),    # Viraj
                            'nwj0s2lu9bdwrknd5yza': ('raj',     'arman'),    # Bunty
                            'amiaxapsdoaihjqbsazj': ('priya',   'emily'),    # Priya (F)
                            '6jsmtroalvewg1ga6jmw': ('mithali', 'emily'),    # Sia (F)
                            '9vp6r7vvxnwgiglnpl17': ('mithali', 'jasmine'),  # Suhana (F)
                            'ho2yz8lxm3axuxl8oekx': ('mithali', 'jasmine'),  # Mini (F)
                            's0oisosj9raium7djnzw': ('raj',     'james'),    # Default
                        }
                        _el_lower = (tts_voice_override or "").lower()
                        _lang = (tts_language_override or "hi").lower()
                        _pair = _EL_TO_SMALLEST.get(_el_lower)
                        if _pair:
                            _fallback_voice = _pair[1] if _lang == "en" else _pair[0]
                        else:
                            _fallback_voice = "james" if _lang == "en" else "raj"
                        tts_logger.warning(f"TTS ElevenLabs 402 → SmallestAI fallback voice={_fallback_voice} (lang={_lang})")
                        try:
                            await websocket.send_text(json.dumps({
                                "event": "tts_warning",
                                "message": "This ElevenLabs voice requires a paid subscription. Falling back to SmallestAI for audio playback.",
                            }))
                        except Exception:
                            pass
                        await _synthesize_smallest(text, stream_sid, websocket, _fallback_voice, tts_language_override, needs_raw_pcm, tts_logger)
                        return
                    if stream_sid.startswith("web_sim_"):
                        try:
                            err_text = body[:300].decode('utf-8', errors='replace')
                            await websocket.send_text(json.dumps({
                                "event": "tts_error",
                                "provider": "elevenlabs",
                                "code": response.status_code,
                                "error": err_text,
                            }))
                        except Exception:
                            pass
                    return
                chunk_count = 0
                pcm_buffer = b""

                async for chunk in response.aiter_bytes(chunk_size=1280):
                    if chunk:
                        if needs_raw_pcm:
                            pcm_buffer += chunk
                            # Send in 1280-byte aligned blocks (16kHz→8kHz via 2:1 decimation)
                            while len(pcm_buffer) >= 1280:
                                raw = pcm_buffer[:1280]
                                pcm_buffer = pcm_buffer[1280:]
                                downsampled = _downsample_pcm_2x(raw)
                                if is_exotel:
                                    import audioop
                                    send_bytes = audioop.lin2ulaw(downsampled, 2)
                                else:
                                    send_bytes = downsampled
                                b64_chunk = base64.b64encode(send_bytes).decode('utf-8')
                                await websocket.send_text(json.dumps({
                                    "event": "media",
                                    "streamSid": stream_sid,
                                    "media": {"payload": b64_chunk}
                                }))
                                if stream_sid in _tts_recording_buffers:
                                    import time as _tts_t
                                    _tts_recording_buffers[stream_sid].append((_tts_t.time(), downsampled))
                                chunk_count += 1
                                await asyncio.sleep(0.020)
                        else:
                            await websocket.send_text(json.dumps({
                                "event": "media",
                                "streamSid": stream_sid,
                                "media": {"payload": base64.b64encode(chunk).decode('utf-8')}
                            }))
                            await asyncio.sleep(0.070)
                            chunk_count += 1

                # Flush remaining bytes so the end of the phrase is never silently dropped
                if needs_raw_pcm and len(pcm_buffer) >= 4:
                    usable = len(pcm_buffer) - (len(pcm_buffer) % 4)
                    if usable:
                        downsampled = _downsample_pcm_2x(pcm_buffer[:usable])
                        if is_exotel:
                            import audioop
                            send_bytes = audioop.lin2ulaw(downsampled, 2)
                        else:
                            send_bytes = downsampled
                        b64_chunk = base64.b64encode(send_bytes).decode('utf-8')
                        await websocket.send_text(json.dumps({
                            "event": "media",
                            "streamSid": stream_sid,
                            "media": {"payload": b64_chunk}
                        }))
                        chunk_count += 1

                tts_logger.info(f"TTS ElevenLabs END: sent {chunk_count} chunks.")
    except asyncio.CancelledError:
        tts_logger.info("TTS ElevenLabs cancelled (barge-in)")
    except Exception as e:
        tts_logger.error(f"TTS ElevenLabs Exception: {e}", exc_info=True)
        if stream_sid.startswith("web_sim_"):
            try:
                await websocket.send_text(json.dumps({
                    "event": "tts_error",
                    "provider": "elevenlabs",
                    "code": 0,
                    "error": str(e),
                }))
            except Exception:
                pass
