import { useAuth } from '../contexts/AuthContext';

/**
 * Returns true when the current user should not see AI-related UI sections.
 * The flag is set per-email via the super-admin feature-flags API.
 */
export function useHideAiFeatures() {
  const { currentUser } = useAuth();
  return Boolean(currentUser?.hide_ai_features);
}
