import React, { useState } from 'react';
import { useToast, useConfirm } from '../../contexts/UIContext';
import { useHideAiFeatures } from '../../hooks/useHideAiFeatures';

const T = {
  bg: '#f4f5f9', card: '#ffffff', border: '#e5e7eb',
  accent: '#6366f1', cyan: '#0891b2', green: '#10b981', amber: '#f59e0b',
  red: '#ef4444', text: '#111827', sub: '#374151', muted: '#9ca3af',
  font: "'DM Sans', sans-serif", mono: "'DM Mono', monospace",
};

const card = {
  background: T.card, border: `1px solid ${T.border}`,
  borderRadius: 12, boxShadow: '0 1px 3px rgba(0,0,0,0.06), 0 4px 12px rgba(0,0,0,0.04)',
};

const inputStyle = {
  width: '100%', padding: '9px 13px', borderRadius: 8, fontSize: 13,
  border: `1px solid ${T.border}`, background: T.card,
  color: T.text, fontFamily: T.font, outline: 'none', boxSizing: 'border-box',
};

const labelStyle = {
  display: 'block', marginBottom: 6, fontSize: 12, fontWeight: 600,
  color: T.muted, textTransform: 'uppercase', letterSpacing: '0.06em', fontFamily: T.font,
};

const T = {
  bg: '#f4f5f9', card: '#ffffff', border: '#e5e7eb',
  accent: '#6366f1', cyan: '#0891b2', green: '#10b981', amber: '#f59e0b',
  red: '#ef4444', text: '#111827', sub: '#374151', muted: '#9ca3af',
  font: "'DM Sans', sans-serif", mono: "'DM Mono', monospace",
};

const card = {
  background: T.card, border: `1px solid ${T.border}`,
  borderRadius: 12, boxShadow: '0 1px 3px rgba(0,0,0,0.06), 0 4px 12px rgba(0,0,0,0.04)',
};

const inputStyle = {
  width: '100%', padding: '9px 13px', borderRadius: 8, fontSize: 13,
  border: `1px solid ${T.border}`, background: T.card,
  color: T.text, fontFamily: T.font, outline: 'none', boxSizing: 'border-box',
};

const labelStyle = {
  display: 'block', marginBottom: 6, fontSize: 12, fontWeight: 600,
  color: T.muted, textTransform: 'uppercase', letterSpacing: '0.06em', fontFamily: T.font,
};

