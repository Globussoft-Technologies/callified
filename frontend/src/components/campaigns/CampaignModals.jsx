import React from 'react';

export default function CampaignModals({
  // Create Campaign Modal
  showCreateModal, setShowCreateModal,
  createForm, setCreateForm,
  handleCreateCampaign, loading, orgProducts,
  // Add Leads Modal
  showAddLeadsModal, setShowAddLeadsModal,
  availableLeads, selectedLeadIds, toggleLeadSelection,
  handleAddLeads,
  // CSV Import Modal
  showCsvImportModal, setShowCsvImportModal,
  csvFile, setCsvFile, handleCsvImport,
  // Edit Lead Modal
  editLead, setEditLead,
  editForm, setEditForm, handleSaveEdit,
  // Edit Campaign Modal
  showEditCampaignModal, setShowEditCampaignModal,
  editCampaignForm, setEditCampaignForm,
  handleSaveEditCampaign
}) {
  return (
    <>
      {/* Create Campaign Modal */}
      {showCreateModal && (
        <div className="modal-overlay" onClick={() => setShowCreateModal(false)}>
          <div className="glass-panel" onClick={e => e.stopPropagation()}
            style={{maxWidth: '450px', width: '90%'}}>
            <h3 style={{marginTop: 0, color: '#e2e8f0'}}>Create New Campaign</h3>
            <form onSubmit={handleCreateCampaign}>
              <div style={{marginBottom: '1rem'}}>
                <label style={{display: 'block', color: '#94a3b8', fontSize: '0.85rem', marginBottom: '4px'}}>Campaign Name</label>
                <input className="form-input" placeholder="e.g. AdsGPT March Campaign"
                  value={createForm.name} onChange={e => setCreateForm({...createForm, name: e.target.value})}
                  style={{width: '100%'}} />
              </div>
              <div style={{marginBottom: '1.5rem'}}>
                <label style={{display: 'block', color: '#94a3b8', fontSize: '0.85rem', marginBottom: '4px'}}>Product</label>
                <select className="form-input" value={createForm.product_id}
                  onChange={e => setCreateForm({...createForm, product_id: e.target.value})}
                  style={{width: '100%'}}>
                  <option value="">-- Select Product --</option>
                  {orgProducts.map(p => <option key={p.id} value={p.id}>{p.name}</option>)}
                </select>
              </div>
              <div style={{marginBottom: '1.5rem'}}>
                <label style={{display: 'block', color: '#94a3b8', fontSize: '0.85rem', marginBottom: '4px'}}>Lead Source (where did these leads come from?)</label>
                <select className="form-input" value={createForm.lead_source}
                  onChange={e => setCreateForm({...createForm, lead_source: e.target.value})}
                  style={{width: '100%'}}>
                  <option value="">-- Select Source --</option>
                  <option value="facebook">Facebook / Meta Ads</option>
                  <option value="google">Google Ads</option>
                  <option value="instagram">Instagram</option>
                  <option value="linkedin">LinkedIn</option>
                  <option value="website">Website Form</option>
                  <option value="referral">Referral</option>
                  <option value="cold">Cold Outreach</option>
                </select>
              </div>
              <div style={{display: 'flex', gap: '10px', justifyContent: 'flex-end'}}>
                <button type="button" onClick={() => setShowCreateModal(false)}
                  style={{background: 'rgba(255,255,255,0.05)', border: '1px solid rgba(255,255,255,0.1)', color: '#94a3b8', padding: '8px 16px', borderRadius: '8px', cursor: 'pointer'}}>
                  Cancel
                </button>
                <button type="submit" className="btn-primary" disabled={loading || !createForm.name.trim()}>
                  {loading ? 'Creating...' : 'Create'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Add Leads Modal */}
      {showAddLeadsModal && (
        <div className="modal-overlay" onClick={() => setShowAddLeadsModal(false)}>
          <div className="glass-panel" onClick={e => e.stopPropagation()}
            style={{maxWidth: '500px', width: '90%', maxHeight: '70vh', display: 'flex', flexDirection: 'column'}}>
            <h3 style={{marginTop: 0, color: '#e2e8f0'}}>Add Leads to Campaign</h3>
            {availableLeads.length === 0 ? (
              <p style={{color: '#64748b'}}>All leads are already in this campaign.</p>
            ) : (
              <div style={{flex: 1, overflowY: 'auto', marginBottom: '1rem'}}>
                {availableLeads.map(lead => (
                  <label key={lead.id} style={{display: 'flex', alignItems: 'center', gap: '10px', padding: '8px 4px', cursor: 'pointer', borderBottom: '1px solid rgba(255,255,255,0.05)'}}>
                    <input type="checkbox" checked={selectedLeadIds.includes(lead.id)}
                      onChange={() => toggleLeadSelection(lead.id)} />
                    <span style={{color: '#e2e8f0', fontWeight: 500}}>{lead.first_name} {lead.last_name}</span>
                    <span style={{color: '#64748b', fontSize: '0.8rem'}}>{lead.phone}</span>
                  </label>
                ))}
              </div>
            )}
            <div style={{display: 'flex', gap: '10px', justifyContent: 'flex-end'}}>
              <button onClick={() => setShowAddLeadsModal(false)}
                style={{background: 'rgba(255,255,255,0.05)', border: '1px solid rgba(255,255,255,0.1)', color: '#94a3b8', padding: '8px 16px', borderRadius: '8px', cursor: 'pointer'}}>
                Cancel
              </button>
              <button className="btn-primary" onClick={handleAddLeads} disabled={loading || selectedLeadIds.length === 0}>
                {loading ? 'Adding...' : `Add Selected (${selectedLeadIds.length})`}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* CSV Import Modal */}
      {showCsvImportModal && (
        <div className="modal-overlay" onClick={() => setShowCsvImportModal(false)}>
          <div className="glass-panel" onClick={e => e.stopPropagation()}
            style={{maxWidth: '450px', width: '90%'}}>
            <h3 style={{marginTop: 0, color: '#e2e8f0'}}>📤 Import Leads from CSV</h3>
            <p style={{color: '#94a3b8', fontSize: '0.85rem', marginBottom: '1rem'}}>
              Upload a CSV with columns: first_name, last_name, phone, source. Leads will be created and added to this campaign.
            </p>
            <input type="file" accept=".csv" onChange={e => setCsvFile(e.target.files[0])}
              style={{marginBottom: '1rem', color: '#e2e8f0', fontSize: '0.85rem'}} />
            <div style={{display: 'flex', gap: '10px', justifyContent: 'flex-end'}}>
              <button onClick={() => setShowCsvImportModal(false)}
                style={{background: 'rgba(255,255,255,0.05)', border: '1px solid rgba(255,255,255,0.1)', color: '#94a3b8', padding: '8px 16px', borderRadius: '8px', cursor: 'pointer'}}>
                Cancel
              </button>
              <button className="btn-primary" onClick={handleCsvImport} disabled={loading || !csvFile}>
                {loading ? 'Importing...' : 'Import & Add to Campaign'}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Edit Campaign Modal */}
      {showEditCampaignModal && (
        <div className="modal-overlay" onClick={() => setShowEditCampaignModal(false)}>
          <div className="glass-panel" onClick={e => e.stopPropagation()}
            style={{maxWidth: '450px', width: '90%'}}>
            <h3 style={{marginTop: 0, color: '#e2e8f0'}}>Edit Campaign</h3>
            <form onSubmit={handleSaveEditCampaign}>
              <div style={{marginBottom: '1rem'}}>
                <label style={{display: 'block', color: '#94a3b8', fontSize: '0.85rem', marginBottom: '4px'}}>Campaign Name</label>
                <input className="form-input" placeholder="e.g. AdsGPT March Campaign"
                  value={editCampaignForm.name} onChange={e => setEditCampaignForm({...editCampaignForm, name: e.target.value})}
                  style={{width: '100%'}} />
              </div>
              <div style={{marginBottom: '1.5rem'}}>
                <label style={{display: 'block', color: '#94a3b8', fontSize: '0.85rem', marginBottom: '4px'}}>Product</label>
                <select className="form-input" value={editCampaignForm.product_id}
                  onChange={e => setEditCampaignForm({...editCampaignForm, product_id: e.target.value})}
                  style={{width: '100%'}}>
                  <option value="">-- Select Product --</option>
                  {orgProducts.map(p => <option key={p.id} value={p.id}>{p.name}</option>)}
                </select>
              </div>
              <div style={{marginBottom: '1.5rem'}}>
                <label style={{display: 'block', color: '#94a3b8', fontSize: '0.85rem', marginBottom: '4px'}}>Lead Source</label>
                <select className="form-input" value={editCampaignForm.lead_source}
                  onChange={e => setEditCampaignForm({...editCampaignForm, lead_source: e.target.value})}
                  style={{width: '100%'}}>
                  <option value="">-- Select Source --</option>
                  <option value="facebook">Facebook / Meta Ads</option>
                  <option value="google">Google Ads</option>
                  <option value="instagram">Instagram</option>
                  <option value="linkedin">LinkedIn</option>
                  <option value="website">Website Form</option>
                  <option value="referral">Referral</option>
                  <option value="cold">Cold Outreach</option>
                </select>
              </div>
              <div style={{display: 'flex', gap: '10px', justifyContent: 'flex-end'}}>
                <button type="button" onClick={() => setShowEditCampaignModal(false)}
                  style={{background: 'rgba(255,255,255,0.05)', border: '1px solid rgba(255,255,255,0.1)', color: '#94a3b8', padding: '8px 16px', borderRadius: '8px', cursor: 'pointer'}}>
                  Cancel
                </button>
                <button type="submit" className="btn-primary" disabled={loading || !editCampaignForm.name.trim()}>
                  {loading ? 'Saving...' : 'Save Changes'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Edit Lead Modal */}
      {editLead && (
        <div className="modal-overlay" onClick={() => setEditLead(null)}>
          <div className="glass-panel modal-content" onClick={e => e.stopPropagation()} style={{maxWidth: '420px'}}>
            <h2 style={{marginTop: 0, marginBottom: '1.5rem'}}>✏️ Edit Lead</h2>
            <div className="form-group">
              <label>First Name</label>
              <input className="form-input" value={editForm.first_name} onChange={e => setEditForm({...editForm, first_name: e.target.value})} />
            </div>
            <div className="form-group">
              <label>Last Name</label>
              <input className="form-input" value={editForm.last_name} onChange={e => setEditForm({...editForm, last_name: e.target.value})} />
            </div>
            <div className="form-group">
              <label>Phone</label>
              <input className="form-input" value={editForm.phone} onChange={e => setEditForm({...editForm, phone: e.target.value})} />
            </div>
            <div className="form-group">
              <label>Source</label>
              <input className="form-input" value={editForm.source} onChange={e => setEditForm({...editForm, source: e.target.value})} />
            </div>
            <div style={{display: 'flex', justifyContent: 'flex-end', gap: '12px', marginTop: '1.5rem'}}>
              <button onClick={() => setEditLead(null)} style={{background: 'transparent', border: '1px solid rgba(255,255,255,0.1)', color: '#cbd5e1', padding: '8px 18px', borderRadius: '8px', cursor: 'pointer'}}>Cancel</button>
              <button className="btn-primary" onClick={handleSaveEdit}>Save</button>
            </div>
          </div>
        </div>
      )}
    </>
  );
}
