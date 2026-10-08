import React, { useEffect, useRef } from 'react';
import { CalendarOutlined, DeleteOutlined, DesktopOutlined, DownOutlined, EditOutlined, ExperimentOutlined, FileTextOutlined, FormOutlined, GlobalOutlined, PhoneOutlined, TagOutlined, UserOutlined } from '@ant-design/icons';
import CampaignVoiceSettings from './campaigns/CampaignVoiceSettings';

const exampleVoice = { tts_provider: 'gemini_live', tts_voice_id: 'en-in-csagent-12', tts_language: 'hi', max_call_duration_seconds: 0 };

export default function MakeCallGuidePreview({ view }) {
  const pageRef = useRef(null);
  const tableScrollRef = useRef(null);
  const focusRef = useRef({});

  useEffect(() => {
    const page = pageRef.current;
    const target = focusRef.current[view];
    if (page && target) page.scrollTop = Math.max(0, target.offsetTop - 96);
    if (tableScrollRef.current) tableScrollRef.current.scrollLeft = view === 'call-actions'
      ? tableScrollRef.current.scrollWidth - tableScrollRef.current.clientWidth
      : 0;
  }, [view]);

  const focus = (...views) => views.includes(view) ? 'make-call-guide-focus' : 'make-call-guide-muted';
  const saveStep = view === 'call-save';

  return <div ref={pageRef} className="make-call-guide-preview" data-view={view} aria-label="Tour-only example of making a browser call" inert>
    <div className="make-call-guide-heading"><strong>Example campaign</strong><span>Guide preview · no data saved · no call placed</span></div>
    <div ref={element => { focusRef.current['call-voice'] = element; focusRef.current['call-save'] = element; }}
      className={`make-call-guide-voice ${focus('call-voice', 'call-save')}`}>
      <CampaignVoiceSettings voice={exampleVoice} tourExample />
      {saveStep && <span className="make-call-guide-save-note">Save these settings before calling</span>}
    </div>
    <div ref={element => { focusRef.current['call-account'] = element; }} className={`make-call-guide-panel ${focus('call-account')}`}>
      <strong>BROWSER CALL ACCOUNT (THIS MACHINE)</strong>
      <div className="make-call-guide-select">[Tata Tele] Example provider account · caller ID configured <span>⌄</span></div>
      <small>In a real campaign, select a saved provider account here.</small>
    </div>
    <div ref={element => { focusRef.current['call-example'] = element; focusRef.current['call-actions'] = element; }} className={`make-call-guide-leads ${focus('call-example', 'call-actions')}`}>
      <div className="make-call-guide-leads-caption"><strong>LEADS (1)</strong><span>Example row · never saved or dialed</span></div>
      <div ref={tableScrollRef} className="make-call-guide-table-scroll">
        <div className="make-call-guide-table">
          <div className="make-call-guide-table-head">
            <span>☑</span><span>NAME</span><span>PHONE</span><span>COMPANY</span><span>SOURCE</span><span>EXECUTIVE</span><span>STATUS</span><span>ACTION</span>
          </div>
          <div className="make-call-guide-table-row">
            <span className="make-call-guide-checkbox">□</span>
            <strong>Example customer</strong>
            <span>9876543210</span>
            <span>-</span>
            <span className="make-call-guide-table-select"><GlobalOutlined /> Manual <DownOutlined /></span>
            <span className="make-call-guide-table-select"><UserOutlined /> Unassigned <DownOutlined /></span>
            <span className="make-call-guide-table-select"><TagOutlined /> New <DownOutlined /></span>
            <span className="make-call-guide-table-actions">
              <i className="is-edit" aria-label="Edit"><EditOutlined /></i>
              <i className="is-dial" aria-label="AI call"><PhoneOutlined /></i>
              <i className="is-browser" aria-label="Browser Call"><DesktopOutlined /></i>
              <i className="is-test" aria-label="Simulated web call"><ExperimentOutlined /></i>
              <i className="is-file" aria-label="Transcripts and history"><FileTextOutlined /></i>
              <i className="is-note" aria-label="Note"><FormOutlined /></i>
              <i className="is-calendar" aria-label="Schedule"><CalendarOutlined /></i>
              <i className="is-delete" aria-label="Remove lead"><DeleteOutlined /></i>
            </span>
          </div>
        </div>
      </div>
    </div>
  </div>;
}
