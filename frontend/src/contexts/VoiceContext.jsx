import React, { createContext, useContext, useState, useEffect } from 'react';
import { API_URL } from '../constants/api';
import { INDIAN_VOICES } from '../constants/voices';
import { useAuth } from './AuthContext';
import { useOrg } from './OrgContext';

const VoiceContext = createContext(null);

export function VoiceProvider({ children }) {
  const { apiFetch, currentUser } = useAuth();
  const { selectedOrg } = useOrg();

  const [activeVoiceProvider, setActiveVoiceProvider] = useState('elevenlabs');
  const [activeVoiceId, setActiveVoiceId] = useState('');
  const [activeLanguage, setActiveLanguage] = useState('hi');
  const [savedVoiceName, setSavedVoiceName] = useState('');

  // Load voice settings when org is selected
  useEffect(() => {
    if (!currentUser || !selectedOrg) return;
    (async () => {
      try {
        const vRes = await apiFetch(`${API_URL}/organizations/${selectedOrg.id}/voice-settings`);
        const vs = await vRes.json();
        if (vs.tts_provider) {
          setActiveVoiceProvider(vs.tts_provider);
          if (vs.tts_voice_id) {
            setActiveVoiceId(vs.tts_voice_id);
            const allV = [
              ...(INDIAN_VOICES[vs.tts_provider] || []),
              ...(INDIAN_VOICES.elevenlabs || []),
              ...(INDIAN_VOICES.smallest || [])
            ];
            const found = allV.find(v => v.id === vs.tts_voice_id);
            if (found) setSavedVoiceName(found.name);
          }
        }
        if (vs.tts_language) setActiveLanguage(vs.tts_language);
      } catch (e) {}
    })();
  }, [currentUser, selectedOrg, apiFetch]);

  return (
    <VoiceContext.Provider value={{
      activeVoiceProvider, setActiveVoiceProvider,
      activeVoiceId, setActiveVoiceId,
      activeLanguage, setActiveLanguage,
      savedVoiceName, setSavedVoiceName
    }}>
      {children}
    </VoiceContext.Provider>
  );
}

export function useVoice() {
  const ctx = useContext(VoiceContext);
  if (!ctx) throw new Error('useVoice must be used within VoiceProvider');
  return ctx;
}
