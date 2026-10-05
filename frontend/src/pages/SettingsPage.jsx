import React, { useState, useEffect } from 'react';
import { Navigate } from 'react-router-dom';
import SettingsTab from '../components/tabs/SettingsTab';
import { useAuth } from '../contexts/AuthContext';
import { useToast } from '../contexts/UIContext';

export default function SettingsPage({ apiFetch, API_URL, selectedOrg, orgTimezone }) {
  const { hasPermission } = useAuth();
  const toast = useToast();
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
  const [timezone, setTimezone] = useState(orgTimezone || 'Asia/Kolkata');
  const [timezoneSaving, setTimezoneSaving] = useState(false);

  useEffect(() => {
    setTimezone(selectedOrg?.timezone || orgTimezone || 'Asia/Kolkata');
  }, [selectedOrg, orgTimezone]);

  const fetchPronunciations = async () => {
    try {
      const res = await apiFetch(`${API_URL}/pronunciation`);
      if (!res.ok) throw new Error('Unable to load pronunciation rules');
      setPronunciations(await res.json());
    } catch (error) { toast(error.message, 'error'); }
  };

  const fetchSystemPrompt = async (orgId) => {
    try {
      const res = await apiFetch(`${API_URL}/organizations/${orgId}/system-prompt`);
      if (!res.ok) throw new Error('Unable to load AI instructions');
      const data = await res.json();
      setSystemPromptAuto(data.auto_generated || '');
      setSystemPromptCustom(data.custom_prompt || '');
      setPromptDirty(false);
    } catch (error) { toast(error.message, 'error'); }
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
        body: JSON.stringify(pronFormData)
      });
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        throw new Error(data.error || 'Unable to save pronunciation rule');
      }
      setPronFormData({ word: '', phonetic: '' });
      fetchPronunciations();
    } catch (error) { setPronError(error.message); }
  };

  const handleDeletePronunciation = async (id) => {
    try {
      const res = await apiFetch(`${API_URL}/pronunciation/${id}`, { method: 'DELETE' });
      if (!res.ok) throw new Error('Unable to remove pronunciation rule');
      fetchPronunciations();
    } catch (error) { toast(error.message, 'error'); }
  };

  const handleSaveSystemPrompt = async () => {
    if (!selectedOrg) return;
    setPromptSaving(true);
    try {
      const res = await apiFetch(`${API_URL}/organizations/${selectedOrg.id}/system-prompt`, {
        method: 'PUT', headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({ custom_prompt: systemPromptCustom })
      });
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        throw new Error(data.error || 'Unable to save AI instructions');
      }
      setPromptDirty(false);
      setPromptSaved(true);
      setTimeout(() => setPromptSaved(false), 3000);
    } catch (error) {
      toast(error.message, 'error');
    } finally {
      setPromptSaving(false);
    }
  };

  const handleSaveTimezone = async () => {
    if (!selectedOrg) return;
    setTimezoneSaving(true);
    try {
      const res = await apiFetch(`${API_URL}/organizations/${selectedOrg.id}/timezone`, {
        method: 'PUT', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ timezone }),
      });
      if (!res.ok) throw new Error('Unable to save organization timezone');
      toast('Organization timezone saved', 'success');
    } catch (error) {
      toast(error.message, 'error');
    } finally {
      setTimezoneSaving(false);
    }
  };

  if (!hasPermission('settings.manage')) return <Navigate to="/crm" replace />;

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
      timezone={timezone} setTimezone={setTimezone}
      timezoneSaving={timezoneSaving} handleSaveTimezone={handleSaveTimezone}
    />
  );
}
