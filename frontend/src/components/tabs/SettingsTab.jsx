import React from 'react';
import { formatDate } from '../../utils/dateFormat';
import { useToast } from '../../contexts/ToastContext';
import { INDIAN_VOICES, INDIAN_LANGUAGES } from '../../constants/voices';

const PROMPT_SOFT_WARN = 6000;  // chars — yellow warning zone
const PROMPT_HARD_WARN = 8000;  // chars — red, AI may truncate

export default function SettingsTab({
  handleAddPronunciation, pronFormData, setPronFormData, pronunciations, handleDeletePronunciation,
  pronError, setPronError,
  selectedOrg, orgs, showProductInput, setShowProductInput, newProductName, setNewProductName,
  productError, setProductError,
  handleAddProduct, orgProducts, handleDeleteProduct, handleSaveProduct, scraping, handleScrapeProduct,
  promptDirty, handleSaveSystemPrompt, promptSaving, promptSaveStatus, systemPromptAuto, systemPromptCustom,
  setSystemPromptCustom, setPromptDirty,
  apiFetch, API_URL, orgTimezone,
  activeVoiceProvider, activeVoiceId, activeLanguage,
  handleSaveOrgVoice, voiceSaving
}) {
  const { showToast } = useToast();
  const [localProvider, setLocalProvider] = React.useState(activeVoiceProvider || 'elevenlabs');
  const [localVoiceId, setLocalVoiceId] = React.useState(activeVoiceId || '');
  const [localLanguage, setLocalLanguage] = React.useState(activeLanguage || 'hi');
  const [confirmDeletePronId, setConfirmDeletePronId] = React.useState(null);
  const [confirmDeleteProdId, setConfirmDeleteProdId] = React.useState(null);

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
  const [productPrompts, setProductPrompts] = React.useState({});
  const loadedProductIds = React.useRef(new Set());

  // Load per-product prompts when products change
  React.useEffect(() => {
    if (!orgProducts || orgProducts.length === 0) return;
    orgProducts.forEach(p => {
      if (loadedProductIds.current.has(p.id)) return; // already loaded
      loadedProductIds.current.add(p.id);
      apiFetch(`${API_URL}/products/${p.id}/prompt`)
        .then(res => res.ok ? res.json() : { agent_persona: '', call_flow_instructions: '' })
        .then(data => {
          setProductPrompts(prev => ({
            ...prev,
            [p.id]: {
              ...(prev[p.id] || {}),
              agent_persona: data.agent_persona || '',
              call_flow_instructions: data.call_flow_instructions || '',
              expanded: prev[p.id]?.expanded || false,
              generating: prev[p.id]?.generating || false,
              saving: prev[p.id]?.saving || false
            }
          }));
        })
        .catch(() => {
          setProductPrompts(prev => ({
            ...prev,
            [p.id]: { ...(prev[p.id] || {}), agent_persona: '', call_flow_instructions: '', expanded: false, generating: false, saving: false }
          }));
        });
    });
  }, [orgProducts]);

  const updateProductPrompt = (productId, field, value) => {
    setProductPrompts(prev => ({
      ...prev,
      [productId]: { ...prev[productId], [field]: value }
    }));
  };

  const handleGenerateProductPrompt = async (productId) => {
    updateProductPrompt(productId, 'generating', true);
    try {
      const res = await apiFetch(`${API_URL}/products/${productId}/generate-prompt`, {
        method: 'POST', headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({
          agent_persona: productPrompts[productId]?.agent_persona || '',
          call_flow: productPrompts[productId]?.call_flow_instructions || ''
        })
      });
      const data = await res.json();
      if (data.prompt) {
        setSystemPromptCustom(data.prompt);
        setPromptDirty(true);
        showToast('System prompt generated — review below and click Save.');
      } else {
        showToast(data.message || 'Generation failed', 'error');
      }
    } catch(e) { showToast('Failed to generate', 'error'); }
    updateProductPrompt(productId, 'generating', false);
  };

  const handleGeneratePersona = async (productId) => {
    updateProductPrompt(productId, 'generatingPersona', true);
    try {
      const res = await apiFetch(`${API_URL}/products/${productId}/generate-persona`, {
        method: 'POST', headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({})
      });
      const data = await res.json();
      if (data.status === 'success') {
        updateProductPrompt(productId, 'agent_persona', data.agent_persona);
        updateProductPrompt(productId, 'call_flow_instructions', data.call_flow_instructions);
        // Auto-save immediately after generation
        await apiFetch(`${API_URL}/products/${productId}/prompt`, {
          method: 'PUT', headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({ agent_persona: data.agent_persona, call_flow_instructions: data.call_flow_instructions })
        });
      } else {
        showToast(data.message || 'Generation failed', 'error');
      }
    } catch(e) { showToast('Failed to generate persona', 'error'); }
    updateProductPrompt(productId, 'generatingPersona', false);
  };

  const handleSaveProductPrompt = async (productId) => {
    updateProductPrompt(productId, 'saving', true);
    try {
      const pp = productPrompts[productId];
      const res = await apiFetch(`${API_URL}/products/${productId}/prompt`, {
        method: 'PUT', headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({
          agent_persona: pp?.agent_persona || '',
          call_flow_instructions: pp?.call_flow_instructions || ''
        })
      });
      if (!res.ok) {
        const err = await res.text();
        throw new Error(err || `HTTP ${res.status}`);
      }
      showToast('Persona & call flow saved!');
    } catch(e) { showToast('Failed to save: ' + (e.message || e), 'error'); }
    updateProductPrompt(productId, 'saving', false);
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

      {/* Product Knowledge Section */}
      <div className="wa-header" style={{borderBottom: '1px solid rgba(255,255,255,0.05)', margin: '2.5rem 0 1.5rem'}}>
        <h3><span style={{color: '#22d3ee'}}>🌐 Product</span> Knowledge</h3>
        <p>Manage your organizations and products. The AI learns from this to have informed conversations.</p>
      </div>

      <div className="glass-panel" style={{marginBottom: '2rem', display: 'flex', alignItems: 'center', gap: '12px', padding: '1rem 1.5rem'}}>
        <span style={{fontSize: '1.3rem'}}>🏛️</span>
        <div>
          <div style={{fontSize: '0.75rem', color: '#64748b', fontWeight: 500, textTransform: 'uppercase', letterSpacing: '0.05em'}}>Your Organization</div>
          <div style={{fontSize: '1.15rem', fontWeight: 700, color: '#22d3ee'}}>{selectedOrg ? selectedOrg.name : (orgs.length > 0 ? orgs[0].name : 'No organization linked')}</div>
        </div>
      </div>

      {selectedOrg && (
        <div className="glass-panel" style={{marginBottom: '2rem'}}>
          <div style={{display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1rem'}}>
            <h4 style={{marginTop: 0, marginBottom: 0, fontSize: '1.1rem', fontWeight: 600, color: '#22d3ee'}}>📦 Products in {selectedOrg.name}</h4>
            {!showProductInput ? (
              <button data-testid="add-product-btn" className="btn-primary" style={{background: 'linear-gradient(135deg, #22d3ee, #06b6d4)', fontSize: '0.85rem', padding: '6px 14px'}}
                onClick={() => setShowProductInput(true)}>+ Add Product</button>
            ) : (
              <div style={{display: 'flex', flexDirection: 'column', alignItems: 'flex-end', gap: '4px'}}>
                <div style={{display: 'flex', gap: '8px', alignItems: 'center'}}>
                  <input data-testid="product-name-input" className="form-input" autoFocus placeholder="Product name (e.g. AdsGPT)..."
                    value={newProductName}
                    onChange={e => { setNewProductName(e.target.value); if (productError) setProductError(''); }}
                    onKeyDown={e => e.key === 'Enter' && handleAddProduct()}
                    style={{width: '220px', height: '36px', fontSize: '0.85rem', borderColor: productError ? 'rgba(239,68,68,0.6)' : undefined}} />
                  <button className="btn-primary" style={{background: 'linear-gradient(135deg, #10b981, #059669)', fontSize: '0.85rem', padding: '6px 14px', height: '36px'}}
                    onClick={handleAddProduct}>Add</button>
                  <button style={{background: 'transparent', border: '1px solid rgba(255,255,255,0.1)', color: '#94a3b8', fontSize: '0.85rem', padding: '6px 10px', borderRadius: '6px', cursor: 'pointer', height: '36px'}}
                    onClick={() => { setShowProductInput(false); setNewProductName(''); setProductError(''); }}>✕</button>
                </div>
                {productError && (
                  <p style={{margin: 0, fontSize: '0.78rem', color: '#f87171'}}>⚠ {productError}</p>
                )}
              </div>
            )}
          </div>

          {orgProducts.length === 0 ? (
            <div style={{padding: '1.5rem', textAlign: 'center', color: '#64748b', background: 'rgba(0,0,0,0.2)', borderRadius: '8px'}}>No products yet. Add one to configure AI knowledge.</div>
          ) : (
            <div style={{display: 'flex', flexDirection: 'column', gap: '16px'}}>
              {orgProducts.map(p => {
                const pp = productPrompts[p.id] || { agent_persona: '', call_flow_instructions: '', expanded: false, generating: false, saving: false };
                return (
                <div key={p.id} style={{background: 'rgba(0,0,0,0.2)', borderRadius: '12px', padding: '1.25rem', border: '1px solid rgba(255,255,255,0.05)'}}>
                  <div style={{display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1rem'}}>
                    <div style={{display: 'flex', alignItems: 'center', gap: '8px', flex: 1}}>
                      {pp.editingName ? (
                        <>
                          <input className="form-input" autoFocus defaultValue={p.name}
                            onBlur={e => {
                              if (e.target.value.trim() && e.target.value !== p.name) handleSaveProduct(p.id, { name: e.target.value.trim() });
                              updateProductPrompt(p.id, 'editingName', false);
                            }}
                            onKeyDown={e => { if (e.key === 'Enter') e.target.blur(); }}
                            style={{fontWeight: 700, fontSize: '1.05rem', color: '#e2e8f0', border: '1px solid rgba(34,211,238,0.4)', borderRadius: '6px', padding: '4px 8px', maxWidth: '400px'}} />
                          <button style={{background: 'transparent', border: 'none', color: '#94a3b8', cursor: 'pointer', fontSize: '0.8rem'}}
                            onClick={() => updateProductPrompt(p.id, 'editingName', false)}>Cancel</button>
                        </>
                      ) : (
                        <>
                          <span style={{fontWeight: 700, fontSize: '1.05rem', color: '#e2e8f0'}}>{p.name}</span>
                          <button style={{background: 'rgba(34,211,238,0.1)', border: '1px solid rgba(34,211,238,0.25)', color: '#22d3ee', padding: '3px 10px', borderRadius: '6px', cursor: 'pointer', fontSize: '0.75rem', fontWeight: 600}}
                            onClick={() => updateProductPrompt(p.id, 'editingName', true)}>✏️ Edit</button>
                        </>
                      )}
                    </div>
                    {confirmDeleteProdId === p.id ? (
                      <span style={{display: 'flex', gap: '6px', alignItems: 'center'}}>
                        <span style={{fontSize: '0.8rem', color: '#fca5a5'}}>Remove?</span>
                        <button onClick={() => { handleDeleteProduct(p.id); setConfirmDeleteProdId(null); }}
                          style={{background: '#ef4444', border: 'none', color: '#fff', borderRadius: '5px', padding: '3px 10px', cursor: 'pointer', fontSize: '0.75rem', fontWeight: 600}}>Yes</button>
                        <button onClick={() => setConfirmDeleteProdId(null)}
                          style={{background: 'transparent', border: '1px solid rgba(255,255,255,0.15)', color: '#94a3b8', borderRadius: '5px', padding: '3px 10px', cursor: 'pointer', fontSize: '0.75rem'}}>No</button>
                      </span>
                    ) : (
                      <button style={{background: 'transparent', border: 'none', color: '#ef4444', cursor: 'pointer', fontSize: '0.85rem'}}
                        onClick={() => setConfirmDeleteProdId(p.id)}>🗑️ Remove</button>
                    )}
                  </div>

                  <div style={{display: 'flex', gap: '10px', marginBottom: '1rem', alignItems: 'flex-end'}}>
                    <div className="form-group" style={{marginBottom: 0, flex: 1}}>
                      <label>Website URL</label>
                      <input className="form-input" placeholder="https://..." defaultValue={p.website_url}
                        onBlur={e => handleSaveProduct(p.id, { website_url: e.target.value })} />
                    </div>
                    <button className="btn-primary" style={{height: '42px', padding: '0 16px', whiteSpace: 'nowrap',
                      background: scraping === p.id ? '#475569' : 'linear-gradient(135deg, #06b6d4, #0891b2)', fontSize: '0.85rem'}}
                      onClick={() => handleScrapeProduct(p.id)} disabled={scraping === p.id}>
                      {scraping === p.id ? '⏳ Analyzing...' : (p.website_url ? '🔍 Scrape Website' : '🧠 AI Research')}
                    </button>
                  </div>

                  {/* Collapsible: All product details */}
                  <div style={{marginTop: '0.5rem'}}>
                    <button
                      onClick={() => updateProductPrompt(p.id, 'expanded', !pp.expanded)}
                      style={{
                        background: 'rgba(34,211,238,0.08)', border: '1px solid rgba(34,211,238,0.2)',
                        color: '#22d3ee', padding: '8px 14px', borderRadius: '8px', cursor: 'pointer',
                        fontSize: '0.85rem', fontWeight: 600, width: '100%', textAlign: 'left'
                      }}>
                      {pp.expanded ? '▾' : '▸'} Product Details, Persona & Call Flow
                      {p.scraped_info ? ' ✅' : ''}
                      {pp.agent_persona ? ' 🎭' : ''}
                    </button>

                    {pp.expanded && (
                      <div style={{marginTop: '12px', padding: '14px', background: 'rgba(0,0,0,0.15)', borderRadius: '8px', border: '1px solid rgba(34,211,238,0.1)'}}>

                        {/* AI-Extracted Info */}
                        {p.scraped_info && (
                          <div style={{marginBottom: '1.25rem'}}>
                            <label style={{display: 'block', marginBottom: '6px', fontWeight: 600, color: '#22d3ee', fontSize: '0.85rem'}}>📄 AI-Extracted Info</label>
                            <div style={{background: 'rgba(0,0,0,0.3)', padding: '12px', borderRadius: '8px',
                              border: '1px solid rgba(34, 211, 238, 0.15)', whiteSpace: 'pre-wrap',
                              color: '#cbd5e1', fontSize: '0.85rem', lineHeight: 1.5, maxHeight: '200px', overflowY: 'auto'}}>
                              {p.scraped_info}
                            </div>
                          </div>
                        )}

                        {/* Manual Notes */}
                        <div style={{marginBottom: '1.25rem'}}>
                          <label style={{display: 'block', marginBottom: '6px', fontWeight: 600, fontSize: '0.85rem'}}>📝 Manual Notes</label>
                          <textarea className="form-input" rows={3} placeholder="Pricing, USPs, objection handling..."
                            defaultValue={p.manual_notes}
                            onBlur={e => handleSaveProduct(p.id, { manual_notes: e.target.value })}
                            style={{resize: 'vertical', minHeight: '70px', fontSize: '0.85rem'}} />
                        </div>

                        <div style={{borderTop: '1px solid rgba(255,255,255,0.06)', margin: '1rem 0', paddingTop: '1rem'}}>
                          {(p.scraped_info || p.manual_notes) && (
                            <div style={{marginBottom: '1rem'}}>
                              <button className="btn-primary"
                                style={{background: 'linear-gradient(135deg, #818cf8, #6366f1)', fontSize: '0.85rem', padding: '8px 16px', width: '100%'}}
                                disabled={pp.generatingPersona}
                                onClick={() => handleGeneratePersona(p.id)}>
                                {pp.generatingPersona ? '⏳ Generating from website info...' : '✨ Auto-Generate Persona & Call Flow from Website'}
                              </button>
                            </div>
                          )}

                          {/* Agent Persona */}
                          <div style={{marginBottom: '1rem'}}>
                            <label style={{display: 'block', marginBottom: '6px', fontWeight: 600, fontSize: '0.85rem', color: '#a78bfa'}}>🎭 Agent Persona</label>
                            <textarea className="form-input" rows={4}
                              value={pp.agent_persona}
                              onChange={e => updateProductPrompt(p.id, 'agent_persona', e.target.value)}
                              placeholder="e.g. You are Meera, a professional sales agent..."
                              style={{resize: 'vertical', minHeight: '80px', fontSize: '0.85rem', lineHeight: 1.6}} />
                          </div>

                          {/* Call Flow */}
                          <div style={{marginBottom: '1rem'}}>
                            <label style={{display: 'block', marginBottom: '6px', fontWeight: 600, fontSize: '0.85rem', color: '#22d3ee'}}>📋 Call Flow Instructions</label>
                            <textarea className="form-input" rows={5}
                              value={pp.call_flow_instructions}
                              onChange={e => updateProductPrompt(p.id, 'call_flow_instructions', e.target.value)}
                              placeholder="e.g. Step 1: Greet. Step 2: Qualify..."
                              style={{resize: 'vertical', minHeight: '100px', fontSize: '0.85rem', lineHeight: 1.6}} />
                          </div>

                          <div style={{display: 'flex', gap: '10px'}}>
                            <button className="btn-primary"
                              style={{background: 'linear-gradient(135deg, #f59e0b, #d97706)', fontSize: '0.85rem', padding: '8px 16px'}}
                              disabled={pp.generating}
                              onClick={() => handleGenerateProductPrompt(p.id)}>
                              {pp.generating ? '⏳ Generating...' : '🤖 Generate Prompt'}
                            </button>
                            <button className="btn-primary"
                              style={{background: 'linear-gradient(135deg, #10b981, #059669)', fontSize: '0.85rem', padding: '8px 16px'}}
                              disabled={pp.saving}
                              onClick={() => handleSaveProductPrompt(p.id)}>
                              {pp.saving ? '⏳ Saving...' : '💾 Save Persona & Flow'}
                            </button>
                          </div>
                        </div>
                      </div>
                    )}
                  </div>
                </div>
                );
              })}
            </div>
          )}
        </div>
      )}

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
