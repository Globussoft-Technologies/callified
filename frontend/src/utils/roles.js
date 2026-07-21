export const ROLES = {
  ADMIN: 'Admin',
  TEAM_LEADER: 'TeamLeader',
  AGENT: 'Agent',
  VIEWER: 'Viewer',
};

export const isAdmin = (role) => role === 'Admin' || role === 'SuperAdmin';
export const isTeamLeader = (role) => role === 'TeamLeader';
export const isAgent = (role) => role === 'Agent';

export const canManageUsers = (role) => isAdmin(role);
export const canManageAgents = (role) => isAdmin(role) || isTeamLeader(role);
export const canViewCampaigns = (role) => isAdmin(role) || isTeamLeader(role) || isAgent(role);
