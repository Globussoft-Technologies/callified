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

  const fetchIntegrations = async () => {
    try { const res = await apiFetch(`${API_URL}/integrations`); setIntegrations(await res.json()); } catch(e){}
  };

  useEffect(() => {
    fetchIntegrations();
  }, []);

  const handleCreateIntegration = async (e) => {
    e.preventDefault();
    setLoading(true);
    try {
      await apiFetch(`${API_URL}/integrations`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          provider: intFormData.provider,
          credentials: intFormData.credentials
        })
      });
      setIntFormData({ provider: 'HubSpot', credentials: {} });
      fetchIntegrations();
      alert("Integration saved successfully!");
    } catch(e) {
      console.error(e);
      alert("Failed to save integration.");
    }
    setLoading(false);
  };

  return (
    <IntegrationsTab
      handleCreateIntegration={handleCreateIntegration}
      intFormData={intFormData} setIntFormData={setIntFormData}
      CRM_SCHEMAS={CRM_SCHEMAS} loading={loading} integrations={integrations}
      orgTimezone={orgTimezone}
    />
  );
}
