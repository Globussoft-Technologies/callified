import React, { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { formatDate } from '../../utils/dateFormat';
import { useHideAiFeatures } from '../../hooks/useHideAiFeatures';
import AppIcon from '../common/AppIcon';
import AppSelect from '../common/AppSelect';

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
  setSystemPromptCustom, systemPromptMode, setSystemPromptMode, setPromptDirty,
  orgTimezone, timezone, setTimezone, timezoneSaving, handleSaveTimezone
}) {
  const hideAiFeatures = useHideAiFeatures();
  const navigate = useNavigate();
  const [callActions, setCallActions] = useState(() => {
    try {
      const saved = JSON.parse(localStorage.getItem('callified_call_actions') || '{}');
      return {
        dial: saved.dial !== false,
        browserCall: saved.browserCall !== false,
        simWebCall: saved.simWebCall !== false,
      };
    } catch {
      return { dial: true, browserCall: true, simWebCall: true };
    }
  });
  const [callActionsSaved, setCallActionsSaved] = useState(false);

  const handleCallActionChange = (key) => {
    setCallActions(prev => ({ ...prev, [key]: !prev[key] }));
    setCallActionsSaved(false);
  };

  const saveCallActions = () => {
    const toSave = hideAiFeatures
      ? { dial: false, browserCall: true, simWebCall: false }
      : callActions;
    localStorage.setItem('callified_call_actions', JSON.stringify(toSave));
    setCallActions(toSave);
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
          Settings
        </h2>
        <p style={{ margin: '4px 0 0', fontSize: 13, color: T.muted }}>
          Manage organization defaults, AI behavior, and your personal workspace preferences.
        </p>
      </div>

      <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>

        <div style={card}>
          <h3 style={{ margin: '0 0 18px', fontSize: 16, fontWeight: 700, color: T.text }}><AppIcon name="settings" /> Organization</h3>
          <div style={{ display: 'grid', gridTemplateColumns: 'minmax(220px, 1fr) minmax(240px, 1fr) auto', gap: 14, alignItems: 'end' }}>
            <div>
              <label style={labelStyle}>Organization</label>
              <input value={selectedOrg?.name || ''} readOnly style={{ ...inputStyle, cursor: 'default' }} />
            </div>
            <div>
              <label style={labelStyle}>Timezone</label>
              <AppSelect
                value={timezone}
                onChange={setTimezone}
                searchable
                options={[
                  'Asia/Kolkata', 'Asia/Dubai', 'Asia/Singapore', 'Europe/London',
                  'America/New_York', 'America/Chicago', 'America/Denver', 'America/Los_Angeles',
                ].map(value => ({ value, label: value }))}
                width="100%"
              />
            </div>
            <button onClick={handleSaveTimezone} disabled={timezoneSaving} style={{
              height: 42, padding: '0 18px', border: 'none', borderRadius: 8,
              background: T.accent, color: '#fff', fontWeight: 700, cursor: timezoneSaving ? 'wait' : 'pointer',
            }}>{timezoneSaving ? 'Saving...' : 'Save'}</button>
          </div>
          <button onClick={() => navigate('/exotel-accounts')} style={{
            marginTop: 16, padding: 0, border: 0, background: 'transparent', color: T.accent,
            fontSize: 13, fontWeight: 700, cursor: 'pointer',
          }}>Manage provider accounts →</button>
        </div>

        {!hideAiFeatures && <h3 style={{ margin: '8px 0 -6px', fontSize: 14, color: T.muted, textTransform: 'uppercase' }}>Voice &amp; AI</h3>}

        {/* Pronunciation Guide */}
        {!hideAiFeatures && (<div style={card}>
          <h3 style={{ margin: '0 0 6px', fontSize: 16, fontWeight: 700, color: T.text }}><AppIcon name="sound" /> Pronunciation Guide</h3>
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
              <AppIcon name="plus" /> Add Rule
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
                      <td style={{ ...rowTd, color: T.green, fontStyle: 'italic' }}><AppIcon name="sound" /> "{p.phonetic}"</td>
                      <td style={{ ...rowTd, color: T.muted }}>{formatDate(p.created_at, orgTimezone)}</td>
                      <td style={rowTd}>
                        {p.inherited ? (
                          <span title="Inherited legacy rule; add the same word above to override it for this organization"
                            style={{ color: T.muted, fontSize: 12, fontWeight: 600 }}>Inherited</span>
                        ) : (
                          <button
                            onClick={() => handleDeletePronunciation(p.id)}
                            style={{
                              background: 'rgba(239,68,68,0.06)', border: '1px solid rgba(239,68,68,0.25)',
                              color: T.red, borderRadius: 6, padding: '4px 12px',
                              cursor: 'pointer', fontSize: 12, fontWeight: 600, fontFamily: T.font,
                            }}>
                            <AppIcon name="delete" /> Remove
                          </button>
                        )}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          )}
        </div>)}

        {/* System Prompt */}
        {!hideAiFeatures && selectedOrg && (
          <div style={card}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
              <h3 style={{ margin: 0, fontSize: 16, fontWeight: 700, color: T.text }}><AppIcon name="robot" /> Additional AI Instructions</h3>
              <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
                {!promptDirty && promptSaved && (
                  <span style={{ color: '#10b981', fontSize: 13, fontWeight: 600 }}><AppIcon name="check" /> Saved</span>
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
                    {promptSaving ? <><AppIcon name="loading" spin /> Saving...</> : <><AppIcon name="save" /> Save Prompt</>}
                  </button>
                )}
              </div>
            </div>
            <p style={{ color: T.muted, fontSize: 13, marginBottom: 16, marginTop: 0 }}>
              Choose whether organization instructions replace the standard prompt or extend Callified's protected prompt.
            </p>

            <div style={{ marginBottom: 16, maxWidth: 420 }}>
              <label style={labelStyle}>Prompt behavior</label>
              <AppSelect
                value={systemPromptMode}
                onChange={value => { setSystemPromptMode(value); setPromptDirty(true); }}
                options={[
                  { value: 'replace', label: 'Replace default prompt' },
                  { value: 'extend', label: 'Extend default prompt' },
                ]}
                width="100%"
              />
              <p style={{ color: T.muted, fontSize: 12, margin: '6px 0 0' }}>
                {systemPromptMode === 'replace'
                  ? 'Preserves the established custom agent behavior for this organization.'
                  : 'Keeps Callified safety, language, product, and call-flow rules, then adds these instructions.'}
              </p>
            </div>

            {systemPromptAuto && !systemPromptCustom && (
              <div style={{ marginBottom: 16 }}>
                <label style={{ ...labelStyle, color: T.accent }}><AppIcon name="file" /> Auto-Generated from Products</label>
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
                <AppIcon name="edit" /> Organization Instructions {systemPromptCustom ? '(Active)' : '(Optional)'}
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
                Product knowledge remains managed in Products. These instructions apply to every AI call for this organization.
              </p>
            </div>
          </div>
        )}

        {/* Call Action Visibility */}
        <h3 style={{ margin: '8px 0 -6px', fontSize: 14, color: T.muted, textTransform: 'uppercase' }}>Personal Preferences</h3>
        <div style={card}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
            <h3 style={{ margin: 0, fontSize: 16, fontWeight: 700, color: T.text }}><AppIcon name="phone" /> Call Action Visibility</h3>
            <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
              {callActionsSaved && (
                <span style={{ color: '#10b981', fontSize: 13, fontWeight: 600 }}><AppIcon name="check" /> Saved</span>
              )}
              <button
                onClick={saveCallActions}
                style={{
                  background: 'linear-gradient(135deg, #6366f1, #8b5cf6)', border: 'none',
                  borderRadius: 8, color: '#fff', padding: '8px 16px',
                  cursor: 'pointer', fontWeight: 700, fontSize: 13, fontFamily: T.font,
                }}>
                <AppIcon name="save" /> Save
              </button>
            </div>
          </div>
          <p style={{ color: T.muted, fontSize: 13, marginBottom: 16, marginTop: 0 }}>
            Choose which call buttons appear in this browser. These preferences do not affect other team members or devices.
          </p>

          {hideAiFeatures ? (
            <label style={{
              display: 'flex', alignItems: 'center', gap: 10,
              padding: '10px 12px', borderRadius: 8,
              border: `1px solid ${T.border}`, marginBottom: 10,
              background: '#f9fafb',
            }}>
              <input
                type="checkbox"
                checked={callActions.browserCall}
                readOnly
                style={{ width: 18, height: 18, cursor: 'default' }}
              />
              <span style={{ fontSize: 14, color: T.text, fontWeight: 600 }}><AppIcon name="audio" /> Browser Call</span>
            </label>
          ) : ([
            { key: 'dial', label: 'Dial', icon: 'phone' },
            { key: 'browserCall', label: 'Browser Call', icon: 'audio' },
            { key: 'simWebCall', label: 'Sim Web Call', icon: 'global' },
          ].map(({ key, label, icon }) => (
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
              <span style={{ fontSize: 14, color: T.text, fontWeight: 600 }}><AppIcon name={icon} /> {label}</span>
            </label>
          )))}
        </div>

      </div>
    </div>
  );
}
