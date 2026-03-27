import os
import sys
import json
import asyncio
import pytest
from unittest.mock import MagicMock, patch, AsyncMock

# Virtualize audioop globally to avoid C extension missing issues on Windows Py3.13+
sys.modules['audioop'] = MagicMock()
import audioop
# Mock specific ratecv and lin2ulaw functions to return dummy values without throwing
audioop.ratecv.return_value = (b"PCM_DOWNSAMPLED", None)
audioop.lin2ulaw.return_value = b"ULAW_PAYLOAD"

sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "..")))

import tts

@pytest.fixture
def mock_websocket():
    mock_ws = AsyncMock()
    mock_ws.send_text = AsyncMock()
    return mock_ws

@pytest.mark.asyncio
async def test_synthesize_smallest_pcm(mock_websocket):
    with patch("httpx.AsyncClient") as MockClient:
        mock_client_instance = MagicMock()
        mock_response = AsyncMock()
        mock_response.status_code = 200
        
        async def mock_audio_stream(*args, **kwargs):
            yield b"PCM_DATA_CHUNK_1"
            yield b"PCM_DATA_CHUNK_2"
            
        mock_response.aiter_bytes = mock_audio_stream
        
        mock_stream_ctx = MagicMock()
        mock_stream_ctx.__aenter__.return_value = mock_response
        mock_stream_ctx.__aexit__.return_value = None
        mock_client_instance.stream.return_value = mock_stream_ctx
        
        mock_client_ctx = MagicMock()
        mock_client_ctx.__aenter__.return_value = mock_client_instance
        mock_client_ctx.__aexit__.return_value = None
        MockClient.return_value = mock_client_ctx

        await tts.synthesize_and_send_audio("Hello", "call_123", mock_websocket, tts_provider_override="smallest")
        
        # Verify websocket got exact 2 messages
        assert mock_websocket.send_text.call_count == 2
        args, _ = mock_websocket.send_text.call_args_list[0]
        payload = json.loads(args[0])
        assert payload["event"] == "media"
        assert payload["stream_sid"] == "call_123"

@pytest.mark.asyncio
async def test_synthesize_elevenlabs_pcm(mock_websocket):
    with patch("httpx.AsyncClient") as MockClient:
        mock_client_instance = MagicMock()
        mock_response = AsyncMock()
        mock_response.status_code = 200
        
        async def mock_audio_stream(*args, **kwargs):
            yield b"\x00" * 2500
            
        mock_response.aiter_bytes = mock_audio_stream
        
        mock_stream_ctx = MagicMock()
        mock_stream_ctx.__aenter__.return_value = mock_response
        mock_stream_ctx.__aexit__.return_value = None
        mock_client_instance.stream.return_value = mock_stream_ctx
        
        mock_client_ctx = MagicMock()
        mock_client_ctx.__aenter__.return_value = mock_client_instance
        mock_client_ctx.__aexit__.return_value = None
        MockClient.return_value = mock_client_ctx

        await tts.synthesize_and_send_audio("Hello Labs", "call_456", mock_websocket, tts_provider_override="elevenlabs")
        
        assert mock_websocket.send_text.call_count >= 1
        args, _ = mock_websocket.send_text.call_args_list[0]
        payload = json.loads(args[0])
        assert payload["stream_sid"] == "call_456"

@pytest.mark.asyncio
async def test_synthesize_elevenlabs_http_failure(mock_websocket, caplog):
    with patch("httpx.AsyncClient") as MockClient:
        mock_client_instance = MagicMock()
        mock_response = AsyncMock()
        mock_response.status_code = 401
        
        async def mock_aread():
            return b'{"detail": "Unauthorized"}'
        mock_response.aread = mock_aread
        
        mock_stream_ctx = MagicMock()
        mock_stream_ctx.__aenter__.return_value = mock_response
        mock_stream_ctx.__aexit__.return_value = None
        mock_client_instance.stream.return_value = mock_stream_ctx
        
        mock_client_ctx = MagicMock()
        mock_client_ctx.__aenter__.return_value = mock_client_instance
        mock_client_ctx.__aexit__.return_value = None
        MockClient.return_value = mock_client_ctx
        
        await tts.synthesize_and_send_audio("Bad Auth", "call_789", mock_websocket, tts_provider_override="elevenlabs")
        
        assert mock_websocket.send_text.call_count == 0
