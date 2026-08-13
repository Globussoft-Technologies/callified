export const ROLES = {
  ADMIN: 'Admin',
  TEAM_LEADER: 'TeamLeader',
  AGENT: 'Agent',
  EXECUTIVE: 'Executive',
};

export const isAdmin = (role) => role === 'Admin' || role === 'SuperAdmin';
export const isTeamLeader = (role) => role === 'TeamLeader';
export const isAgent = (role) => role === 'Agent';
export const isExecutive = (role) => role === 'Executive';
export const isAgentLike = (role) => isAgent(role) || isExecutive(role);

export const canManageUsers = (role) => isAdmin(role);
export const canManageAgents = (role) => isAdmin(role) || isTeamLeader(role);
export const canViewCampaigns = (role) => isAdmin(role) || isTeamLeader(role) || isAgentLike(role);