export default function ProductsTab({
  orgProducts, selectedOrg, orgs,
  newProductName, setNewProductName, showProductInput, setShowProductInput,
  handleAddProduct, handleDeleteProduct, handleSaveProduct, handleScrapeProduct, scraping, scrapeError,
  apiFetch, API_URL,
  onProductsRefresh,
}) {
  const toast = useToast();
  const confirm = useConfirm();
  const [productPrompts, setProductPrompts] = React.useState({});
  const [confirmDeleteId, setConfirmDeleteId] = React.useState(null);
  const loadedProductIds = React.useRef(new Set());
  const [nameError, setNameError] = useState('');
  const fileInputRefs = React.useRef({});
  const hideAiFeatures = useHideAiFeatures();

  const getWebsiteUrl = (productId) => productPrompts[productId]?.websiteUrl;
  const setWebsiteUrl = (productId, url) =>
    setProductPrompts(prev => ({ ...prev, [productId]: { ...(prev[productId] || {}), websiteUrl: url } }));

  const handleScrapeWithSave = async (productId) => {
    const currentUrl = getWebsiteUrl(productId);
    const product = orgProducts.find(p => p.id === productId);
    if (currentUrl !== undefined && currentUrl !== (product?.website_url || '')) {
      await handleSaveProduct(productId, { website_url: currentUrl });
    }
    const urlToScrape = currentUrl !== undefined ? currentUrl : product?.website_url;
    if (!urlToScrape) { toast('Please enter a website URL first.'); return; }
    await handleScrapeProduct(productId);
  };

  const handleUploadImage = async (productId) => {
    const pp = productPrompts[productId] || {};
    const file = pp.pendingFile;
    if (!file) { toast('Please choose an image file first.'); return; }
    updateProductPrompt(productId, 'uploading', true);
    try {
      const formData = new FormData();
      formData.append('file', file);
      if (pp.uploadLabel?.trim()) formData.append('label', pp.uploadLabel.trim());
      const res = await apiFetch(`${API_URL}/products/${productId}/images`, { method: 'POST', body: formData });
      if (!res.ok) {
        const txt = await res.text();
        toast('Upload failed: ' + txt);
      } else {
        updateProductPrompt(productId, 'pendingFile', null);
        updateProductPrompt(productId, 'uploadLabel', '');
        if (fileInputRefs.current[productId]) fileInputRefs.current[productId].value = '';
        if (onProductsRefresh) onProductsRefresh();
        else if (selectedOrg) handleSaveProduct && window.location.reload();
      }
    } catch(e) {
      toast('Upload error: ' + e.message);
    }
    updateProductPrompt(productId, 'uploading', false);
  };

  const handleDeleteManualImage = async (productId, idx) => {
    if (!await confirm({ message: 'Remove this image?' })) return;
    try {
      await apiFetch(`${API_URL}/products/${productId}/images/${idx}`, { method: 'DELETE' });
      if (onProductsRefresh) onProductsRefresh();
    } catch(e) {
      toast('Delete error: ' + e.message);
    }
  };

  const handleUpdateImageLabel = async (productId, idx, newLabel) => {
    const product = orgProducts.find(p => p.id === productId);
    if (!product) return;
    const updated = product.manual_images.map((img, i) =>
      i === idx ? { ...img, label: newLabel } : img
    );
    try {
      await apiFetch(`${API_URL}/products/${productId}/images`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(updated),
      });
      if (onProductsRefresh) onProductsRefresh();
    } catch(e) {
      toast('Save error: ' + e.message);
    }
  };

  React.useEffect(() => {
    if (!orgProducts || orgProducts.length === 0) return;
    orgProducts.forEach(p => {
      if (loadedProductIds.current.has(p.id)) return;
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
              saving: prev[p.id]?.saving || false,
              websiteUrl: prev[p.id]?.websiteUrl !== undefined ? prev[p.id].websiteUrl : (p.website_url || ''),
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
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [orgProducts]);

  const updateProductPrompt = (productId, field, value) => {
    setProductPrompts(prev => ({ ...prev, [productId]: { ...prev[productId], [field]: value } }));
  };

  const handleGenerateProductPrompt = async (productId) => {
    updateProductPrompt(productId, 'generating', true);
    try {
      const res = await apiFetch(`${API_URL}/products/${productId}/generate-prompt`, {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ agent_persona: productPrompts[productId]?.agent_persona || '', call_flow: productPrompts[productId]?.call_flow_instructions || '' })
      });
      const data = await res.json();
      if (data.prompt) {
        await apiFetch(`${API_URL}/organizations/${selectedOrg.id}/system-prompt`, {
          method: 'PUT', headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ custom_prompt: data.prompt })
        });
        toast('System prompt generated and saved! Review it in Settings → AI System Prompt.');
      } else { toast(data.message || 'Generation failed'); }
    } catch { toast('Failed to generate');  }
    updateProductPrompt(productId, 'generating', false);
  };

  const handleGeneratePersona = async (productId) => {
    updateProductPrompt(productId, 'generatingPersona', true);
    try {
      const res = await apiFetch(`${API_URL}/products/${productId}/generate-persona`, {
        method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({})
      });
      const data = await res.json();
      if (data.status === 'success') {
        updateProductPrompt(productId, 'agent_persona', data.agent_persona);
        updateProductPrompt(productId, 'call_flow_instructions', data.call_flow_instructions);
        await apiFetch(`${API_URL}/products/${productId}/prompt`, {
          method: 'PUT', headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ agent_persona: data.agent_persona, call_flow_instructions: data.call_flow_instructions })
        });
      } else { toast(data.message || 'Generation failed'); }
    } catch { toast('Failed to generate persona');  }
    updateProductPrompt(productId, 'generatingPersona', false);
  };

  const handleSaveProductPrompt = async (productId) => {
    updateProductPrompt(productId, 'saving', true);
    try {
      const pp = productPrompts[productId];
      const res = await apiFetch(`${API_URL}/products/${productId}/prompt`, {
        method: 'PUT', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ agent_persona: pp?.agent_persona || '', call_flow_instructions: pp?.call_flow_instructions || '' })
      });
      if (!res.ok) throw new Error((await res.text()) || `HTTP ${res.status}`);
      toast('Persona & call flow saved!');
    } catch(e) { toast('Failed to save: ' + (e.message || e)); }
    updateProductPrompt(productId, 'saving', false);
  };

  const orgName = selectedOrg ? selectedOrg.name : (orgs && orgs.length > 0 ? orgs[0].name : 'No organization linked');

  return (
    <div style={{ padding: '28px 32px', background: T.bg, minHeight: '100%', fontFamily: T.font }}>

      {/* Page title */}
      <div style={{ marginBottom: 24 }}>
        <h2 style={{ margin: 0, fontSize: 22, fontWeight: 700, color: T.text }}>
          {hideAiFeatures ? '📦 Products' : <>📦 <span style={{ color: T.cyan }}>Product</span> Knowledge</>}
        </h2>
        <p style={{ margin: '4px 0 0', fontSize: 13, color: T.muted }}>
          {hideAiFeatures ? 'Manage your products.' : 'Manage your products. The AI learns from this to have informed conversations.'}
        </p>
      </div>

      {/* Org card */}
      <div style={{ ...card, padding: '16px 20px', marginBottom: 16, display: 'flex', alignItems: 'center', gap: 12 }}>
        <span style={{ fontSize: 22 }}>🏛️</span>
        <div>
          <div style={{ fontSize: 10, fontWeight: 700, color: T.muted, textTransform: 'uppercase', letterSpacing: '0.07em', marginBottom: 2 }}>
            Your Organization
          </div>
          <div style={{ fontSize: 16, fontWeight: 700, color: T.cyan }}>{orgName}</div>
        </div>
      </div>

      {/* Products section */}
      {selectedOrg && (
        <div style={{ ...card, padding: '24px 28px' }}>
          {/* Header */}
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
            <h3 style={{ margin: 0, fontSize: 15, fontWeight: 700, color: T.text }}>
              📦 Products in <span style={{ color: T.cyan }}>{selectedOrg.name}</span>
            </h3>
            {!showProductInput ? (
              <button data-testid="add-product-btn"
                onClick={() => setShowProductInput(true)}
                style={{
                  padding: '8px 16px', borderRadius: 8, border: 'none',
                  background: T.accent, color: '#fff', fontWeight: 600,
                  fontSize: 13, fontFamily: T.font, cursor: 'pointer',
                }}>+ Add Product</button>
            ) : (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
              <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
                <input data-testid="product-name-input" autoFocus
                  placeholder="Product name (e.g. AdsGPT)..."
                  value={newProductName}
                  onChange={e => { setNewProductName(e.target.value); if (nameError) setNameError(''); }}
                  onKeyDown={e => { if (e.key === 'Enter') { if (!newProductName.trim()) { setNameError('Product name is required'); return; } setNameError(''); handleAddProduct(); } }}
                  style={{ ...inputStyle, width: 220, height: 36, border: nameError ? `1px solid ${T.red}` : `1px solid ${T.border}` }} />
                <button onClick={() => { if (!newProductName.trim()) { setNameError('Product name is required'); return; } setNameError(''); handleAddProduct(); }} style={{
                  padding: '0 14px', height: 36, borderRadius: 8, border: 'none',
                  background: T.green, color: '#fff', fontWeight: 600, fontSize: 13,
                  fontFamily: T.font, cursor: 'pointer',
                }}>Add</button>
                <button onClick={() => { setShowProductInput(false); setNewProductName(''); setNameError(''); }} style={{
                  padding: '0 10px', height: 36, borderRadius: 8,
                  border: `1px solid ${T.border}`, background: T.card,
                  color: T.muted, fontSize: 13, cursor: 'pointer', fontFamily: T.font,
                }}>✕</button>
              </div>
              {nameError && <span style={{ color: T.red, fontSize: '0.72rem', marginLeft: 2 }}>{nameError}</span>}
              </div>
            )}
          </div>

          {orgProducts.length === 0 ? (
            <div style={{ padding: '2rem', textAlign: 'center', color: T.muted, background: T.bg, borderRadius: 8, fontSize: 14 }}>
              No products yet. Add one to configure AI knowledge.
            </div>
          ) : (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
              {orgProducts.map(p => {
                const pp = productPrompts[p.id] || { agent_persona: '', call_flow_instructions: '', expanded: false, generating: false, saving: false };
                return (
                  <div key={p.id} style={{ background: T.bg, borderRadius: 10, padding: '16px 20px', border: `1px solid ${T.border}` }}>

                    {/* Product name row */}
                    <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 14 }}>
                      <div style={{ display: 'flex', alignItems: 'center', gap: 8, flex: 1 }}>
                        {pp.editingName ? (
                          <>
                            <input autoFocus defaultValue={p.name}
                              onBlur={e => {
                                if (e.target.value.trim() && e.target.value !== p.name) handleSaveProduct(p.id, { name: e.target.value.trim() });
                                updateProductPrompt(p.id, 'editingName', false);
                              }}
                              onKeyDown={e => { if (e.key === 'Enter') e.target.blur(); }}
                              style={{ ...inputStyle, fontWeight: 700, maxWidth: 320 }} />
                            <button onClick={() => updateProductPrompt(p.id, 'editingName', false)}
                              style={{ background: 'transparent', border: 'none', color: T.muted, cursor: 'pointer', fontSize: 13, fontFamily: T.font }}>
                              Cancel
                            </button>
                          </>
                        ) : (
                          <>
                            <span style={{ fontWeight: 700, fontSize: 14, color: T.text }}>{p.name}</span>
                            <button onClick={() => updateProductPrompt(p.id, 'editingName', true)} style={{
                              background: 'rgba(8,145,178,0.08)', border: `1px solid rgba(8,145,178,0.25)`,
                              color: T.cyan, padding: '3px 10px', borderRadius: 6,
                              cursor: 'pointer', fontSize: 12, fontWeight: 600, fontFamily: T.font,
                            }}>✏️ Edit</button>
                          </>
                        )}
                      </div>
                      {confirmDeleteId === p.id ? (
                        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                          <span style={{ color: T.amber, fontSize: 12, fontFamily: T.font }}>Delete this product?</span>
                          <button onClick={() => { setConfirmDeleteId(null); handleDeleteProduct(p.id); }} style={{
                            background: 'rgba(239,68,68,0.08)', border: `1px solid rgba(239,68,68,0.3)`,
                            color: T.red, padding: '4px 12px', borderRadius: 6,
                            cursor: 'pointer', fontSize: 12, fontWeight: 600, fontFamily: T.font,
                          }}>Confirm</button>
                          <button onClick={() => setConfirmDeleteId(null)} style={{
                            background: T.card, border: `1px solid ${T.border}`,
                            color: T.muted, padding: '4px 12px', borderRadius: 6,
                            cursor: 'pointer', fontSize: 12, fontFamily: T.font,
                          }}>Cancel</button>
                        </div>
                      ) : (
                        <button onClick={() => setConfirmDeleteId(p.id)} style={{
                          background: 'transparent', border: 'none', color: T.red,
                          cursor: 'pointer', fontSize: 13, fontFamily: T.font,
                        }}>🗑️ Remove</button>
                      )}
                    </div>

                    {!hideAiFeatures && (
                    <div style={{ marginBottom: 12 }}>
                      <div style={{ display: 'flex', gap: 10, alignItems: 'flex-end' }}>
                        <div style={{ flex: 1 }}>
                          <label style={labelStyle}>Website URL</label>
                          <input placeholder="https://..."
                            value={getWebsiteUrl(p.id) !== undefined ? getWebsiteUrl(p.id) : (p.website_url || '')}
                            onChange={e => setWebsiteUrl(p.id, e.target.value)}
                            onBlur={e => handleSaveProduct(p.id, { website_url: e.target.value })}
                            style={{ ...inputStyle, background: T.card }} />
                        </div>
                        <button onClick={() => handleScrapeWithSave(p.id)} disabled={scraping === p.id} style={{
                          height: 38, padding: '0 16px', borderRadius: 8, border: 'none', whiteSpace: 'nowrap',
                          background: scraping === p.id ? T.muted : 'linear-gradient(135deg, #0891b2, #06b6d4)',
                          color: '#fff', fontWeight: 600, fontSize: 13, fontFamily: T.font,
                          cursor: scraping === p.id ? 'not-allowed' : 'pointer',
                        }}>
                          {scraping === p.id ? '⏳ Analyzing...' : ((getWebsiteUrl(p.id) || p.website_url) ? '🔍 Scrape Website' : '🧠 AI Research')}
                        </button>
                      </div>
                      {scrapeError?.[p.id] && (
                        <div style={{ marginTop: 6, padding: '8px 12px', borderRadius: 6,
                          background: '#fef2f2', border: '1px solid #fca5a5',
                          color: '#dc2626', fontSize: 12, lineHeight: 1.5, fontFamily: T.font }}>
                          ⚠️ {scrapeError[p.id]}
                        </div>
                      )}
                    </div>
                    )}

                    {!hideAiFeatures && <div>
                      <button
                        onClick={() => updateProductPrompt(p.id, 'expanded', !pp.expanded)}
                        style={{
                          background: 'rgba(8,145,178,0.06)', border: `1px solid rgba(8,145,178,0.2)`,
                          color: T.cyan, padding: '8px 14px', borderRadius: 8, cursor: 'pointer',
                          fontSize: 13, fontWeight: 600, fontFamily: T.font, width: '100%', textAlign: 'left',
                        }}>
                        {pp.expanded ? '▾' : '▸'} Product Details, Persona & Call Flow
                        {p.scraped_info ? ' ✅' : ''}{pp.agent_persona ? ' 🎭' : ''}
                      </button>

                      {pp.expanded && (
                        <div style={{ marginTop: 12, padding: '16px 18px', background: T.card, borderRadius: 8, border: `1px solid ${T.border}` }}>

                          {p.scraped_info && (
                            <div style={{ marginBottom: 16 }}>
                              <label style={{ ...labelStyle, color: T.cyan }}>📄 AI-Extracted Info</label>
                              <textarea readOnly value={p.scraped_info} style={{
                                width: '100%', boxSizing: 'border-box', padding: 12, borderRadius: 8,
                                border: `1px solid ${T.border}`, background: T.bg,
                                color: T.sub, fontSize: 13, lineHeight: 1.6,
                                height: 220, resize: 'vertical', overflowY: 'scroll',
                                fontFamily: T.font, cursor: 'text',
                              }} />
                            </div>
                          )}

                          {p.image_urls && p.image_urls.length > 0 && (
                            <div style={{ marginBottom: 16 }}>
                              <label style={{ ...labelStyle, color: '#f59e0b' }}>🖼️ Scraped Images ({p.image_urls.length})</label>
                              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8, marginTop: 8 }}>
                                {p.image_urls.map((url, i) => (
                                  <div key={i} style={{ position: 'relative', cursor: 'pointer' }}
                                    onClick={() => window.open(url, '_blank')}>
                                    <img src={url} alt={`img-${i+1}`}
                                      style={{ width: 100, height: 70, objectFit: 'cover', borderRadius: 6,
                                        border: `1px solid ${T.border}`, background: T.bg }}
                                      onError={e => { e.target.style.display='none'; e.target.nextSibling.style.display='flex'; }} />
                                    <div style={{ display: 'none', width: 100, height: 70, borderRadius: 6,
                                      border: `1px solid ${T.border}`, background: T.bg,
                                      alignItems: 'center', justifyContent: 'center',
                                      fontSize: 11, color: T.sub, textAlign: 'center', padding: 4 }}>
                                      ❌ Failed
                                    </div>
                                    <div style={{ position: 'absolute', bottom: 0, left: 0, right: 0,
                                      background: 'rgba(0,0,0,0.65)', color: '#fff', fontSize: 9,
                                      padding: '2px 4px', borderRadius: '0 0 6px 6px',
                                      overflow: 'hidden', whiteSpace: 'nowrap', textOverflow: 'ellipsis' }}>
                                      {url.split('/').pop().split('?')[0]}
                                    </div>
                                  </div>
                                ))}
                              </div>
                            </div>
                          )}

                          {/* Manual Images */}
                          <div style={{ marginBottom: 16 }}>
                            <label style={{ ...labelStyle, color: '#8b5cf6' }}>📸 Custom Images (AI uses these labels for WhatsApp matching)</label>

                            {p.manual_images && p.manual_images.length > 0 && (
                              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 12, marginBottom: 10 }}>
                                {p.manual_images.map((img, i) => (
                                  <div key={i} style={{ display: 'flex', flexDirection: 'column', gap: 4, width: 110 }}>
                                    {/* Image thumbnail */}
                                    <div style={{ position: 'relative', cursor: 'pointer' }}
                                      onClick={() => window.open(img.url, '_blank')}>
                                      <img src={img.url} alt={img.label}
                                        style={{ width: 110, height: 75, objectFit: 'cover', borderRadius: 6,
                                          border: `2px solid #8b5cf6`, background: T.bg, display: 'block' }}
                                        onError={e => { e.target.style.display='none'; e.target.nextSibling.style.display='flex'; }} />
                                      <div style={{ display: 'none', width: 110, height: 75, borderRadius: 6,
                                        border: `2px solid #8b5cf6`, background: T.bg,
                                        alignItems: 'center', justifyContent: 'center',
                                        fontSize: 11, color: T.sub, textAlign: 'center', padding: 4 }}>
                                        ❌ Failed
                                      </div>
                                      <button
                                        onClick={e => { e.stopPropagation(); handleDeleteManualImage(p.id, i); }}
                                        style={{ position: 'absolute', top: 2, right: 2, width: 18, height: 18,
                                          borderRadius: '50%', border: 'none', background: 'rgba(239,68,68,0.9)',
                                          color: '#fff', fontSize: 10, cursor: 'pointer',
                                          display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 0,
                                          zIndex: 1 }}>
                                        ✕
                                      </button>
                                    </div>
                                    {/* Editable label */}
                                    <input
                                      defaultValue={img.label}
                                      onBlur={e => {
                                        const newLabel = e.target.value.trim();
                                        if (newLabel && newLabel !== img.label)
                                          handleUpdateImageLabel(p.id, i, newLabel);
                                      }}
                                      onKeyDown={e => { if (e.key === 'Enter') e.target.blur(); }}
                                      title="Click to edit label"
                                      style={{ width: '100%', boxSizing: 'border-box', padding: '3px 6px',
                                        fontSize: 11, borderRadius: 4, border: `1px solid ${T.border}`,
                                        background: T.card, color: T.text, fontFamily: T.font,
                                        textAlign: 'center', outline: 'none' }} />
                                  </div>
                                ))}
                              </div>
                            )}

                            {/* Upload form */}
                            <div style={{ padding: '12px 14px', background: 'rgba(139,92,246,0.05)',
                              border: '1px dashed rgba(139,92,246,0.3)', borderRadius: 8 }}>
                              <div style={{ marginBottom: 8 }}>
                                <label style={{ fontSize: 11, color: T.muted, display: 'block', marginBottom: 4, fontFamily: T.font }}>
                                  Label (AI uses this to match customer queries)
                                </label>
                                <input placeholder="e.g. Attendance Dashboard, Lavender Sofa..."
                                  value={pp.uploadLabel || ''}
                                  onChange={e => updateProductPrompt(p.id, 'uploadLabel', e.target.value)}
                                  style={{ ...inputStyle, padding: '7px 10px', fontSize: 13 }} />
                              </div>
                              <div style={{ display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
                                {/* Hidden native file input */}
                                <input type="file" accept="image/*"
                                  ref={el => { if(el) fileInputRefs.current[p.id] = el; }}
                                  style={{ display: 'none' }}
                                  onChange={e => updateProductPrompt(p.id, 'pendingFile', e.target.files[0] || null)} />
                                {/* Styled choose button */}
                                <button onClick={() => fileInputRefs.current[p.id]?.click()} style={{
                                  height: 36, padding: '0 14px', borderRadius: 8,
                                  border: `1px solid ${T.border}`, background: T.card,
                                  color: T.sub, fontWeight: 600, fontSize: 12, fontFamily: T.font, cursor: 'pointer',
                                }}>
                                  📁 {pp.pendingFile ? pp.pendingFile.name : 'Choose Image'}
                                </button>
                                <button onClick={() => handleUploadImage(p.id)} disabled={pp.uploading || !pp.pendingFile} style={{
                                  height: 36, padding: '0 16px', borderRadius: 8, border: 'none',
                                  background: (pp.uploading || !pp.pendingFile) ? T.muted : 'linear-gradient(135deg, #8b5cf6, #7c3aed)',
                                  color: '#fff', fontWeight: 600, fontSize: 12, fontFamily: T.font,
                                  cursor: (pp.uploading || !pp.pendingFile) ? 'not-allowed' : 'pointer', whiteSpace: 'nowrap',
                                }}>
                                  {pp.uploading ? '⏳ Uploading...' : '⬆️ Upload'}
                                </button>
                              </div>
                            </div>
                          </div>

                          <div style={{ marginBottom: 16 }}>
                            <label style={labelStyle}>📝 Manual Notes</label>
                            <textarea placeholder="Pricing, USPs, objection handling..."
                              defaultValue={p.manual_notes}
                              onBlur={e => handleSaveProduct(p.id, { manual_notes: e.target.value })}
                              rows={3} style={{ ...inputStyle, resize: 'vertical', minHeight: 70, lineHeight: 1.6 }} />
                          </div>

                          <div style={{ borderTop: `1px solid ${T.border}`, margin: '16px 0', paddingTop: 16 }}>
                            {(p.scraped_info || p.manual_notes) && (
                              <div style={{ marginBottom: 12 }}>
                                <button disabled={pp.generatingPersona} onClick={() => handleGeneratePersona(p.id)} style={{
                                  width: '100%', padding: '9px 16px', borderRadius: 8, border: 'none',
                                  background: 'linear-gradient(135deg, #818cf8, #6366f1)',
                                  color: '#fff', fontWeight: 600, fontSize: 13,
                                  fontFamily: T.font, cursor: pp.generatingPersona ? 'not-allowed' : 'pointer',
                                }}>
                                  {pp.generatingPersona ? '⏳ Generating from website info...' : '✨ Auto-Generate Persona & Call Flow from Website'}
                                </button>
                              </div>
                            )}

                            <div style={{ marginBottom: 12 }}>
                              <label style={{ ...labelStyle, color: '#7c3aed' }}>🎭 Agent Persona</label>
                              <textarea rows={4} value={pp.agent_persona}
                                onChange={e => updateProductPrompt(p.id, 'agent_persona', e.target.value)}
                                placeholder="e.g. You are Meera, a professional sales agent..."
                                style={{ ...inputStyle, resize: 'vertical', minHeight: 80, lineHeight: 1.6 }} />
                            </div>

                            <div style={{ marginBottom: 16 }}>
                              <label style={{ ...labelStyle, color: T.cyan }}>📋 Call Flow Instructions</label>
                              <textarea rows={5} value={pp.call_flow_instructions}
                                onChange={e => updateProductPrompt(p.id, 'call_flow_instructions', e.target.value)}
                                placeholder="e.g. Step 1: Greet. Step 2: Qualify..."
                                style={{ ...inputStyle, resize: 'vertical', minHeight: 100, lineHeight: 1.6 }} />
                            </div>

                            <div style={{ display: 'flex', gap: 10 }}>
                              <button disabled={pp.generating} onClick={() => handleGenerateProductPrompt(p.id)} style={{
                                padding: '9px 16px', borderRadius: 8, border: 'none',
                                background: 'linear-gradient(135deg, #f59e0b, #d97706)',
                                color: '#fff', fontWeight: 600, fontSize: 13,
                                fontFamily: T.font, cursor: pp.generating ? 'not-allowed' : 'pointer',
                              }}>
                                {pp.generating ? '⏳ Generating...' : '🤖 Generate Prompt'}
                              </button>
                              <button disabled={pp.saving} onClick={() => handleSaveProductPrompt(p.id)} style={{
                                padding: '9px 16px', borderRadius: 8, border: 'none',
                                background: 'linear-gradient(135deg, #10b981, #059669)',
                                color: '#fff', fontWeight: 600, fontSize: 13,
                                fontFamily: T.font, cursor: pp.saving ? 'not-allowed' : 'pointer',
                              }}>
                                {pp.saving ? '⏳ Saving...' : '💾 Save Persona & Flow'}
                              </button>
                            </div>
                          </div>
                        </div>
                      )}
                    </div>}

                  </div>
                );
              })}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
