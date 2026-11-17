import { useCallback, useEffect, useState } from 'react';
import { CloseOutlined, DeleteOutlined, PlusOutlined, SaveOutlined } from '@ant-design/icons';

const fallbackColor = '#64748b';
const draftOptions = options => options.map(option => ({ ...option, _rowKey: crypto.randomUUID() }));

export default function DispositionSettingsModal({ open, onClose, campaignId, apiFetch, apiUrl, toast }) {
  const [options, setOptions] = useState([]);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  const load = useCallback(async () => {
    if (!open || !campaignId) return;
    setLoading(true);
    setError('');
    try {
      const response = await apiFetch(`${apiUrl}/campaigns/${campaignId}/dispositions`);
      const body = await response.json().catch(() => []);
      if (!response.ok) throw new Error(body.error || `HTTP ${response.status}`);
      setOptions(draftOptions(Array.isArray(body) ? body : []));
    } catch (requestError) {
      setError(requestError.message || 'Could not load disposition options');
    } finally {
      setLoading(false);
    }
  }, [apiFetch, apiUrl, campaignId, open]);

  useEffect(() => { load(); }, [load]);
  if (!open) return null;

  const change = (index, key, value) => setOptions((current) => current.map((option, optionIndex) => (
    optionIndex === index ? { ...option, [key]: value } : option
  )));
  const add = () => setOptions(current => {
    const key = crypto.randomUUID();
    return [...current, {
      _rowKey: key, code: `custom_${key.replaceAll('-', '')}`, label: 'New disposition', color: fallbackColor,
      sort_order: (current.length + 1) * 10, is_active: true,
    }];
  });
  const save = async () => {
    setSaving(true);
    setError('');
    try {
      const codes = options.map(option => option.code.trim());
      if (codes.some(code => !code) || new Set(codes).size !== codes.length) throw new Error('Each outcome needs a unique, non-empty code.');
      const response = await apiFetch(`${apiUrl}/campaigns/${campaignId}/dispositions`, {
        method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ options: options.map(option => { const payload = { ...option }; delete payload._rowKey; return payload; }) }),
      });
      const body = await response.json().catch(() => ({}));
      if (!response.ok) throw new Error(body.error || `HTTP ${response.status}`);
      setOptions(Array.isArray(body) ? draftOptions(body) : options);
      toast('Disposition options saved');
      onClose();
    } catch (requestError) {
      setError(requestError.message || 'Could not save disposition options');
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="modal-overlay" onClick={(event) => { if (event.target === event.currentTarget) onClose(); }}>
      <div className="disposition-settings-modal" role="dialog" aria-modal="true" aria-labelledby="disposition-settings-title">
        <header className="disposition-settings-header">
          <div>
            <h3 id="disposition-settings-title">Call dispositions</h3>
            <p>AI selects one of these outcomes after every call.</p>
          </div>
          <button type="button" className="icon-button" onClick={onClose} aria-label="Close"><CloseOutlined /></button>
        </header>
        <div className="disposition-settings-body">
          {loading ? <div className="disposition-settings-empty">Loading options…</div> : options.map((option, index) => (
            <div className="disposition-option-row" key={option._rowKey}>
              <input type="color" value={option.color || fallbackColor} onChange={(event) => change(index, 'color', event.target.value)} aria-label={`${option.label} color`} />
              <label><span>Label</span><input className="form-input" value={option.label} maxLength={100} onChange={(event) => change(index, 'label', event.target.value)} /></label>
              <label><span>Code</span><input className="form-input" value={option.code} maxLength={64} onChange={(event) => change(index, 'code', event.target.value.toLowerCase().replace(/[^a-z0-9_]/g, '_'))} /></label>
              <button type="button" className="icon-button danger" onClick={() => setOptions((current) => current.filter((_, optionIndex) => optionIndex !== index))} aria-label={`Remove ${option.label}`}><DeleteOutlined /></button>
            </div>
          ))}
          {error && <div className="disposition-settings-error">{error}</div>}
          <button type="button" className="disposition-add-button" onClick={add}><PlusOutlined /> Add outcome</button>
        </div>
        <footer className="disposition-settings-footer">
          <button type="button" className="btn-secondary" onClick={onClose}>Cancel</button>
          <button type="button" className="btn-primary" onClick={save} disabled={saving || options.length === 0}><SaveOutlined /> {saving ? 'Saving…' : 'Save options'}</button>
        </footer>
      </div>
    </div>
  );
}
