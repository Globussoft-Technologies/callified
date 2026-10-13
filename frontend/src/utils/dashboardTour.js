export const TOUR_KEY_PREFIX = 'callified.dashboard-tour.v1.';

export function dashboardTourKey(user, org) {
  if (!user?.id) return null;
  return `${TOUR_KEY_PREFIX}${user.id}.${org?.id ?? user.org_id ?? 'default'}`;
}
