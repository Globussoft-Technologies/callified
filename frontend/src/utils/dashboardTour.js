export const TOUR_KEY_PREFIX = 'callified.dashboard-tour.v1.';

export function dashboardTourKey(user, org) {
  if (!user?.id) return null;
  return `${TOUR_KEY_PREFIX}${user.id}.${org?.id ?? user.org_id ?? 'default'}`;
}

export function claimFirstLoginTour(storage, key, firstLogin) {
  if (!firstLogin || !key) return false;
  try {
    if (storage?.getItem(key) === 'done') return false;
    // Mark as seen on display, so refreshing midway never restarts the tour.
    storage?.setItem(key, 'done');
  } catch { /* The auth context also consumes eligibility for this session. */ }
  return true;
}
