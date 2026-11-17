import test from 'node:test';
import assert from 'node:assert/strict';
import { geminiCampaignVoice } from './campaignVoice.js';
import { INDIAN_VOICES } from '../constants/voices.js';

test('legacy campaign settings select a valid Gemini voice without losing language or duration', () => {
  const legacy = { tts_provider: 'elevenlabs', tts_voice_id: 'default', tts_language: 'hi', max_call_duration_seconds: 300 };
  const result = geminiCampaignVoice(legacy);
  assert.equal(result.tts_provider, 'gemini_live');
  assert.ok(INDIAN_VOICES.gemini_live.some(voice => voice.id === result.tts_voice_id));
  assert.equal(result.tts_language, 'hi');
  assert.equal(result.max_call_duration_seconds, 300);
  assert.equal(legacy.tts_provider, 'elevenlabs');
});

test('Gemini selections survive normalization and empty defaults offer Gemini', () => {
  const settings = { tts_provider: 'gemini_live', tts_voice_id: 'Kore' };
  assert.deepEqual(geminiCampaignVoice(settings), settings);
  assert.equal(geminiCampaignVoice().tts_provider, 'gemini_live');
  assert.ok(geminiCampaignVoice({ ...settings, tts_voice_id: 'old-voice' }).tts_voice_id !== 'old-voice');
});
