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
  // Platform-wide default is English. Was 'hi' which forced Hindi on every
  // org that hadn't explicitly saved a language — including new orgs and
  // any org where tts_provider is NULL (which used to keep this loader from
  // ever applying the saved language).
  const [activeLanguage, setActiveLanguage] = useState('en');
  const [savedVoiceName, setSavedVoiceName] = useState('');

  // Load voice settings when org is selected. Each field is applied
  // independently so a partially-configured org (e.g. language saved but no
  // provider) still picks up the saved language. Previously the language
  // load was nested under `if (vs.tts_provider)` so orgs without a provider
  // silently kept the hard-coded default.
  useEffect(() => {
    if (!currentUser || !selectedOrg) return;
    (async () => {
      try {
        const vRes = await apiFetch(`${API_URL}/organizations/${selectedOrg.id}/voice-settings`);
        const vs = await vRes.json();
        if (vs.tts_provider) setActiveVoiceProvider(vs.tts_provider);
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
