import React from 'react';
import AppSelect from '../common/AppSelect';

const sourceOptions = ['manual', 'facebook', 'google', 'instagram', 'linkedin', 'website', 'referral', 'cold', 'other']
  .map(source => ({
    value: source,
    label: source === 'other' ? 'Others' : source[0].toUpperCase() + source.slice(1),
  }));

const executiveOptions = executives => [
  { value: '', label: 'Unassigned' },
  ...(executives || []).map(executive => ({
    value: String(executive.id),
    label: executive.name || executive.full_name || executive.email,
  })),
];

export default function LeadModals({
  isModalOpen, setIsModalOpen, handleCreateLead, formData, setFormData, loading,
  editModalOpen, setEditModalOpen, editingLead, handleSaveEdit, editFormData, setEditFormData,
  executives
}) {
  return (
    <>
      {isModalOpen && (
        <div className="modal-overlay" onClick={() => setIsModalOpen(false)} role="button" tabIndex={0} onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); e.stopPropagation(); e.currentTarget.click(); } }}>
          <div className="glass-panel modal-content" onClick={e => e.stopPropagation()} role="button" tabIndex={0} onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); e.stopPropagation(); e.currentTarget.click(); } }}>
            <h2 style={{marginTop: 0, marginBottom: '2rem'}}>New Lead</h2>
            <form onSubmit={handleCreateLead}>
              <div className="form-group">
                <label>First Name</label>
                <input data-testid="lead-first-name" name="first_name" className="form-input" required value={formData.first_name} onChange={e => setFormData({...formData, first_name: e.target.value})} placeholder="e.g. John" />
              </div>
              <div className="form-group">
                <label>Last Name <span style={{color: '#64748b', fontSize: '0.8rem'}}>(Optional)</span></label>
                <input data-testid="lead-last-name" name="last_name" className="form-input" value={formData.last_name} onChange={e => setFormData({...formData, last_name: e.target.value})} placeholder="e.g. Doe" />
              </div>
              <div className="form-group">
                <label>Phone Number</label>
                <input data-testid="lead-phone" name="phone" className="form-input" required type="tel" value={formData.phone} onChange={e => setFormData({...formData, phone: e.target.value})} placeholder="+917406317771" />
              </div>
              <div className="form-group">
                <label>Company <span style={{color: '#64748b', fontSize: '0.8rem'}}>(Optional)</span></label>
                <input data-testid="lead-company" name="company" className="form-input" value={formData.company || ""} onChange={e => setFormData({...formData, company: e.target.value})} placeholder="e.g. Acme Inc." />
              </div>
              <div className="form-group">
                <label>Source</label>
                <AppSelect data-testid="lead-source" searchable size="large" value={(formData.source || 'manual').toLowerCase()}
                  options={sourceOptions} onChange={value => setFormData({...formData, source: value})} />
              </div>
              {executives && executives.length > 0 && (
                <div className="form-group">
                  <label>Executive</label>
                  <AppSelect searchable size="large" value={formData.executive_id ? String(formData.executive_id) : ''}
                    popupWidth={260} options={executiveOptions(executives)}
                    onChange={value => setFormData({...formData, executive_id: value ? parseInt(value, 10) : 0})} />
                </div>
              )}
              <div style={{display: 'flex', justifyContent: 'flex-end', gap: '12px', marginTop: '2.5rem'}}>
                <button type="button" className="btn-call" style={{borderColor: 'transparent', color: '#cbd5e1', background: 'transparent'}} onClick={() => setIsModalOpen(false)}>Cancel</button>
                <button data-testid="save-lead-btn" type="submit" className="btn-primary" disabled={loading}>
                  {loading ? 'Saving...' : 'Save Lead'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {editModalOpen && editingLead && (
        <div className="modal-overlay" onClick={() => setEditModalOpen(false)} role="button" tabIndex={0} onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); e.stopPropagation(); e.currentTarget.click(); } }}>
          <div className="glass-panel modal-content" onClick={e => e.stopPropagation()} role="button" tabIndex={0} onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); e.stopPropagation(); e.currentTarget.click(); } }}>
            <h2 style={{marginTop: 0, marginBottom: '2rem'}}>Edit Lead</h2>
            <form onSubmit={handleSaveEdit}>
              <div className="form-group">
                <label>First Name</label>
                <input name="first_name" className="form-input" required value={editFormData.first_name} onChange={e => setEditFormData({...editFormData, first_name: e.target.value})} />
              </div>
              <div className="form-group">
                <label>Last Name <span style={{color: '#64748b', fontSize: '0.8rem'}}>(Optional)</span></label>
                <input name="last_name" className="form-input" value={editFormData.last_name} onChange={e => setEditFormData({...editFormData, last_name: e.target.value})} />
              </div>
              <div className="form-group">
                <label>Phone Number</label>
                <input name="phone" className="form-input" required type="tel" value={editFormData.phone} onChange={e => setEditFormData({...editFormData, phone: e.target.value})} />
              </div>
              <div className="form-group">
                <label>Company <span style={{color: '#64748b', fontSize: '0.8rem'}}>(Optional)</span></label>
                <input name="company" className="form-input" value={editFormData.company || ''} onChange={e => setEditFormData({...editFormData, company: e.target.value})} placeholder="e.g. Acme Inc." />
              </div>
              <div className="form-group">
                <label>Source</label>
                <AppSelect searchable size="large" value={(editFormData.source || 'manual').toLowerCase()}
                  options={sourceOptions} onChange={value => setEditFormData({...editFormData, source: value})} />
              </div>
              {executives && executives.length > 0 && (
                <div className="form-group">
                  <label>Executive</label>
                  <AppSelect searchable size="large" value={editFormData.executive_id ? String(editFormData.executive_id) : ''}
                    popupWidth={260} options={executiveOptions(executives)}
                    onChange={value => setEditFormData({...editFormData, executive_id: value ? parseInt(value, 10) : 0})} />
                </div>
              )}
              <div style={{display: 'flex', justifyContent: 'flex-end', gap: '12px', marginTop: '2.5rem'}}>
                <button type="button" className="btn-call" style={{borderColor: 'transparent', color: '#cbd5e1', background: 'transparent'}} onClick={() => setEditModalOpen(false)}>Cancel</button>
                <button data-testid="update-lead-btn" type="submit" className="btn-primary" disabled={loading}>
                  {loading ? 'Saving...' : 'Update Lead'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </>
  );
}
