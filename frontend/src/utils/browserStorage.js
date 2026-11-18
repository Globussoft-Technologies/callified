import { TOUR_KEY_PREFIX } from './dashboardTour.js';

// Clear session data while retaining only non-sensitive UI preferences.
export function clearSessionStorageData(local, session) {
  try {
    const keep = [];
    for (let i = 0; i < local.length; i += 1) {
      const key = local.key(i);
      const value = local.getItem(key);
      if ((key === 'callified_theme' && ['light', 'dark'].includes(value)) ||
          (key.startsWith(TOUR_KEY_PREFIX) && value === 'done')) keep.push([key, value]);
    }
    local.clear();
    keep.forEach(([key, value]) => local.setItem(key, value));
  } catch { /* Storage may be blocked by the browser. */ }
  try { session.clear(); } catch { /* ignore */ }
}
