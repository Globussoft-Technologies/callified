import React, { useState, useEffect } from 'react';
import SettingsTab from '../components/tabs/SettingsTab';
import { useVoice } from '../contexts/VoiceContext';

export default function SettingsPage({ apiFetch, API_URL, selectedOrg, orgTimezone }) {
  const { activeVoiceProvider, setActiveVoiceProvider, activeVoiceId, setActiveVoiceId, activeLanguage, setActiveLanguage, setSavedVoiceName } = useVoice();

export default function SettingsPage({ apiFetch, API_URL, selectedOrg, orgTimezone }) {
  // Pronunciation State
  const [pronunciations, setPronunciations] = useState([]);
  const [pronFormData, setPronFormData] = useState({ word: '', phonetic: '' });
  const [pronError, setPronError] = useState('');

  // System Prompt State
  const [systemPromptAuto, setSystemPromptAuto] = useState('');
  const [systemPromptCustom, setSystemPromptCustom] = useState('');
  const [promptSaving, setPromptSaving] = useState(false);
  const [promptDirty, setPromptDirty] = useState(false);
  const [promptSaved, setPromptSaved] = useState(false);

  const fetchPronunciations = async () => {
    try { const res = await apiFetch(`${API_URL}/pronunciation`); setPronunciations(await res.json()); } catch { /* ignore */ }
  };

  const fetchSystemPrompt = async (orgId) => {
    try {
      const res = await apiFetch(`${API_URL}/organizations/${orgId}/system-prompt`);
      const data = await res.json();
      setSystemPromptAuto(data.auto_generated || '');
      setSystemPromptCustom(data.custom_prompt || '');
      setPromptDirty(false);
    } catch { /* ignore */ }
  };

  useEffect(() => {
     
    fetchPronunciations();
    if (selectedOrg) fetchSystemPrompt(selectedOrg.id);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedOrg]);

  const handleAddPronunciation = async (e) => {
    e.preventDefault();
    if (!pronFormData.word.trim() || !pronFormData.phonetic.trim()) return;
    if (pronFormData.word.trim().toLowerCase() === pronFormData.phonetic.trim().toLowerCase()) {
      setPronError('The written word and phonetic version cannot be identical.');
      return;
    }
    setPronError('');
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

  const handleSaveSystemPrompt = async () => {
    if (!selectedOrg) return;
    setPromptSaving(true);
    await apiFetch(`${API_URL}/organizations/${selectedOrg.id}/system-prompt`, {
      method: 'PUT', headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({ custom_prompt: systemPromptCustom })
    });
    setPromptSaving(false);
    setPromptDirty(false);
    setPromptSaved(true);
    setTimeout(() => setPromptSaved(false), 3000);
  };

  return (
    <SettingsTab
      orgTimezone={orgTimezone}
      handleAddPronunciation={handleAddPronunciation} pronFormData={pronFormData}
      setPronFormData={setPronFormData} pronError={pronError} setPronError={setPronError}
      pronunciations={pronunciations}
      handleDeletePronunciation={handleDeletePronunciation} selectedOrg={selectedOrg}
      promptDirty={promptDirty} handleSaveSystemPrompt={handleSaveSystemPrompt}
      promptSaving={promptSaving} promptSaved={promptSaved} systemPromptAuto={systemPromptAuto}
      systemPromptCustom={systemPromptCustom} setSystemPromptCustom={setSystemPromptCustom}
      setPromptDirty={setPromptDirty}
    />
  );
}
