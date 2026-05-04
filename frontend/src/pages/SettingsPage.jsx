import React, { useState, useEffect } from 'react';
import SettingsTab from '../components/tabs/SettingsTab';
import { useVoice } from '../contexts/VoiceContext';
import { useToast } from '../contexts/ToastContext';

export default function SettingsPage({ apiFetch, API_URL, selectedOrg, orgs, orgProducts, orgTimezone, fetchOrgProducts }) {
  const { activeVoiceProvider, setActiveVoiceProvider, activeVoiceId, setActiveVoiceId, activeLanguage, setActiveLanguage, setSavedVoiceName } = useVoice();
  const { showToast } = useToast();
  // Pronunciation State
  const [pronunciations, setPronunciations] = useState([]);
  const [pronFormData, setPronFormData] = useState({ word: '', phonetic: '' });
  const [pronError, setPronError] = useState({ word: '', phonetic: '', api: '' });

  // Product Input State
  const [newProductName, setNewProductName] = useState('');
  const [showProductInput, setShowProductInput] = useState(false);
  const [productError, setProductError] = useState('');
  const [scraping, setScraping] = useState(null);

  // System Prompt State
  const [systemPromptAuto, setSystemPromptAuto] = useState('');
  const [systemPromptCustom, setSystemPromptCustom] = useState('');
  const [promptSaving, setPromptSaving] = useState(false);
  const [promptDirty, setPromptDirty] = useState(false);
  const [promptSaveStatus, setPromptSaveStatus] = useState(null); // 'saved' | 'error' | null

  useEffect(() => {
    fetchPronunciations();
    if (selectedOrg) fetchSystemPrompt(selectedOrg.id);
  }, [selectedOrg]);

  const fetchPronunciations = async () => {
    try { const res = await apiFetch(`${API_URL}/pronunciation`); setPronunciations(await res.json()); } catch(e){}
  };

  const fetchSystemPrompt = async (orgId) => {
    try {
      const res = await apiFetch(`${API_URL}/organizations/${orgId}/system-prompt`);
      const data = await res.json();
      setSystemPromptAuto(data.auto_generated || '');
      setSystemPromptCustom(data.custom_prompt || '');
      setPromptDirty(false);
    } catch(e) {}
  };

  const handleAddPronunciation = async (e) => {
    e.preventDefault();
    const word = pronFormData.word.trim().replace(/\s+/g, ' ');
    const phonetic = pronFormData.phonetic.trim().replace(/\s+/g, ' ');
    const errors = { word: '', phonetic: '', api: '' };
    // Allowlist mirrors the backend: Unicode letters, digits, spaces, hyphens, apostrophes, dots.
    const PRON_SAFE = /^[\w\s\-'.]+$/u;
    if (!word) errors.word = 'Written word is required.';
    if (!phonetic) errors.phonetic = 'Phonetic spelling is required.';
    if (word && !PRON_SAFE.test(word)) errors.word = "Only letters, digits, spaces, hyphens, apostrophes, and dots are allowed.";
    if (phonetic && !PRON_SAFE.test(phonetic)) errors.phonetic = "Only letters, digits, spaces, hyphens, apostrophes, and dots are allowed.";
    if (!errors.word && word.length > 100) errors.word = 'Written word must be 100 characters or fewer.';
    if (!errors.phonetic && phonetic.length > 200) errors.phonetic = 'Phonetic spelling must be 200 characters or fewer.';
    if (word && phonetic && !errors.word && !errors.phonetic && word.toLowerCase() === phonetic.toLowerCase())
      errors.phonetic = 'Phonetic spelling must differ from the written word.';
    if (errors.word || errors.phonetic) { setPronError(errors); return; }
    setPronError({ word: '', phonetic: '', api: '' });
    try {
      const res = await apiFetch(`${API_URL}/pronunciation`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ word, phonetic })
      });
      if (res.ok) {
        setPronFormData({ word: '', phonetic: '' });
        fetchPronunciations();
      } else {
        const data = await res.json();
        setPronError(prev => ({ ...prev, api: data.detail || data.error || 'Failed to add rule' }));
      }
    } catch(e) {
      setPronError(prev => ({ ...prev, api: 'Network error. Please try again.' }));
    }
  };

  const handleDeletePronunciation = async (id) => {
    try {
      await apiFetch(`${API_URL}/pronunciation/${id}`, { method: 'DELETE' });
      fetchPronunciations();
    } catch(e) { console.error(e); }
  };

  const handleAddProduct = async () => {
    if (!selectedOrg || !newProductName.trim()) return;
    setProductError('');
    try {
      const res = await apiFetch(`${API_URL}/organizations/${selectedOrg.id}/products`, {
        method: 'POST', headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({ name: newProductName.trim() })
      });
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        setProductError(data.detail || 'Failed to add product.');
        return;
      }
      setNewProductName(''); setShowProductInput(false);
      fetchOrgProducts(selectedOrg.id);
    } catch {
      setProductError('Network error. Please try again.');
    }
  };

  const handleScrapeProduct = async (productId) => {
    setScraping(productId);
    try {
      const res = await apiFetch(`${API_URL}/products/${productId}/scrape`, { method: 'POST' });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      await res.json();
      fetchOrgProducts(selectedOrg.id);
      showToast('Website scraped. Knowledge updated.', 'success');
    } catch (e) {
      console.error(e);
      showToast('Could not scrape website. Please check the URL.', 'error');
    }
    setScraping(null);
  };

  const handleSaveProduct = async (productId, updates) => {
    await apiFetch(`${API_URL}/products/${productId}`, {
      method: 'PUT', headers: {'Content-Type': 'application/json'},
      body: JSON.stringify(updates)
    });
    fetchOrgProducts(selectedOrg.id);
    if (selectedOrg) fetchSystemPrompt(selectedOrg.id);
  };

  const handleDeleteProduct = async (productId) => {
    await apiFetch(`${API_URL}/products/${productId}`, { method: 'DELETE' });
    fetchOrgProducts(selectedOrg.id);
  };

  const [voiceSaving, setVoiceSaving] = useState(false);

  const handleSaveOrgVoice = async ({ provider, voiceId, language, voiceName }) => {
    if (!selectedOrg) return;
    setVoiceSaving(true);
    try {
      const res = await apiFetch(`${API_URL}/organizations/${selectedOrg.id}/voice-settings`, {
        method: 'PUT', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ tts_provider: provider, tts_voice_id: voiceId, tts_language: language })
      });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      setActiveVoiceProvider(provider);
      setActiveVoiceId(voiceId);
      setActiveLanguage(language);
      if (voiceName) setSavedVoiceName(voiceName);
      return true;
    } catch (e) {
      return false;
    } finally {
      setVoiceSaving(false);
    }
  };

  const handleSaveSystemPrompt = async () => {
    if (!selectedOrg) return;
    setPromptSaving(true);
    setPromptSaveStatus(null);
    try {
      await apiFetch(`${API_URL}/organizations/${selectedOrg.id}/system-prompt`, {
        method: 'PUT', headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({ custom_prompt: systemPromptCustom })
      });
      setPromptDirty(false);
      setPromptSaveStatus('saved');
      setTimeout(() => setPromptSaveStatus(null), 4000);
    } catch(e) {
      setPromptSaveStatus('error');
    } finally {
      setPromptSaving(false);
    }
  };

  return (
    <SettingsTab
      orgTimezone={orgTimezone}
      handleAddPronunciation={handleAddPronunciation} pronFormData={pronFormData}
      setPronFormData={setPronFormData} pronunciations={pronunciations}
      pronError={pronError} setPronError={setPronError}
      handleDeletePronunciation={handleDeletePronunciation} selectedOrg={selectedOrg}
      orgs={orgs} showProductInput={showProductInput} setShowProductInput={setShowProductInput}
      newProductName={newProductName} setNewProductName={setNewProductName}
      productError={productError} setProductError={setProductError}
      handleAddProduct={handleAddProduct} orgProducts={orgProducts}
      handleDeleteProduct={handleDeleteProduct} handleSaveProduct={handleSaveProduct}
      scraping={scraping} handleScrapeProduct={handleScrapeProduct}
      promptDirty={promptDirty} handleSaveSystemPrompt={handleSaveSystemPrompt}
      promptSaving={promptSaving} promptSaveStatus={promptSaveStatus}
      systemPromptAuto={systemPromptAuto}
      systemPromptCustom={systemPromptCustom} setSystemPromptCustom={setSystemPromptCustom}
      setPromptDirty={setPromptDirty}
      apiFetch={apiFetch} API_URL={API_URL}
      activeVoiceProvider={activeVoiceProvider}
      activeVoiceId={activeVoiceId}
      activeLanguage={activeLanguage}
      handleSaveOrgVoice={handleSaveOrgVoice}
      voiceSaving={voiceSaving}
    />
  );
}
