import React, { useState, useEffect } from 'react';
import SettingsTab from '../components/tabs/SettingsTab';

export default function SettingsPage({ apiFetch, API_URL, selectedOrg, orgTimezone }) {
  // Pronunciation State
  const [pronunciations, setPronunciations] = useState([]);
  const [pronFormData, setPronFormData] = useState({ word: '', phonetic: '' });

  // System Prompt State
  const [systemPromptAuto, setSystemPromptAuto] = useState('');
  const [systemPromptCustom, setSystemPromptCustom] = useState('');
  const [promptSaving, setPromptSaving] = useState(false);
  const [promptDirty, setPromptDirty] = useState(false);

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
    if (!pronFormData.word.trim() || !pronFormData.phonetic.trim()) return;
    try {
      await apiFetch(`${API_URL}/pronunciation`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(pronFormData)
      });
      setPronFormData({ word: '', phonetic: '' });
      fetchPronunciations();
    } catch(e) { console.error(e); }
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
  };

  return (
    <SettingsTab
      orgTimezone={orgTimezone}
      handleAddPronunciation={handleAddPronunciation} pronFormData={pronFormData}
      setPronFormData={setPronFormData} pronunciations={pronunciations}
      handleDeletePronunciation={handleDeletePronunciation} selectedOrg={selectedOrg}
      promptDirty={promptDirty} handleSaveSystemPrompt={handleSaveSystemPrompt}
      promptSaving={promptSaving} systemPromptAuto={systemPromptAuto}
      systemPromptCustom={systemPromptCustom} setSystemPromptCustom={setSystemPromptCustom}
      setPromptDirty={setPromptDirty}
    />
  );
}
