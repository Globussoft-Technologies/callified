import React from 'react';
import { formatDate } from '../../utils/dateFormat';
import { useToast } from '../../contexts/ToastContext';
import { INDIAN_VOICES, INDIAN_LANGUAGES } from '../../constants/voices';

const PROMPT_SOFT_WARN = 6000;  // chars — yellow warning zone
const PROMPT_HARD_WARN = 8000;  // chars — red, AI may truncate

export default function SettingsTab({
  handleAddPronunciation, pronFormData, setPronFormData, pronunciations, handleDeletePronunciation,
  pronError, setPronError,
  selectedOrg,
  promptDirty, handleSaveSystemPrompt, promptSaving, promptSaveStatus, systemPromptAuto, systemPromptCustom,
  setSystemPromptCustom, setPromptDirty,
  orgTimezone,
  activeVoiceProvider, activeVoiceId, activeLanguage,
  handleSaveOrgVoice, voiceSaving
}) {
  const { showToast } = useToast();
  const [localProvider, setLocalProvider] = React.useState(activeVoiceProvider || 'elevenlabs');
  const [localVoiceId, setLocalVoiceId] = React.useState(activeVoiceId || '');
  const [localLanguage, setLocalLanguage] = React.useState(activeLanguage || 'hi');
  const [confirmDeletePronId, setConfirmDeletePronId] = React.useState(null);

  React.useEffect(() => {
    if (activeVoiceProvider) setLocalProvider(activeVoiceProvider);
    if (activeVoiceId) setLocalVoiceId(activeVoiceId);
    if (activeLanguage) setLocalLanguage(activeLanguage);
  }, [activeVoiceProvider, activeVoiceId, activeLanguage]);

  const handleProviderChange = (p) => {
    setLocalProvider(p);
    const firstVoice = INDIAN_VOICES[p]?.[0];
    if (firstVoice) setLocalVoiceId(firstVoice.id);
  };

  const handleVoiceSave = async () => {
    const voices = INDIAN_VOICES[localProvider] || [];
    const found = voices.find(v => v.id === localVoiceId);
    const ok = await handleSaveOrgVoice({ provider: localProvider, voiceId: localVoiceId, language: localLanguage, voiceName: found?.name || '' });
    if (ok) showToast('Voice & language settings saved!');
    else showToast('Failed to save voice settings', 'error');
  };
  return (
    <div style={{padding: '1rem', maxWidth: '800px', margin: '0 auto'}}>
      <div className="wa-header" style={{borderBottom: '1px solid rgba(255,255,255,0.05)', marginBottom: '2rem'}}>
        <h3><span style={{color: '#f59e0b'}}>AI Voice</span> Settings</h3>
        <p>Configure how the AI pronounces product names, brand names, and technical terms during calls.</p>
      </div>

      {/* Voice & Language Settings */}
      {selectedOrg && (
        <div className="glass-panel" style={{marginBottom: '2rem'}}>
          <h4 style={{marginTop: 0, marginBottom: '0.5rem', fontSize: '1.1rem', fontWeight: 600}}>🎙️ Voice & Language Settings</h4>
          <p style={{color: '#94a3b8', fontSize: '0.85rem', marginBottom: '1.5rem'}}>
            These settings apply to CRM sim web calls and as the default for new campaigns.
          </p>
          <div style={{display: 'flex', gap: '1rem', flexWrap: 'wrap'}}>
            {/* Provider */}
            <div style={{flex: 1, minWidth: '140px'}}>
              <label style={{display: 'block', fontSize: '0.78rem', color: '#64748b', fontWeight: 600, marginBottom: '6px', textTransform: 'uppercase', letterSpacing: '0.05em'}}>Provider</label>
              <div style={{display: 'flex', gap: '6px'}}>
                {Object.keys(INDIAN_VOICES).map(key => (
                  <button key={key} onClick={() => handleProviderChange(key)}
                    style={{flex: 1, padding: '8px 10px', borderRadius: '8px', cursor: 'pointer', fontWeight: 600, fontSize: '0.8rem', border: 'none',
                      background: localProvider === key ? 'linear-gradient(135deg, #8b5cf6, #6d28d9)' : 'rgba(255,255,255,0.05)',
                      color: localProvider === key ? '#fff' : '#94a3b8', transition: 'all 0.2s', textTransform: 'capitalize'}}>
                    {key === 'elevenlabs' ? 'ElevenLabs' : key === 'smallest' ? 'Smallest AI' : 'Sarvam'}
                  </button>
                ))}
              </div>
            </div>
          </div>
          <div style={{display: 'flex', gap: '1rem', marginTop: '1rem', flexWrap: 'wrap'}}>
            {/* Voice */}
            <div style={{flex: 2, minWidth: '200px'}}>
              <label style={{display: 'block', fontSize: '0.78rem', color: '#64748b', fontWeight: 600, marginBottom: '6px', textTransform: 'uppercase', letterSpacing: '0.05em'}}>Voice</label>
              <select className="form-input" value={localVoiceId} onChange={e => setLocalVoiceId(e.target.value)} style={{width: '100%', fontSize: '0.9rem'}}>
                {(INDIAN_VOICES[localProvider] || []).map(v => (
                  <option key={v.id} value={v.id}>{v.name}</option>
                ))}
              </select>
            </div>
            {/* Language */}
            <div style={{flex: 1, minWidth: '140px'}}>
              <label style={{display: 'block', fontSize: '0.78rem', color: '#64748b', fontWeight: 600, marginBottom: '6px', textTransform: 'uppercase', letterSpacing: '0.05em'}}>Language</label>
              <select className="form-input" value={localLanguage} onChange={e => setLocalLanguage(e.target.value)} style={{width: '100%', fontSize: '0.9rem'}}>
                {INDIAN_LANGUAGES.map(l => (
                  <option key={l.code} value={l.code}>{l.name}</option>
                ))}
              </select>
            </div>
          </div>
          <div style={{marginTop: '1rem'}}>
            <button className="btn-primary"
              style={{background: 'linear-gradient(135deg, #10b981, #059669)', fontSize: '0.9rem', padding: '10px 24px'}}
              onClick={handleVoiceSave} disabled={voiceSaving}>
              {voiceSaving ? '⏳ Saving...' : '💾 Save Voice & Language'}
            </button>
          </div>
        </div>
      )}

      <div className="glass-panel" style={{marginBottom: '2rem'}}>
        <h4 style={{marginTop: 0, marginBottom: '1.5rem', fontSize: '1.1rem', fontWeight: 600}}>🗣️ Pronunciation Guide</h4>
        <p style={{color: '#94a3b8', fontSize: '0.9rem', marginBottom: '1.5rem'}}>
          Teach the AI how to speak your product names correctly. The AI will use the phonetic version in conversations.
        </p>

        <form onSubmit={handleAddPronunciation} style={{marginBottom: '2rem'}}>
          <div style={{display: 'flex', gap: '12px', alignItems: 'flex-start'}}>
            <div className="form-group" style={{marginBottom: 0, flex: 1}}>
              <label>Written Word</label>
              <input
                className="form-input"
                value={pronFormData.word}
                onChange={e => {
                  setPronFormData({...pronFormData, word: e.target.value});
                  if (pronError?.word) setPronError(p => ({...p, word: ''}));
                }}
                placeholder="e.g. Adsgpt"
                maxLength={100}
                data-testid="pron-word"
                style={pronError?.word ? {borderColor: 'rgba(239,68,68,0.6)', boxShadow: '0 0 0 3px rgba(239,68,68,0.15)'} : undefined}
              />
              {pronError?.word && (
                <p style={{margin: '4px 0 0', fontSize: '0.78rem', color: '#fca5a5'}}>{pronError.word}</p>
              )}
            </div>
            <div style={{fontSize: '1.5rem', color: '#64748b', paddingTop: '34px'}}>→</div>
            <div className="form-group" style={{marginBottom: 0, flex: 1}}>
              <label>How to Pronounce</label>
              <input
                className="form-input"
                value={pronFormData.phonetic}
                onChange={e => {
                  setPronFormData({...pronFormData, phonetic: e.target.value});
                  if (pronError?.phonetic) setPronError(p => ({...p, phonetic: ''}));
                }}
                placeholder="e.g. Ads G P T"
                maxLength={200}
                data-testid="pron-phonetic"
                style={pronError?.phonetic ? {borderColor: 'rgba(239,68,68,0.6)', boxShadow: '0 0 0 3px rgba(239,68,68,0.15)'} : undefined}
              />
              {pronError?.phonetic && (
                <p style={{margin: '4px 0 0', fontSize: '0.78rem', color: '#fca5a5'}}>{pronError.phonetic}</p>
              )}
            </div>
            <button data-testid="add-rule-btn" type="submit" className="btn-primary" style={{height: '46px', padding: '0 20px', whiteSpace: 'nowrap', marginTop: '28px', flexShrink: 0}}>
              + Add Rule
            </button>
          </div>
          {pronError?.api && (
            <div style={{
              marginTop: '12px', padding: '10px 14px', borderRadius: '8px',
              background: 'rgba(239,68,68,0.1)', border: '1px solid rgba(239,68,68,0.3)',
              color: '#fca5a5', fontSize: '0.85rem',
            }}>
              {pronError.api}
            </div>
          )}
        </form>

        {pronunciations.length === 0 ? (
          <div style={{padding: '2rem', textAlign: 'center', color: '#64748b', background: 'rgba(0,0,0,0.2)', borderRadius: '8px'}}>
            No pronunciation rules added yet. Add one above to get started!
          </div>
        ) : (
          <table className="leads-table">
            <thead>
              <tr>
                <th>Written Word</th>
                <th>AI Says</th>
                <th>Added</th>
                <th>Action</th>
              </tr>
            </thead>
            <tbody>
              {pronunciations.map(p => (
                <tr key={p.id}>
                  <td style={{fontWeight: 600, color: '#e2e8f0', fontFamily: 'monospace'}}>{p.word}</td>
                  <td style={{color: '#4ade80', fontStyle: 'italic'}}>🔊 "{p.phonetic}"</td>
                  <td style={{color: '#94a3b8', fontSize: '0.85rem'}}>{formatDate(p.created_at, orgTimezone)}</td>
                  <td>
                    {confirmDeletePronId === p.id ? (
                      <span style={{display: 'flex', gap: '6px', alignItems: 'center'}}>
                        <span style={{fontSize: '0.8rem', color: '#fca5a5'}}>Remove?</span>
                        <button onClick={() => { handleDeletePronunciation(p.id); setConfirmDeletePronId(null); }}
                          style={{background: '#ef4444', border: 'none', color: '#fff', borderRadius: '5px', padding: '3px 10px', cursor: 'pointer', fontSize: '0.75rem', fontWeight: 600}}>Yes</button>
                        <button onClick={() => setConfirmDeletePronId(null)}
                          style={{background: 'transparent', border: '1px solid rgba(255,255,255,0.15)', color: '#94a3b8', borderRadius: '5px', padding: '3px 10px', cursor: 'pointer', fontSize: '0.75rem'}}>No</button>
                      </span>
                    ) : (
                      <button
                        className="btn-call"
                        style={{background: 'rgba(239, 68, 68, 0.15)', color: '#ef4444', borderColor: 'rgba(239, 68, 68, 0.3)', padding: '4px 12px', fontSize: '0.8rem'}}
                        onClick={() => setConfirmDeletePronId(p.id)}
                      >
                        🗑️ Remove
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      <div className="glass-panel" style={{background: 'rgba(245, 158, 11, 0.05)', border: '1px solid rgba(245, 158, 11, 0.15)'}}>
        <h4 style={{marginTop: 0, color: '#f59e0b', fontSize: '0.95rem'}}>💡 How it works</h4>
        <p style={{color: '#94a3b8', fontSize: '0.85rem', margin: 0, lineHeight: 1.7}}>
          The pronunciation guide is injected into the AI's prompt at the start of every call.
          When the AI generates a response containing a mapped word, it will use the phonetic version instead.
          The TTS engine then speaks the phonetic text, resulting in correct pronunciation.
          <br/><br/>
          <strong style={{color: '#e2e8f0'}}>Example:</strong> If you add "Adsgpt" → "Ads G P T", the AI will say "Ads G P T" instead of trying to sound out "Adsgpt".
        </p>
      </div>



      {/* System Prompt Preview & Edit */}
      {selectedOrg && (
        <div className="glass-panel" style={{marginBottom: '2rem'}}>
          <div style={{display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1rem'}}>
            <h4 style={{marginTop: 0, marginBottom: 0, fontSize: '1.1rem', fontWeight: 600}}>🤖 AI System Prompt</h4>
            <div style={{display: 'flex', gap: '8px', alignItems: 'center'}}>
              {promptSaveStatus === 'saved' && (
                <span style={{fontSize: '0.8rem', color: '#22c55e', background: 'rgba(34,197,94,0.1)', border: '1px solid rgba(34,197,94,0.3)', padding: '4px 10px', borderRadius: '6px'}}>✓ Saved</span>
              )}
              {promptSaveStatus === 'error' && (
                <span style={{fontSize: '0.8rem', color: '#ef4444', background: 'rgba(239,68,68,0.1)', border: '1px solid rgba(239,68,68,0.3)', padding: '4px 10px', borderRadius: '6px'}}>⚠ Save failed — try again</span>
              )}
              {promptDirty && (
                <button className="btn-primary" style={{background: 'linear-gradient(135deg, #10b981, #059669)', fontSize: '0.85rem', padding: '6px 14px'}}
                  onClick={handleSaveSystemPrompt} disabled={promptSaving}>
                  {promptSaving ? '⏳ Saving...' : '💾 Save Prompt'}
                </button>
              )}
            </div>
          </div>
          <p style={{color: '#94a3b8', fontSize: '0.85rem', marginBottom: '1rem'}}>This is the product knowledge the AI receives during calls. Edit to customize what the AI knows.</p>

          {systemPromptAuto && !systemPromptCustom && (
            <div style={{marginBottom: '1rem'}}>
              <label style={{display: 'block', marginBottom: '6px', fontWeight: 600, color: '#22d3ee', fontSize: '0.85rem'}}>📄 Auto-Generated from Products</label>
              <div style={{background: 'rgba(0,0,0,0.3)', padding: '12px', borderRadius: '8px',
                border: '1px solid rgba(34, 211, 238, 0.15)', whiteSpace: 'pre-wrap',
                color: '#cbd5e1', fontSize: '0.85rem', lineHeight: 1.6, maxHeight: '200px', overflowY: 'auto'}}>
                {systemPromptAuto}
              </div>
            </div>
          )}

          <div>
            <label style={{display: 'block', marginBottom: '6px', fontWeight: 600, fontSize: '0.85rem'}}>✏️ Custom System Prompt {systemPromptCustom ? '(Active)' : '(Optional Override)'}</label>
            <textarea className="form-input" rows={8}
              placeholder={systemPromptAuto || 'Add product info, scrape a website, then customize the prompt here...'}
              value={systemPromptCustom}
              onChange={e => { setSystemPromptCustom(e.target.value); setPromptDirty(true); }}
              style={{resize: 'vertical', minHeight: '120px', fontSize: '0.85rem', lineHeight: 1.6,
                borderColor: systemPromptCustom.length > PROMPT_HARD_WARN ? '#ef4444'
                  : systemPromptCustom.length > PROMPT_SOFT_WARN ? '#f59e0b' : undefined}} />

            {/* ── Point 1: char / token counter ── */}
            {(() => {
              const chars  = systemPromptCustom.length;
              const tokens = Math.round(chars / 4);
              const pct    = Math.min(chars / PROMPT_HARD_WARN * 100, 100);
              const color  = chars > PROMPT_HARD_WARN ? '#ef4444'
                           : chars > PROMPT_SOFT_WARN ? '#f59e0b'
                           : chars > 4000            ? '#fb923c'
                           : '#22c55e';
              return (
                <div style={{marginTop: '6px'}}>
                  {/* progress bar */}
                  <div style={{height: '3px', background: 'rgba(255,255,255,0.07)', borderRadius: '2px', overflow: 'hidden', marginBottom: '5px'}}>
                    <div style={{height: '100%', width: `${pct}%`, background: color, borderRadius: '2px', transition: 'width 0.15s, background 0.15s'}} />
                  </div>
                  {/* counter row */}
                  <div style={{display: 'flex', justifyContent: 'space-between', alignItems: 'center'}}>
                    <span style={{fontSize: '0.75rem', color}}>
                      {chars.toLocaleString()} chars · ~{tokens.toLocaleString()} tokens
                      {/* ── Point 2: soft-cap warnings ── */}
                      {chars > PROMPT_HARD_WARN && (
                        <span style={{marginLeft: '8px', fontWeight: 600}}>⚠ Exceeds {PROMPT_HARD_WARN.toLocaleString()} char limit — AI may silently truncate context</span>
                      )}
                      {chars > PROMPT_SOFT_WARN && chars <= PROMPT_HARD_WARN && (
                        <span style={{marginLeft: '8px'}}>— approaching limit, consider trimming</span>
                      )}
                      {chars > 4000 && chars <= PROMPT_SOFT_WARN && (
                        <span style={{marginLeft: '8px', color: '#fb923c'}}>— getting long</span>
                      )}
                    </span>
                    <span style={{fontSize: '0.7rem', color: '#475569'}}>{PROMPT_HARD_WARN.toLocaleString()} char max</span>
                  </div>
                </div>
              );
            })()}

            <p style={{color: '#64748b', fontSize: '0.75rem', marginTop: '8px', marginBottom: 0}}>If empty, the auto-generated version from your products is used. If you write a custom prompt, it overrides the auto-generated one.</p>
          </div>
        </div>
      )}
    </div>
  );
}
