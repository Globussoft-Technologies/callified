import os
import sys
import pytest
from unittest.mock import AsyncMock, patch, MagicMock

sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "..")))

import llm_provider

# Helper to configure the global llm_provider LLM_PROVIDER variable dynamically
@pytest.fixture
def override_provider(monkeypatch):
    def _override(provider: str):
        # We also need to patch the os.getenv evaluation if it happens dynamically,
        # but llm_provider evaluates it globally. We mutate the module directly.
        llm_provider.LLM_PROVIDER = provider
    return _override

@pytest.mark.asyncio
async def test_groq_generate_success(override_provider):
    override_provider("groq")
    
    with patch("groq.AsyncGroq") as MockGroq:
        mock_client = MagicMock()
        mock_response = AsyncMock()
        mock_response.parse.return_value = MagicMock(choices=[MagicMock(message=MagicMock(content="Mocked Groq Response"))])
        
        mock_client.chat.completions.with_raw_response.create = AsyncMock(return_value=mock_response)
        MockGroq.return_value = mock_client
        
        history = [{"role": "user", "parts": [{"text": "Hello"}]}]
        res = await llm_provider.generate_response(history, "Test Instruction", 150)
        
        assert "Mocked Groq Response" in res
        MockGroq.assert_called_once()

@pytest.mark.asyncio
async def test_gemini_generate_success(override_provider):
    override_provider("gemini")
    
    with patch("google.genai.Client") as MockGemini:
        mock_client = MagicMock()
        mock_response = AsyncMock()
        mock_response.text = "Mocked Gemini Response"
        
        mock_client.aio.models.generate_content = AsyncMock(return_value=mock_response)
        MockGemini.return_value = mock_client
        
        history = [{"role": "user", "parts": [{"text": "Hello"}]}]
        res = await llm_provider.generate_response(history, "Test Instruction", 150)
        
        assert "Mocked Gemini Response" in res
        MockGemini.assert_called_once()

@pytest.mark.asyncio
async def test_groq_alias_groc_failover(override_provider):
    override_provider("groc") # The accidental typo case verified earlier
    
    with patch("groq.AsyncGroq") as MockGroq:
        mock_client = MagicMock()
        mock_response = AsyncMock()
        mock_response.parse.return_value = MagicMock(choices=[MagicMock(message=MagicMock(content="Routed to Groq despite typo"))])
        
        mock_client.chat.completions.with_raw_response.create = AsyncMock(return_value=mock_response)
        MockGroq.return_value = mock_client
        
        res = await llm_provider.generate_response([], "Test", 150)
        assert "Routed to Groq" in res
        MockGroq.assert_called_once()

@pytest.mark.asyncio
async def test_groq_rate_limit_fallback_to_gemini(override_provider, caplog):
    override_provider("groq")
    
    with patch("groq.AsyncGroq") as MockGroq, patch("google.genai.Client") as MockGemini:
        mock_groq_client = MagicMock()
        
        # Simulate an HTTP 429 Rate Limit exception
        mock_groq_client.chat.completions.with_raw_response.create = AsyncMock(side_effect=Exception("HTTP 429: rate_limit exceeded"))
        MockGroq.return_value = mock_groq_client
        
        mock_gemini_client = MagicMock()
        mock_gemini_response = AsyncMock()
        mock_gemini_response.text = "Gemini Fallback Response"
        mock_gemini_client.aio.models.generate_content = AsyncMock(return_value=mock_gemini_response)
        MockGemini.return_value = mock_gemini_client
        
        res = await llm_provider.generate_response([], "Test", 150)
        
        assert "Gemini Fallback Response" in res
        assert "Groq rate limited, falling back to Gemini" in caplog.text
        MockGroq.assert_called_once()
        MockGemini.assert_called_once()

@pytest.mark.asyncio
async def test_generate_response_stream_groq(override_provider):
    override_provider("groq")
    
    with patch("groq.AsyncGroq") as MockGroq:
        mock_client = MagicMock()
        
        # We must return an async generator from an awaited coroutine
        async def mock_stream_chunks():
            yield MagicMock(choices=[MagicMock(delta=MagicMock(content="Hello "))])
            yield MagicMock(choices=[MagicMock(delta=MagicMock(content="streaming world!"))])
            
        mock_client.chat.completions.create = AsyncMock(return_value=mock_stream_chunks())
        MockGroq.return_value = mock_client
        
        chunks = []
        async for chunk in llm_provider.generate_response_stream([], "Test", 150):
            chunks.append(chunk)
            
        assert chunks == ["Hello ", "streaming world!"]
