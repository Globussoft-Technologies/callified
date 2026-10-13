import React, { useState, useEffect, useCallback } from 'react';
import { Routes, Route, Navigate, useLocation } from 'react-router-dom';
import ResetPasswordPage from './pages/ResetPasswordPage';
import AcceptInvitePage from './pages/AcceptInvitePage';
import SsoReturn from './pages/SsoReturn';
import MonitorPage from './pages/MonitorPage';
import SandboxPage from './pages/SandboxPage';
import AuthPage from './components/AuthPage';
import TopHeader from './components/TopHeader';
import DashboardTour from './components/DashboardTour';
import ProviderAccountsTour from './components/ProviderAccountsTour';
import MakeCallGuide from './components/MakeCallGuide';
import AnalyticsGuide from './components/AnalyticsGuide';
import ProductGuide from './components/ProductGuide';
import CrmPage from './pages/CrmPage';
import OpsPage from './pages/OpsPage';
import AnalyticsPage from './pages/AnalyticsPage';
import SettingsPage from './pages/SettingsPage';
import ProductsPage from './pages/ProductsPage';
import LogsPage from './pages/LogsPage';
import CheckInPage from './pages/CheckInPage';
import BillingPage from './pages/BillingPage';
import DndPage from './pages/DndPage';
import ScheduledCallsPage from './pages/ScheduledCallsPage';
import CampaignsPage from './pages/CampaignsPage';
import TeamPage from './pages/TeamPage';
import UserManagementPage from './pages/UserManagementPage';
import ReceptionistPage from './pages/ReceptionistPage';
import ExotelAccountsPage from './pages/ExotelAccountsPage';
import DeleteLeadsPage from './pages/DeleteLeadsPage';
import ManualDialPage from './pages/ManualDialPage';
import InteractionHistoryPage from './pages/InteractionHistoryPage';
import AgentPresencePage from './pages/AgentPresencePage';
import AgentReportPage from './pages/AgentReportPage';
import CampaignProgressPage from './pages/CampaignProgressPage';
import LeadCallStatusReportPage from './pages/LeadCallStatusReportPage';
import SubscriptionsPage from './pages/SubscriptionsPage';
import FeatureFlagsPage from './pages/FeatureFlagsPage';
import RequireRole from './components/RequireRole';
import './index.css';
import { API_URL } from './constants/api';
import { INDIAN_VOICES, INDIAN_LANGUAGES } from './constants/voices';
import { useAuth } from './contexts/AuthContext';
import { useOrg } from './contexts/OrgContext';
import { useVoice } from './contexts/VoiceContext';
import { useCall } from './contexts/CallContext';
import { useHideAiFeatures } from './hooks/useHideAiFeatures';
import { isCustomerProductionDomain } from './utils/domainFeatures';
import { dashboardTourKey } from './utils/dashboardTour';

function AdminOnly({ children, userRole }) {
  return (userRole === 'Admin' || userRole === 'SuperAdmin') ? children : <Navigate to="/crm" replace />;
}

