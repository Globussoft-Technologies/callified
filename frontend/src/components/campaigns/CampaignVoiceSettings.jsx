import React from 'react';
import { InfoCircleOutlined, SoundOutlined } from '@ant-design/icons';
import AppSelect from '../common/AppSelect';
import { INDIAN_LANGUAGES, INDIAN_VOICES, VOICE_RECOMMENDATIONS } from '../../constants/voices';

const T = { accent: '#6366f1', green: '#10b981', red: '#ef4444', muted: 'var(--text-muted)', border: 'var(--border)', font: "'DM Sans', sans-serif" };

function voiceOptions(provider, language) {
  const recommendedIds = VOICE_RECOMMENDATIONS[language]?.[provider]?.top || [];
  const voices = INDIAN_VOICES[provider] || [];
  const recommended = voices.filter(voice => recommendedIds.includes(voice.id));
  const others = voices.filter(voice => !recommendedIds.includes(voice.id));
  const toOptions = list => list.map(voice => ({ value: voice.id, label: voice.name }));
  return recommended.length ? [
    { label: 'Recommended', options: toOptions(recommended) },
    { label: 'All Voices', options: toOptions(others) },
  ] : toOptions(voices);
}

export default function CampaignVoiceSettings({ voice, onChange, canSave = false, saveStatus, onSave, onReset, tourExample = false }) {
  const provider = voice.tts_provider;
  const maxMinutes = Number(voice.max_call_duration_seconds || 0) / 60;
  const providerLabel = provider === 'elevenlabs' ? 'ElevenLabs' : provider === 'sarvam' ? 'Sarvam AI' : provider === 'gemini_live' ? 'Gemini Live' : 'Smallest AI';
  const voiceLabel = (INDIAN_VOICES[provider] || []).find(v => v.id === voice.tts_voice_id)?.name || voice.tts_voice_id || 'none';
  const langLabel = INDIAN_LANGUAGES.find(l => l.code === voice.tts_language)?.name || voice.tts_language;
  return <div data-tour={tourExample ? undefined : 'campaign-voice-settings'} className="campaign-voice-settings-panel" style={{ background: 'var(--surface-raised)', border: `1px solid ${T.border}`, borderRadius: 12, boxShadow: '0 1px 3px rgba(0,0,0,0.04)', marginBottom: 16, padding: '14px 18px' }}>
    <div className="campaign-voice-tour-heading" style={{ display: 'flex', width: 'fit-content', alignItems: 'center', gap: 6, fontSize: 12, color: T.muted, fontWeight: 700, whiteSpace: 'nowrap', textTransform: 'uppercase', letterSpacing: '0.05em', marginBottom: 10 }}><SoundOutlined /> Voice Settings</div>
    <div className="campaign-voice-controls">
      <div className="campaign-voice-selects">
        <AppSelect size="small" value={provider || undefined} placeholder="Provider" options={[{ value: 'gemini_live', label: 'Gemini Live' }]} onChange={value => onChange?.({ ...voice, tts_provider: value, tts_voice_id: (INDIAN_VOICES[value] || [])[0]?.id || '' })} />
        <AppSelect searchable size="small" value={voice.tts_voice_id || undefined} placeholder="Voice" popupWidth={260} options={voiceOptions(provider, voice.tts_language)} onChange={value => onChange?.({ ...voice, tts_voice_id: value })} />
        <AppSelect searchable size="small" value={voice.tts_language || undefined} placeholder="Language" options={INDIAN_LANGUAGES.map(language => ({ value: language.code, label: language.name }))} onChange={value => onChange?.({ ...voice, tts_language: value })} />
      </div>
      <label style={{ display: 'inline-flex', alignItems: 'center', gap: 6, fontSize: 12, color: T.muted, fontWeight: 700, whiteSpace: 'nowrap' }}>
        Max Call Time
        <input className="form-input" type="number" min="0" max="60" step="1" value={maxMinutes ? Math.round(maxMinutes) : ''} readOnly={tourExample} onChange={event => {
          const minutes = Math.max(0, Math.min(60, Number(event.target.value || 0)));
          onChange?.({ ...voice, max_call_duration_seconds: minutes ? minutes * 60 : 0 });
        }} placeholder="No limit" style={{ padding: '6px 10px', border: `1px solid ${T.border}`, borderRadius: 8, height: 32, width: 92 }} />
        min
      </label>
      {(canSave || tourExample) && <button type="button" style={{ background: saveStatus === 'saved' ? T.green : saveStatus === 'error' ? T.red : T.accent, border: 'none', color: '#fff', fontSize: 12, padding: '6px 14px', borderRadius: 8, cursor: saveStatus === 'saving' ? 'wait' : 'pointer', whiteSpace: 'nowrap', opacity: saveStatus === 'saving' ? 0.7 : 1, fontWeight: 600, fontFamily: T.font }} disabled={saveStatus === 'saving'} onClick={onSave}>{saveStatus === 'saving' ? 'Saving…' : saveStatus === 'saved' ? '✓ Saved' : saveStatus === 'error' ? '✗ Failed' : 'Save'}</button>}
      {(canSave || tourExample) && <button type="button" style={{ background: 'var(--surface-raised)', border: `1px solid ${T.border}`, color: 'var(--text-secondary)', borderRadius: 8, padding: '6px 14px', fontSize: 12, fontWeight: 600, fontFamily: T.font }} onClick={onReset}>Reset to Org Default</button>}
    </div>
    <div style={{ fontSize: '0.7rem', color: T.accent, marginTop: 6 }}>{provider ? `Current: ${providerLabel} - ${voiceLabel}${langLabel ? ` (${langLabel})` : ''}${maxMinutes > 0 ? ` · Max call time ${Math.round(maxMinutes)} min` : ' · No max limit'}` : 'Using org default'}</div>
    {VOICE_RECOMMENDATIONS[voice.tts_language]?.[provider]?.note && <div style={{ fontSize: '0.65rem', color: '#0891b2', marginTop: 4 }}><InfoCircleOutlined /> {VOICE_RECOMMENDATIONS[voice.tts_language][provider].note}</div>}
  </div>;
}
