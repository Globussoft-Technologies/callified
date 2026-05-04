import React from 'react';
import { formatDateTime } from '../../utils/dateFormat';

export default function IntegrationsTab({
  handleCreateIntegration, intFormData, setIntFormData, CRM_SCHEMAS, loading, integrations, orgTimezone,
  fieldErrors = {}, setFieldErrors = () => {}
}) {
  const schema = CRM_SCHEMAS[intFormData.provider] || [{ key: 'api_key', label: 'API Key / Token', type: 'password' }, { key: 'base_url', label: 'REST API Base URL', type: 'text' }];
  const allFieldsFilled = schema.every(f => (intFormData.credentials[f.key] || '').trim().length > 0);

  return (
    <div className="integrations-container" style={{padding: '1rem'}}>
      <div className="wa-header" style={{borderBottom: '1px solid rgba(255,255,255,0.05)', marginBottom: '2rem'}}>
        <h3><span style={{color: '#38bdf8'}}>CRM</span> Integrations</h3>
        <p>Connect external CRM platforms to pull leads automatically and push call outcomes back.</p>
      </div>

      <div style={{display: 'grid', gridTemplateColumns: 'minmax(300px, 400px) 1fr', gap: '2rem'}}>
        <div className="glass-panel" style={{height: 'fit-content'}}>
          <h4 style={{marginTop: 0, marginBottom: '1.5rem', fontSize: '1.1rem', fontWeight: 600}}>Add New Connection</h4>
          <form onSubmit={handleCreateIntegration} style={{display: 'flex', flexDirection: 'column', gap: '1rem'}}>
            <div className="form-group" style={{marginBottom: 0}}>
              <label>Provider</label>
              <select className="form-input" value={intFormData.provider} onChange={e => setIntFormData({provider: e.target.value, credentials: {}})}>
                <option value="HubSpot">HubSpot</option>
                <option value="Salesforce">Salesforce</option>
                <option value="Zoho">Zoho CRM</option>
                <option value="Pipedrive">Pipedrive</option>
                <option value="ActiveCampaign">ActiveCampaign</option>
                <option value="Freshsales">Freshsales</option>
                <option value="Monday">Monday</option>
                <option value="Keap">Keap</option>
                <option value="Zendesk">Zendesk</option>
                <option value="Bitrix24">Bitrix24</option>
                <option value="Insightly">Insightly</option>
                <option value="Copper">Copper</option>
                <option value="Nimble">Nimble</option>
                <option value="Nutshell">Nutshell</option>
                <option value="Capsule">Capsule</option>
                <option value="AgileCRM">AgileCRM</option>
                <option value="SugarCRM">SugarCRM</option>
                <option value="Vtiger">Vtiger</option>
                <option value="Apptivo">Apptivo</option>
                <option value="Creatio">Creatio</option>
                <option value="Maximizer">Maximizer</option>
                <option value="Salesflare">Salesflare</option>
                <option value="Close">Close</option>
                <option value="Pipeline">Pipeline</option>
                <option value="ReallySimpleSystems">ReallySimpleSystems</option>
                <option value="EngageBay">EngageBay</option>
                <option value="Ontraport">Ontraport</option>
                <option value="Kustomer">Kustomer</option>
                <option value="Dynamics365">Dynamics365</option>
                <option value="OracleCX">OracleCX</option>
                <option value="SAPCRM">SAPCRM</option>
                <option value="NetSuite">NetSuite</option>
                <option value="SageCRM">SageCRM</option>
                <option value="Pegasystems">Pegasystems</option>
                <option value="InforCRM">InforCRM</option>
                <option value="Workbooks">Workbooks</option>
                <option value="Kintone">Kintone</option>
                <option value="Scoro">Scoro</option>
                <option value="Odoo">Odoo</option>
                <option value="Streak">Streak</option>
                <option value="LessAnnoyingCRM">LessAnnoyingCRM</option>
                <option value="Daylite">Daylite</option>
                <option value="ConvergeHub">ConvergeHub</option>
                <option value="Claritysoft">Claritysoft</option>
                <option value="AmoCRM">AmoCRM</option>
                <option value="BenchmarkONE">BenchmarkONE</option>
                <option value="Bigin">Bigin</option>
                <option value="BoomTown">BoomTown</option>
                <option value="BuddyCRM">BuddyCRM</option>
                <option value="Bullhorn">Bullhorn</option>
                <option value="CiviCRM">CiviCRM</option>
                <option value="ClientLook">ClientLook</option>
                <option value="ClientSuccess">ClientSuccess</option>
                <option value="ClientTether">ClientTether</option>
                <option value="CommandCenter">CommandCenter</option>
                <option value="ConnectWise">ConnectWise</option>
                <option value="Contactually">Contactually</option>
                <option value="Corezoid">Corezoid</option>
                <option value="CRMNext">CRMNext</option>
                <option value="Daycos">Daycos</option>
                <option value="DealerSocket">DealerSocket</option>
                <option value="Efficy">Efficy</option>
                <option value="Enquire">Enquire</option>
                <option value="Entrata">Entrata</option>
                <option value="Epsilon">Epsilon</option>
                <option value="EspoCRM">EspoCRM</option>
                <option value="Exact">Exact</option>
                <option value="Flowlu">Flowlu</option>
                <option value="FollowUpBoss">FollowUpBoss</option>
                <option value="Front">Front</option>
                <option value="Funnel">Funnel</option>
                <option value="Genesis">Genesis</option>
                <option value="GoHighLevel">GoHighLevel</option>
                <option value="GoldMine">GoldMine</option>
                <option value="GreenRope">GreenRope</option>
                <option value="Highrise">Highrise</option>
                <option value="iContact">iContact</option>
                <option value="Infusionsoft">Infusionsoft</option>
                <option value="IxactContact">IxactContact</option>
                <option value="Jobber">Jobber</option>
                <option value="Junxure">Junxure</option>
                <option value="Kaseya">Kaseya</option>
                <option value="Kixie">Kixie</option>
                <option value="Klaviyo">Klaviyo</option>
                <option value="Kommo">Kommo</option>
                <option value="LeadSquared">LeadSquared</option>
                <option value="LionDesk">LionDesk</option>
                <option value="Lusha">Lusha</option>
                <option value="Mailchimp">Mailchimp</option>
                <option value="Marketo">Marketo</option>
                <option value="Membrain">Membrain</option>
                <option value="MethodCRM">MethodCRM</option>
                <option value="MightyCRM">MightyCRM</option>
                <option value="Mindbody">Mindbody</option>
                <option value="Mixpanel">Mixpanel</option>
                <option value="Navatar">Navatar</option>
                <option value="NetHunt">NetHunt</option>
                <option value="NexTravel">NexTravel</option>
                <option value="Nurture">Nurture</option>
                <option value="OnePageCRM">OnePageCRM</option>
                <option value="Pipeliner">Pipeliner</option>
                <option value="Planhat">Planhat</option>
                <option value="Podio">Podio</option>
              </select>
            </div>
            {schema.map(field => (
              <div className="form-group" key={field.key} style={{marginBottom: 0}}>
                <label>{field.label}</label>
                <input
                  type={field.type}
                  className="form-input"
                  value={intFormData.credentials[field.key] || ''}
                  onChange={e => {
                    setIntFormData({...intFormData, credentials: {...intFormData.credentials, [field.key]: e.target.value}});
                    if (fieldErrors[field.key]) setFieldErrors({...fieldErrors, [field.key]: ''});
                  }}
                  placeholder={field.label + "..."}
                  style={fieldErrors[field.key] ? {
                    border: '1px solid rgba(239,68,68,0.6)',
                    boxShadow: '0 0 0 3px rgba(239,68,68,0.12)',
                  } : {}}
                />
                {fieldErrors[field.key] && (
                  <div style={{marginTop: '4px', fontSize: '0.78rem', color: '#fca5a5'}}>{fieldErrors[field.key]}</div>
                )}
              </div>
            ))}
            <button
              type="submit"
              className="btn-primary"
              disabled={loading || !allFieldsFilled}
              style={{marginTop: '0.5rem', opacity: (!allFieldsFilled || loading) ? 0.45 : 1, cursor: (!allFieldsFilled || loading) ? 'not-allowed' : 'pointer'}}
            >
              {loading ? 'Connecting...' : '🔌 Save Connection'}
            </button>
          </form>
        </div>

        <div className="glass-panel" style={{overflowX: 'auto'}}>
          <h4 style={{marginTop: 0, marginBottom: '1.5rem', fontSize: '1.1rem', fontWeight: 600}}>Active Connections</h4>
          <table className="leads-table">
            <thead>
              <tr>
                <th>Provider</th>
                <th>API Key (Masked)</th>
                <th>Status</th>
                <th>Last Synced</th>
              </tr>
            </thead>
            <tbody>
              {integrations.length === 0 ? (
                 <tr><td colSpan="4" style={{textAlign: "center", padding: "2rem", color: '#94a3b8'}}>No implementations hooked yet.</td></tr>
              ) : integrations.map(intg => (
                <tr key={intg.id}>
                  <td style={{fontWeight: 'bold', color: '#e2e8f0'}}>{intg.provider}</td>
                  <td style={{fontFamily: 'monospace', color: '#cbd5e1', fontSize: '0.85rem'}}>
                     {Object.keys(intg.credentials || {}).map(k => (
                        <div key={k}>{k}: ****</div>
                     ))}
                  </td>
                  <td>
                    {!intg.is_active ? (
                      <span className="badge" style={{background: 'rgba(239, 68, 68, 0.1)', color: '#f87171'}}>Disconnected</span>
                    ) : intg.last_synced_at ? (
                      <span className="badge" style={{background: 'rgba(34, 197, 94, 0.1)', color: '#4ade80'}}>Active Sync</span>
                    ) : (
                      <span className="badge" style={{background: 'rgba(234, 179, 8, 0.1)', color: '#facc15'}}>Connected</span>
                    )}
                  </td>
                  <td style={{color: '#94a3b8', fontSize: '0.9rem'}}>{intg.last_synced_at ? formatDateTime(intg.last_synced_at, orgTimezone) : 'Never'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}