export default function App() {
  const { authToken, currentUser, apiFetch, logout, loading, permissions, hasPermission } = useAuth();
  const { selectedOrg, orgTimezone, orgProducts, orgs, fetchOrgProducts } = useOrg();
  const { activeVoiceProvider, setActiveVoiceProvider, activeVoiceId, setActiveVoiceId, activeLanguage, setActiveLanguage, savedVoiceName, setSavedVoiceName } = useVoice();
  const { dialingId, setDialingId, webCallActive, handleDial, handleWebCall, handleCampaignDial, handleCampaignWebCall } = useCall();
  const hideAiFeatures = useHideAiFeatures();
  const hidePrelaunchAiTools = isCustomerProductionDomain();

  const location = useLocation();

  // RBAC Global State
  const userRole = currentUser?.role || 'Agent';
  const canManageProviderAccounts = ['Admin', 'SuperAdmin'].includes(userRole) && hasPermission('provider_accounts.global') && !hideAiFeatures;

  const [campaigns, setCampaigns] = useState([]);
  const [campaignsReady, setCampaignsReady] = useState(false);
  const [tourMode, setTourMode] = useState(null);
  const [tourUi, setTourUi] = useState({ moreOpen: false, providerForm: null });
  const tourKey = dashboardTourKey(currentUser, selectedOrg);

  const fetchCampaigns = async () => {
    try {
      const res = await apiFetch(`${API_URL}/campaigns`);
      const data = await res.json();
      if (!Array.isArray(data)) {
        console.warn('[fetchCampaigns] expected array, got:', { status: res.status, body: data });
        setCampaigns([]);
        return;
      }
      setCampaigns(data);
    } catch(e) {
      console.warn('[fetchCampaigns] error:', e);
    }
  };

  useEffect(() => {
    if (!currentUser) return;
    let active = true;
    setCampaignsReady(false);
    // Campaign list is scoped by role on the backend.
    // campaigns.
    if (userRole === 'Admin' || userRole === 'SuperAdmin' || userRole === 'TeamLeader' || userRole === 'Agent' || userRole === 'Executive') {
      fetchCampaigns().finally(() => { if (active) setCampaignsReady(true); });
    } else {
      setCampaigns([]);
      setCampaignsReady(true);
    }
    return () => { active = false; };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [currentUser, userRole]);

  useEffect(() => {
    if (!tourKey || permissions === null || !campaignsReady) return;
    try {
      if (localStorage.getItem(tourKey) !== 'done') setTourMode(mode => mode || 'product');
    } catch { /* tour remains available from the user menu */ }
  }, [tourKey, permissions, campaignsReady]);

  const closeProductTour = useCallback(() => {
    if (tourKey) {
      try { localStorage.setItem(tourKey, 'done'); } catch { /* private browsing */ }
    }
    setTourMode(null);
    setTourUi({ moreOpen: false, providerForm: null });
  }, [tourKey]);
  const closeProviderTour = useCallback(() => {
    setTourMode(null);
    setTourUi({ moreOpen: false, providerForm: null });
  }, []);

  // ─── PUBLIC ROUTES (no auth required) ───
  if (location.pathname === '/reset-password') {
    return <ResetPasswordPage />;
  }
  if (location.pathname === '/accept-invite') {
    return <AcceptInvitePage />;
  }
  if (location.pathname === '/sso/return') {
    return <SsoReturn />;
  }

  // ─── AUTH PAGES (after all hooks) ───
  if (loading) {
    return (
      <div style={{ minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center', background: 'linear-gradient(135deg, #0f0c29, #302b63, #24243e)' }}>
        <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: '1rem' }}>
          <div style={{ width: 40, height: 40, border: '3px solid rgba(255,255,255,0.1)', borderTop: '3px solid #a78bfa', borderRadius: '50%', animation: 'spin 0.8s linear infinite' }} />
          <span style={{ color: '#94a3b8', fontSize: '0.9rem' }}>Loading...</span>
        </div>
        <style>{`@keyframes spin { to { transform: rotate(360deg); } }`}</style>
      </div>
    );
  }

  if (!currentUser) {
    return <AuthPage redirectTo={location.pathname !== '/reset-password' ? location.pathname : '/crm'} />;
  }

  return (
    <div className="dashboard-container">
      <TopHeader
        userRole={userRole} currentUser={currentUser}
        handleLogout={logout}
        apiFetch={apiFetch}
        onStartTour={() => setTourMode('product')}
        onStartProviderTour={() => setTourMode('provider')}
        onStartCallGuide={() => setTourMode('call')}
        onStartAnalyticsGuide={() => setTourMode('analytics')}
        onStartProductGuide={() => setTourMode('products')}
        canStartCallGuide={hasPermission('calls.browser_call') && !hideAiFeatures}
        canStartAnalyticsGuide={['Admin', 'SuperAdmin'].includes(userRole) && hasPermission('reports.view')}
        canStartProductGuide={['Admin', 'SuperAdmin'].includes(userRole) && hasPermission('products.manage') && !hideAiFeatures}
        canStartProviderTour={canManageProviderAccounts}
        tourMoreOpen={tourUi.moreOpen}
      />

      {tourMode === 'product' && <DashboardTour
        fixedCard
        campaigns={campaigns}
        canViewCampaigns={hasPermission('campaigns.view') && ['Admin', 'SuperAdmin', 'TeamLeader', 'Agent', 'Executive'].includes(userRole)}
        canCreateCampaigns={hasPermission('campaigns.create')}
        canViewProducts={['Admin', 'SuperAdmin'].includes(userRole)}
        hideAiFeatures={hideAiFeatures}
        onClose={closeProductTour}
      />}
      {tourMode === 'provider' && <ProviderAccountsTour onStepChange={setTourUi} onClose={closeProviderTour} />}
      {tourMode === 'call' && <MakeCallGuide onClose={closeProviderTour} />}
      {tourMode === 'analytics' && <AnalyticsGuide onClose={closeProviderTour} />}
      {tourMode === 'products' && <ProductGuide onClose={closeProviderTour} />}

      <main className="main-content">
      <Routes>
        <Route path="/" element={<Navigate to="/crm" replace />} />
        <Route path="/crm" element={
          <CrmPage
            apiFetch={apiFetch} API_URL={API_URL}
            selectedOrg={selectedOrg} orgTimezone={orgTimezone}
            dialingId={dialingId} setDialingId={setDialingId}
            webCallActive={webCallActive}
            handleDial={handleDial} handleWebCall={handleWebCall}
            campaigns={campaigns}
            activeVoiceProvider={activeVoiceProvider} setActiveVoiceProvider={setActiveVoiceProvider}
            activeVoiceId={activeVoiceId} setActiveVoiceId={setActiveVoiceId}
            activeLanguage={activeLanguage} setActiveLanguage={setActiveLanguage}
            INDIAN_VOICES={INDIAN_VOICES} INDIAN_LANGUAGES={INDIAN_LANGUAGES}
            savedVoiceName={savedVoiceName} setSavedVoiceName={setSavedVoiceName}
            userRole={userRole} authToken={authToken}
          />
        } />
        <Route path="/campaigns" element={
          <RequireRole allow={['Admin', 'SuperAdmin', 'TeamLeader', 'Agent', 'Executive']}>
            <CampaignsPage
              key={location.pathname}
              apiFetch={apiFetch} API_URL={API_URL}
              selectedOrg={selectedOrg} orgTimezone={orgTimezone} orgProducts={orgProducts}
              dialingId={dialingId} webCallActive={webCallActive}
              handleCampaignDial={handleCampaignDial} handleCampaignWebCall={handleCampaignWebCall}
              activeVoiceProvider={activeVoiceProvider} activeVoiceId={activeVoiceId}
              activeLanguage={activeLanguage}
              INDIAN_VOICES={INDIAN_VOICES} INDIAN_LANGUAGES={INDIAN_LANGUAGES}
              campaigns={campaigns} fetchCampaigns={fetchCampaigns}
            />
          </RequireRole>
        } />
        <Route path="/campaigns/:campaignId" element={
          <RequireRole allow={['Admin', 'SuperAdmin', 'TeamLeader', 'Agent', 'Executive']}>
            <CampaignsPage
              key={location.pathname}
              apiFetch={apiFetch} API_URL={API_URL}
              selectedOrg={selectedOrg} orgTimezone={orgTimezone} orgProducts={orgProducts}
              dialingId={dialingId} webCallActive={webCallActive}
              handleCampaignDial={handleCampaignDial} handleCampaignWebCall={handleCampaignWebCall}
              activeVoiceProvider={activeVoiceProvider} activeVoiceId={activeVoiceId}
              activeLanguage={activeLanguage}
              INDIAN_VOICES={INDIAN_VOICES} INDIAN_LANGUAGES={INDIAN_LANGUAGES}
              campaigns={campaigns} fetchCampaigns={fetchCampaigns}
            />
          </RequireRole>
        } />
        <Route path="/manual-dial" element={
          <RequireRole allow={['Admin', 'SuperAdmin', 'TeamLeader', 'Agent', 'Executive']}>
            <ManualDialPage apiFetch={apiFetch} API_URL={API_URL} campaigns={campaigns} />
          </RequireRole>
        } />
        <Route path="/ops" element={<AdminOnly userRole={userRole}>{hideAiFeatures ? <Navigate to="/crm" replace /> : <OpsPage apiFetch={apiFetch} API_URL={API_URL} />}</AdminOnly>} />
        <Route path="/analytics" element={<AdminOnly userRole={userRole}><AnalyticsPage apiFetch={apiFetch} API_URL={API_URL} /></AdminOnly>} />
        {/* Hidden until the customer-facing communications module is launched. */}
        <Route path="/whatsapp" element={<Navigate to="/crm" replace />} />
        {/* Hidden until CRM credential handling and synchronization are production-ready. */}
        <Route path="/integrations" element={<Navigate to="/crm" replace />} />
        <Route path="/monitor" element={<AdminOnly userRole={userRole}>{hideAiFeatures ? <Navigate to="/crm" replace /> : <MonitorPage API_URL={API_URL} apiFetch={apiFetch} />}</AdminOnly>} />
        {/* Hidden until RAG retrieval is integrated with the voice pipeline. */}
        <Route path="/knowledge" element={<Navigate to="/crm" replace />} />
        <Route path="/sandbox" element={hidePrelaunchAiTools
          ? <Navigate to="/crm" replace />
          : <AdminOnly userRole={userRole}>{hideAiFeatures ? <Navigate to="/crm" replace /> : <SandboxPage API_URL={API_URL} />}</AdminOnly>
        } />
        <Route path="/products" element={
          <AdminOnly userRole={userRole}>
            <ProductsPage
              apiFetch={apiFetch} API_URL={API_URL}
              selectedOrg={selectedOrg} orgs={orgs}
              orgProducts={orgProducts} fetchOrgProducts={fetchOrgProducts}
            />
          </AdminOnly>
        } />
        <Route path="/settings" element={
          <SettingsPage
            apiFetch={apiFetch} API_URL={API_URL}
            selectedOrg={selectedOrg} orgTimezone={orgTimezone}
          />
        } />
        <Route path="/logs" element={hideAiFeatures ? <Navigate to="/crm" replace /> : <LogsPage API_URL={API_URL} authToken={authToken} apiFetch={apiFetch} />} />
        <Route path="/checkin" element={<CheckInPage apiFetch={apiFetch} API_URL={API_URL} />} />
        <Route path="/billing" element={hideAiFeatures ? <Navigate to="/crm" replace /> : <BillingPage apiFetch={apiFetch} API_URL={API_URL} />} />
        <Route path="/dnd" element={<AdminOnly userRole={userRole}>{hideAiFeatures ? <Navigate to="/crm" replace /> : <DndPage apiFetch={apiFetch} API_URL={API_URL} />}</AdminOnly>} />
        <Route path="/scheduled" element={
          <RequireRole allow={['Admin', 'SuperAdmin', 'TeamLeader', 'Agent', 'Executive']}>
            {hideAiFeatures ? <Navigate to="/crm" replace /> : <ScheduledCallsPage apiFetch={apiFetch} API_URL={API_URL} orgTimezone={orgTimezone} />}
          </RequireRole>
        } />
        <Route path="/interaction-history" element={<InteractionHistoryPage apiFetch={apiFetch} API_URL={API_URL} orgTimezone={orgTimezone} />} />
        <Route path="/agent-presence" element={
          <AdminOnly userRole={userRole}>
            <AgentPresencePage apiFetch={apiFetch} API_URL={API_URL} />
          </AdminOnly>
        } />
        <Route path="/agent-report" element={
          <AdminOnly userRole={userRole}>
            <AgentReportPage apiFetch={apiFetch} API_URL={API_URL} campaigns={campaigns} />
          </AdminOnly>
        } />
        <Route path="/campaign-progress" element={<AdminOnly userRole={userRole}><CampaignProgressPage apiFetch={apiFetch} API_URL={API_URL} /></AdminOnly>} />
        <Route path="/lead-call-status" element={
          <RequireRole allow={['Admin', 'SuperAdmin', 'TeamLeader']}>
            <LeadCallStatusReportPage apiFetch={apiFetch} API_URL={API_URL} campaigns={campaigns} orgTimezone={orgTimezone} />
          </RequireRole>
        } />
        <Route path="/team" element={
          <AdminOnly userRole={userRole}>
            <TeamPage apiFetch={apiFetch} API_URL={API_URL} />
          </AdminOnly>
        } />
        <Route path="/user-management" element={
          <RequireRole allow={['Admin', 'SuperAdmin']}>
            <UserManagementPage apiFetch={apiFetch} API_URL={API_URL} currentUser={currentUser} />
          </RequireRole>
        } />
        <Route path="/ai-receptionist" element={hidePrelaunchAiTools || hideAiFeatures
          ? <Navigate to="/crm" replace />
          : <ReceptionistPage />
        } />
        <Route path="/receptionist" element={<Navigate to={hidePrelaunchAiTools ? '/crm' : '/ai-receptionist'} replace />} />
        <Route path="/exotel-accounts" element={<ExotelAccountsPage tourFormMode={tourUi.providerForm} tourActive={tourMode === 'provider'} />} />
        <Route path="/delete-leads" element={<DeleteLeadsPage />} />
        <Route path="/subscriptions" element={
          <RequireRole allow={['Admin', 'SuperAdmin']}>
            <SubscriptionsPage apiFetch={apiFetch} />
          </RequireRole>
        } />
        <Route path="/feature-flags" element={
          <RequireRole allow={['Admin', 'SuperAdmin']}>
            <FeatureFlagsPage apiFetch={apiFetch} />
          </RequireRole>
        } />
        <Route path="/rag" element={<Navigate to="/knowledge" replace />} />
        <Route path="/livelogs" element={<Navigate to="/logs" replace />} />
        <Route path="*" element={<Navigate to="/crm" replace />} />
      </Routes>
      </main>

    </div>
  );
}
