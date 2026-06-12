import React, { useState, useEffect } from 'react';
import { formatDate } from '../../utils/dateFormat';

const T = {
  bg: '#f4f5f9', card: '#ffffff', border: '#e5e7eb',
  accent: '#6366f1', green: '#10b981', amber: '#f59e0b',
  red: '#ef4444', text: '#111827', sub: '#374151', muted: '#9ca3af',
  font: "'DM Sans', sans-serif", mono: "'DM Mono', monospace",
};

const card = {
  background: T.card, border: `1px solid ${T.border}`,
  borderRadius: 12, boxShadow: '0 1px 3px rgba(0,0,0,0.06), 0 4px 12px rgba(0,0,0,0.04)',
  padding: '24px 28px',
};

export default function SettingsTab({
  handleAddPronunciation, pronFormData, setPronFormData, pronError, setPronError, pronunciations, handleDeletePronunciation,
  selectedOrg,
  promptDirty, handleSaveSystemPrompt, promptSaving, promptSaved, systemPromptAuto, systemPromptCustom,
  setSystemPromptCustom, setPromptDirty,
  orgTimezone
}) {
  const [callActions, setCallActions] = useState({
    dial: true,
    browserCall: true,
    simWebCall: true,
  });
  const [callActionsSaved, setCallActionsSaved] = useState(false);

  useEffect(() => {
    try {
      const saved = JSON.parse(localStorage.getItem('callified_call_actions') || '{}');
      setCallActions({
        dial: saved.dial !== false,
        browserCall: saved.browserCall !== false,
        simWebCall: saved.simWebCall !== false,
      });
    } catch { /* ignore */ }
  }, []);

  const handleCallActionChange = (key) => {
    setCallActions(prev => ({ ...prev, [key]: !prev[key] }));
    setCallActionsSaved(false);
  };

  const saveCallActions = () => {
    localStorage.setItem('callified_call_actions', JSON.stringify(callActions));
    setCallActionsSaved(true);
    setTimeout(() => setCallActionsSaved(false), 3000);
  };

  const labelStyle = { fontSize: 13, fontWeight: 600, color: T.sub, marginBottom: 6, display: 'block', fontFamily: T.font };
  const inputStyle = {
    width: '100%', padding: '10px 14px', borderRadius: 8, fontSize: 13,
    border: `1px solid ${T.border}`, background: '#f9fafb', color: T.text,
    fontFamily: T.font, outline: 'none', boxSizing: 'border-box',
  };
  const thStyle = {
    fontSize: 10, fontWeight: 700, color: T.muted, textTransform: 'uppercase',
    letterSpacing: '0.07em', padding: '0 0 10px', textAlign: 'left',
    borderBottom: `1px solid ${T.border}`,
  };
  const tdStyle = {
    fontSize: 13, color: T.sub, padding: '11px 0',
    borderBottom: `1px solid ${T.border}`, verticalAlign: 'middle',
  };

  return (
    <div style={{ padding: '28px 32px', background: T.bg, minHeight: '100%', fontFamily: T.font }}>

      {/* Page title */}
      <div style={{ marginBottom: 24 }}>
        <h2 style={{ margin: 0, fontSize: 22, fontWeight: 700, color: T.text }}>
          <span style={{ color: T.amber }}>AI Voice</span> Settings
        </h2>
        <p style={{ margin: '4px 0 0', fontSize: 13, color: T.muted }}>
          Configure how the AI pronounces product names, brand names, and technical terms during calls.
        </p>
      </div>

      <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>

        {/* Pronunciation Guide */}
        <div style={card}>
          <h3 style={{ margin: '0 0 6px', fontSize: 16, fontWeight: 700, color: T.text }}>🗣️ Pronunciation Guide</h3>
          <p style={{ margin: '0 0 20px', fontSize: 13, color: T.muted }}>
            Teach the AI how to speak your product names correctly. The AI will use the phonetic version in conversations.
          </p>

          <form onSubmit={handleAddPronunciation} style={{ display: 'flex', gap: 12, marginBottom: 20, alignItems: 'flex-end' }}>
            <div style={{ flex: 1 }}>
              <label style={labelStyle}>Written Word</label>
              <input
                required value={pronFormData.word}
                onChange={e => { setPronFormData({ ...pronFormData, word: e.target.value }); if (pronError) setPronError(''); }}
                placeholder="e.g. Adsgpt"
                data-testid="pron-word"
                style={inputStyle}
              />
            </div>
            <div style={{ fontSize: 20, color: T.muted, paddingBottom: 10 }}>→</div>
            <div style={{ flex: 1 }}>
              <label style={labelStyle}>How to Pronounce</label>
              <input
                required value={pronFormData.phonetic}
                onChange={e => { setPronFormData({ ...pronFormData, phonetic: e.target.value }); if (pronError) setPronError(''); }}
                placeholder="e.g. Ads G P T"
                data-testid="pron-phonetic"
                style={inputStyle}
              />
            </div>
            <button data-testid="add-rule-btn" type="submit"
              style={{
                height: 42, padding: '0 20px', borderRadius: 8, border: 'none',
                background: 'linear-gradient(135deg, #6366f1, #8b5cf6)',
                color: '#fff', fontWeight: 700, fontSize: 13, fontFamily: T.font,
                cursor: 'pointer', whiteSpace: 'nowrap',
              }}>
              + Add Rule
            </button>
          </form>
          {pronError && (
            <p style={{ margin: '-12px 0 16px', fontSize: 12, fontWeight: 600, color: '#ef4444' }}>
              {pronError}
            </p>
          )}

          {pronunciations.length === 0 ? (
            <div style={{
              padding: '2rem', textAlign: 'center', color: T.muted,
              background: T.bg, borderRadius: 8, border: `1px solid ${T.border}`,
            }}>
              No pronunciation rules added yet. Add one above to get started!
            </div>
          ) : (
            <table style={{ width: '100%', borderCollapse: 'collapse' }}>
              <thead>
                <tr>
                  <th style={thStyle}>Written Word</th>
                  <th style={thStyle}>AI Says</th>
                  <th style={thStyle}>Added</th>
                  <th style={thStyle}>Action</th>
                </tr>
              </thead>
              <tbody>
                {pronunciations.map((p, i) => {
                  const isLast = i === pronunciations.length - 1;
                  const rowTd = { ...tdStyle, borderBottom: isLast ? 'none' : `1px solid ${T.border}` };
                  return (
                    <tr key={p.id}>
                      <td style={{ ...rowTd, fontWeight: 600, color: T.text, fontFamily: T.mono }}>{p.word}</td>
                      <td style={{ ...rowTd, color: T.green, fontStyle: 'italic' }}>🔊 "{p.phonetic}"</td>
                      <td style={{ ...rowTd, color: T.muted }}>{formatDate(p.created_at, orgTimezone)}</td>
                      <td style={rowTd}>
                        <button
                          onClick={() => handleDeletePronunciation(p.id)}
                          style={{
                            background: 'rgba(239,68,68,0.06)', border: '1px solid rgba(239,68,68,0.25)',
                            color: T.red, borderRadius: 6, padding: '4px 12px',
                            cursor: 'pointer', fontSize: 12, fontWeight: 600, fontFamily: T.font,
                          }}>
                          🗑️ Remove
                        </button>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          )}
        </div>

        {/* How it works */}
        <div style={{
          ...card,
          background: 'rgba(245,158,11,0.04)', border: '1px solid rgba(245,158,11,0.2)',
          boxShadow: 'none',
        }}>
          <h4 style={{ margin: '0 0 10px', fontSize: 14, fontWeight: 700, color: T.amber }}>💡 How it works</h4>
          <p style={{ color: T.sub, fontSize: 13, margin: 0, lineHeight: 1.7 }}>
            The pronunciation guide is injected into the AI's prompt at the start of every call.
            When the AI generates a response containing a mapped word, it will use the phonetic version instead.
            The TTS engine then speaks the phonetic text, resulting in correct pronunciation.
            <br /><br />
            <strong style={{ color: T.text }}>Example:</strong> If you add "Adsgpt" → "Ads G P T", the AI will say "Ads G P T" instead of trying to sound out "Adsgpt".
          </p>
        </div>

        {/* System Prompt */}
        {selectedOrg && (
          <div style={card}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
              <h3 style={{ margin: 0, fontSize: 16, fontWeight: 700, color: T.text }}>🤖 AI System Prompt</h3>
              <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
                {!promptDirty && promptSaved && (
                  <span style={{ color: '#10b981', fontSize: 13, fontWeight: 600 }}>✓ Saved</span>
                )}
                {promptDirty && (
                  <button
                    onClick={handleSaveSystemPrompt} disabled={promptSaving}
                    style={{
                      background: 'linear-gradient(135deg, #10b981, #059669)', border: 'none',
                      borderRadius: 8, color: '#fff', padding: '8px 16px',
                      cursor: promptSaving ? 'not-allowed' : 'pointer',
                      fontWeight: 700, fontSize: 13, fontFamily: T.font,
                      opacity: promptSaving ? 0.7 : 1,
                    }}>
                    {promptSaving ? '⏳ Saving...' : '💾 Save Prompt'}
                  </button>
                )}
              </div>
            </div>
            <p style={{ color: T.muted, fontSize: 13, marginBottom: 16, marginTop: 0 }}>
              This is the product knowledge the AI receives during calls. Edit to customize what the AI knows.
            </p>

            {systemPromptAuto && !systemPromptCustom && (
              <div style={{ marginBottom: 16 }}>
                <label style={{ ...labelStyle, color: T.accent }}>📄 Auto-Generated from Products</label>
                <div style={{
                  background: T.bg, padding: 12, borderRadius: 8,
                  border: `1px solid ${T.border}`, whiteSpace: 'pre-wrap',
                  color: T.sub, fontSize: 13, lineHeight: 1.6, maxHeight: 200, overflowY: 'auto',
                  fontFamily: T.mono,
                }}>
                  {systemPromptAuto}
                </div>
              </div>
            )}

            <div>
              <label style={labelStyle}>
                ✏️ Custom System Prompt {systemPromptCustom ? '(Active)' : '(Optional Override)'}
              </label>
              <textarea
                rows={8}
                placeholder={systemPromptAuto || 'Add product info, scrape a website, then customize the prompt here...'}
                value={systemPromptCustom}
                onChange={e => { setSystemPromptCustom(e.target.value); setPromptDirty(true); }}
                style={{
                  ...inputStyle, resize: 'vertical', minHeight: 120, lineHeight: 1.6,
                  fontFamily: T.mono,
                }}
              />
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginTop: 4 }}>
                <span style={{ fontSize: 11, color: (systemPromptCustom || '').length > 8000 ? '#ef4444' : '#9ca3af' }}>
                  {(systemPromptCustom || '').length.toLocaleString()} chars
                  {(systemPromptCustom || '').length > 8000 && ' — approaching token limit'}
                </span>
              </div>
              <p style={{ color: T.muted, fontSize: 12, marginTop: 6 }}>
                If empty, the auto-generated version from your products is used. If you write a custom prompt, it overrides the auto-generated one.
              </p>
            </div>
          </div>
        )}

        {/* Call Action Visibility */}
        <div style={card}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
            <h3 style={{ margin: 0, fontSize: 16, fontWeight: 700, color: T.text }}>☎️ Call Action Visibility</h3>
            <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
              {callActionsSaved && (
                <span style={{ color: '#10b981', fontSize: 13, fontWeight: 600 }}>✓ Saved</span>
              )}
              <button
                onClick={saveCallActions}
                style={{
                  background: 'linear-gradient(135deg, #6366f1, #8b5cf6)', border: 'none',
                  borderRadius: 8, color: '#fff', padding: '8px 16px',
                  cursor: 'pointer', fontWeight: 700, fontSize: 13, fontFamily: T.font,
                }}>
                💾 Save
              </button>
            </div>
          </div>
          <p style={{ color: T.muted, fontSize: 13, marginBottom: 16, marginTop: 0 }}>
            Choose which call buttons appear in the lead action row on the campaign page.
          </p>

          {[
            { key: 'dial', label: '📞 Dial' },
            { key: 'browserCall', label: '🎙 Browser Call' },
            { key: 'simWebCall', label: '🌐 Sim Web Call' },
          ].map(({ key, label }) => (
            <label key={key} style={{
              display: 'flex', alignItems: 'center', gap: 10,
              padding: '10px 12px', borderRadius: 8, cursor: 'pointer',
              border: `1px solid ${T.border}`, marginBottom: 10,
              background: '#f9fafb',
            }}>
              <input
                type="checkbox"
                checked={callActions[key]}
                onChange={() => handleCallActionChange(key)}
                style={{ width: 18, height: 18, cursor: 'pointer' }}
              />
              <span style={{ fontSize: 14, color: T.text, fontWeight: 600 }}>{label}</span>
            </label>
          ))}
        </div>

      </div>
    </div>
  );
}
