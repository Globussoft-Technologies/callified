import { INDIAN_VOICES } from '../constants/voices.js';

// Legacy campaigns may still return providers no longer offered in the editor.
// Keep the draft and the saved payload consistent with its Gemini-only choices.
export function geminiCampaignVoice(settings = {}) {
  const voices = INDIAN_VOICES.gemini_live;
  const validVoice = settings.tts_provider === 'gemini_live'
    && voices.some(voice => voice.id === settings.tts_voice_id);
  return {
    ...settings,
    tts_provider: 'gemini_live',
    tts_voice_id: validVoice ? settings.tts_voice_id : voices[0].id,
  };
}
