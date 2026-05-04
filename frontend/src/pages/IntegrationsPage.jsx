import React, { useState, useEffect } from 'react';
import IntegrationsTab from '../components/tabs/IntegrationsTab';

const CRM_SCHEMAS = {
  "Salesforce": [{ key: "client_id", label: "OAuth Client ID", type: "text" }, { key: "client_secret", label: "OAuth Client Secret", type: "password" }, { key: "instance_url", label: "Instance Base URL", type: "text" }],
  "HubSpot": [{ key: "api_key", label: "Private App Access Token", type: "password" }],
  "Zoho CRM": [{ key: "client_id", label: "Client ID", type: "text" }, { key: "client_secret", label: "Client Secret", type: "password" }, { key: "refresh_token", label: "OAuth Refresh Token", type: "password" }, { key: "base_url", label: "Data Center (e.g. www.zohoapis.com)", type: "text" }],
  "Pipedrive": [{ key: "api_key", label: "Personal API Token", type: "password" }],
  "ActiveCampaign": [{ key: "api_key", label: "Developer API Token", type: "password" }, { key: "base_url", label: "Account URL (https://xyz.api-us1.com/api/3)", type: "text" }],
  "Freshsales": [{ key: "api_key", label: "API Token", type: "password" }, { key: "base_url", label: "Bundle URL (https://domain.myfreshworks.com/crm/sales/api)", type: "text" }],
  "Zendesk": [{ key: "api_key", label: "API Token or Password", type: "password" }, { key: "base_url", label: "Subdomain Base URL", type: "text" }, { key: "email", label: "Admin Email (If Basic Auth)", type: "text" }],
  "Monday": [{ key: "api_key", label: "Personal API Token", type: "password" }, { key: "board_id", label: "Leads Board ID", type: "text" }],
  "Close": [{ key: "api_key", label: "API Key", type: "password" }]
};

export default function IntegrationsPage({ apiFetch, API_URL, orgTimezone }) {
  const [integrations, setIntegrations] = useState([]);
  const [intFormData, setIntFormData] = useState({ provider: 'HubSpot', credentials: {} });
  const [loading, setLoading] = useState(false);
  const [fieldErrors, setFieldErrors] = useState({});
  const [toast, setToast] = useState(null); // { type: 'success'|'error', message }

  // Auto-dismiss toast after 4s
  useEffect(() => {
    if (!toast) return;
    const t = setTimeout(() => setToast(null), 4000);
    return () => clearTimeout(t);
  }, [toast]);

  const fetchIntegrations = async () => {
    try { const res = await apiFetch(`${API_URL}/integrations`); setIntegrations(await res.json()); } catch(e){}
  };

  useEffect(() => {
    fetchIntegrations();
  }, []);

  const handleCreateIntegration = async (e) => {
    e.preventDefault();
    setToast(null);

    const schema = CRM_SCHEMAS[intFormData.provider] || [{ key: 'api_key', label: 'API Key / Token', type: 'password' }, { key: 'base_url', label: 'REST API Base URL', type: 'text' }];
    const errors = {};
    schema.forEach(field => {
      if (!intFormData.credentials[field.key] || !intFormData.credentials[field.key].trim()) {
        errors[field.key] = 'Token is required.';
      }
    });
    if (Object.keys(errors).length > 0) {
      setFieldErrors(errors);
      return;
    }
    setFieldErrors({});

    setLoading(true);
    try {
      const res = await apiFetch(`${API_URL}/integrations`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          provider: intFormData.provider,
          credentials: intFormData.credentials
        })
      });
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        setToast({ type: 'error', message: data.detail || 'Save rejected — check your token and try again.' });
        setLoading(false);
        return;
      }
      setIntFormData({ provider: 'HubSpot', credentials: {} });
      setToast({ type: 'success', message: 'Integration saved successfully!' });
      fetchIntegrations();
    } catch(e) {
      console.error(e);
      setToast({ type: 'error', message: 'Save rejected — unable to reach the server. Please try again.' });
    }
    setLoading(false);
  };

  return (
    <>
      <IntegrationsTab
        handleCreateIntegration={handleCreateIntegration}
        intFormData={intFormData}
        setIntFormData={(data) => {
          setIntFormData(data);
          setFieldErrors({});
          setToast(null);
        }}
        CRM_SCHEMAS={CRM_SCHEMAS} loading={loading} integrations={integrations}
        orgTimezone={orgTimezone}
        fieldErrors={fieldErrors}
        setFieldErrors={setFieldErrors}
      />

      {/* Floating toast */}
      {toast && (
        <div style={{
          position: 'fixed', bottom: '28px', right: '28px', zIndex: 9999,
          display: 'flex', alignItems: 'center', gap: '10px',
          padding: '12px 18px', borderRadius: '8px', fontSize: '0.85rem', fontWeight: 500,
          boxShadow: '0 8px 32px rgba(0,0,0,0.4)',
          color: toast.type === 'success' ? '#4ade80' : '#fca5a5',
          background: toast.type === 'success' ? 'rgba(15,23,42,0.95)' : 'rgba(15,23,42,0.95)',
          border: `1px solid ${toast.type === 'success' ? 'rgba(34,197,94,0.4)' : 'rgba(239,68,68,0.4)'}`,
          animation: 'fadeInUp 0.2s ease',
        }}>
          <span style={{ fontSize: '1rem' }}>{toast.type === 'success' ? '✓' : '⚠'}</span>
          {toast.message}
          <button onClick={() => setToast(null)} style={{
            marginLeft: '8px', background: 'none', border: 'none', cursor: 'pointer',
            color: 'inherit', opacity: 0.6, fontSize: '1rem', lineHeight: 1, padding: 0,
          }}>✕</button>
        </div>
      )}

      <style>{`
        @keyframes fadeInUp {
          from { opacity: 0; transform: translateY(12px); }
          to   { opacity: 1; transform: translateY(0); }
        }
      `}</style>
    </>
  );
}
