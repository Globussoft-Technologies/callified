import React, { useEffect, useMemo, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { useLocation, useNavigate } from 'react-router-dom';
import { CalendarOutlined, DeleteOutlined, DesktopOutlined, EditOutlined, ExperimentOutlined, FileTextOutlined, FormOutlined, PhoneOutlined } from '@ant-design/icons';
import CampaignVoiceSettings from './campaigns/CampaignVoiceSettings';
import CampaignDetailTabs from './campaigns/CampaignDetailTabs';
import MakeCallGuidePreview from './MakeCallGuidePreview';
import AnalyticsPage from '../pages/AnalyticsPage';
import ProductGuidePreview from './ProductGuidePreview';
import './DashboardTour.css';

const leadActions = [
  { icon: <EditOutlined />, label: 'Edit', detail: 'Update lead details', className: 'is-edit' },
  { icon: <PhoneOutlined />, label: 'AI call', detail: 'Let the AI call', className: 'is-dial' },
  { icon: <DesktopOutlined />, label: 'Browser call', detail: 'Call with your mic', className: 'is-browser' },
  { icon: <ExperimentOutlined />, label: 'Sim web call', detail: 'Try a test call', className: 'is-test' },
  { icon: <FileTextOutlined />, label: 'Transcripts', detail: 'See call history', className: 'is-file' },
  { icon: <FormOutlined />, label: 'Note', detail: 'Add or edit a note', className: 'is-note' },
  { icon: <CalendarOutlined />, label: 'Schedule', detail: 'Plan a follow-up', className: 'is-calendar' },
  { icon: <DeleteOutlined />, label: 'Remove', detail: 'Remove from campaign', className: 'is-delete' },
];

function TourExampleCampaign({ view }) {
  const pageRef = useRef(null);
  const logRef = useRef(null);
  useEffect(() => {
    const page = pageRef.current;
    if (!page) return;
    page.scrollTop = view === 'call-log' && logRef.current
      ? Math.max(0, logRef.current.offsetTop - page.clientHeight * 0.42)
      : 0;
  }, [view]);
  const defaultVoice = { tts_provider: 'gemini_live', tts_voice_id: 'en-in-csagent-12', tts_language: 'hi', max_call_duration_seconds: 0 };
  return <div ref={pageRef} className="dashboard-tour-demo-page" aria-label="Tour-only example campaign detail" inert>
    <div className="dashboard-tour-demo-heading"><strong>Example campaign</strong><span>Tour preview · nothing is saved</span></div>
    <div className="dashboard-tour-demo-outcomes dashboard-tour-demo-muted">
      <strong>CALL OUTCOMES</strong>
      <div>{[['0', 'TOTAL CALLS'], ['0', 'CONNECTED'], ['0', 'COMPLETED'], ['0', 'UNANSWERED'], ['0', 'BUSY / FAILED']].map(([count, label]) => <span key={label}><b>{count}</b>{label}</span>)}</div>
    </div>
    <div className={view === 'voice' ? 'dashboard-tour-demo-focus dashboard-tour-demo-voice' : 'dashboard-tour-demo-muted dashboard-tour-demo-voice'}>
      <CampaignVoiceSettings voice={defaultVoice} tourExample />
    </div>
    <div className="dashboard-tour-demo-placeholder dashboard-tour-demo-muted"><strong>▣ BROWSER CALL ACCOUNT (THIS MACHINE)</strong><div>Choose a calling account after creating a campaign</div></div>
    <div className="dashboard-tour-demo-placeholder dashboard-tour-demo-muted"><strong>▥ LIVE CAMPAIGN ACTIVITY</strong><div>Listening for new events… start a dial to see activity here.</div></div>
    <div className="dashboard-tour-demo-placeholder dashboard-tour-demo-muted"><strong>＋ QUICK ADD:</strong><span>Name</span><span>Phone (e.g. 9876543210)</span><span className="dashboard-tour-demo-action">Add &amp; Assign</span></div>
    <div className="dashboard-tour-demo-toolbar dashboard-tour-demo-muted"><span>Add from CRM</span><span>Import CSV</span><span>Export</span><span>Dial All (0)</span><span>Auto Dial</span></div>
    <div ref={logRef} className={view === 'call-log' ? 'dashboard-tour-demo-log dashboard-tour-demo-focus' : 'dashboard-tour-demo-log dashboard-tour-demo-muted'}>
      <CampaignDetailTabs activeTab={view === 'call-log' ? 'calllog' : 'leads'} leadCount={0} callCount={0} tourExample />
      <div className="dashboard-tour-example-table" role="table" aria-label="Example call log">
        <div className="dashboard-tour-example-table-head" role="row">{['Lead', 'Phone', 'Source', 'Time', 'Outcome', 'Disposition', 'Quality', 'Duration', 'Recording'].map(label => <span role="columnheader" key={label}>{label}</span>)}</div>
        <div className="dashboard-tour-example-table-empty" role="row">No calls made yet.</div>
      </div>
    </div>
  </div>;
}

function TourExampleAnalytics({ view }) {
  const pageRef = useRef(null);
  useEffect(() => {
    const page = pageRef.current;
    const target = page?.querySelector(`[data-analytics-tour="${view.replace('analytics-', '')}"]`);
    if (page && target) {
      page.scrollTop += target.getBoundingClientRect().top - page.getBoundingClientRect().top - 100;
    }
  }, [view]);
  return <div ref={pageRef} className="analytics-guide-preview" data-view={view} aria-label="Tour-only example analytics dashboard" inert>
    <AnalyticsPage tourExample />
  </div>;
}

export default function DashboardTour({ campaigns = [], canViewCampaigns, canCreateCampaigns, canViewProducts, hideAiFeatures, stepsOverride, tourLabel = 'PRODUCT TOUR', fixedCard = false, onStepChange, onClose }) {
  const navigate = useNavigate();
  const location = useLocation();
  const [index, setIndex] = useState(0);
  const [position, setPosition] = useState(null);
  const expectedPath = useRef(null);

  const steps = useMemo(() => {
    if (stepsOverride) return stepsOverride;
    const list = [{ path: '/crm', selector: '[data-tour="dashboard-summary"]', title: 'Your dashboard', body: 'See campaign, lead, call and appointment totals at a glance.' }];
    if (canViewProducts) list.push({ path: '/products', selector: '[data-tour="products"]', title: 'Products', body: 'Add your products and their details so the AI can speak accurately about them.' });
    if (canViewCampaigns) {
      list.push(canCreateCampaigns
        ? { path: '/campaigns', selector: '[data-tour="campaign-create"]', title: 'Create a campaign', body: 'Start here to set up a campaign, then open it to manage its calls and leads.' }
        : { path: '/campaigns', selector: '[data-tour="campaign-list"]', title: 'Campaigns', body: 'Open a campaign to manage its calls and leads.' });
      const voiceCampaign = campaigns.find(c => c.channel !== 'whatsapp');
      if (voiceCampaign && !hideAiFeatures) {
        const path = `/campaigns/${voiceCampaign.id}`;
        list.push({ path, selector: '[data-tour="campaign-voice-settings"]', title: 'Voice settings', body: 'Choose the voice and language for this campaign.' });
        list.push({ path, selector: '[data-tour="campaign-call-log"]', title: 'Call log', body: 'Find call results, recordings and transcripts here.' });
      } else if (!voiceCampaign && canCreateCampaigns && !hideAiFeatures) {
        list.push({ path: '/campaigns', preview: 'voice', title: 'Voice settings', body: 'Choose the AI voice and language in a campaign’s Voice Settings panel.' });
        list.push({ path: '/campaigns', preview: 'call-log', title: 'Call log', body: 'Open Call Log to see call results, recordings and transcripts.' });
      }
    }
    return list;
  }, [campaigns, canViewCampaigns, canCreateCampaigns, canViewProducts, hideAiFeatures, stepsOverride]);

  const step = steps[index];
  const isReady = Boolean(step && location.pathname === step.path && (step.preview || position?.selector === step.selector));
  const centerCard = Boolean(step?.preview) || step?.selector === '[data-tour="campaign-voice-settings"]';
  const finish = () => onClose();
  const next = () => index + 1 >= steps.length ? finish() : setIndex(i => i + 1);
  const back = () => setIndex(i => Math.max(0, i - 1));

  useEffect(() => {
    onStepChange?.({ moreOpen: Boolean(step?.tourMoreOpen), providerForm: step?.tourFormMode || null });
  }, [step?.tourMoreOpen, step?.tourFormMode, onStepChange]);

  useEffect(() => {
    if (!step) return;
    if (window.location.pathname !== step.path) {
      expectedPath.current = step.path;
      navigate(step.path);
    }
  }, [step, navigate]);

  useEffect(() => {
    if (!step) return;
    if (location.pathname === step.path) expectedPath.current = null;
    else if (expectedPath.current !== step.path) onClose();
  }, [location.pathname, step, onClose]);

  useEffect(() => {
    if (!step || step.preview || location.pathname !== step.path) return undefined;
    let attempts = 0;
    let timer;
    let cancelled = false;
    let target;
    let originalScrollMarginTop;
    let measureFrame;
    let settleTimer;
    const measure = () => {
      if (!target || cancelled) return;
      const rect = target.getBoundingClientRect();
      const vw = window.innerWidth;
      const vh = window.innerHeight;
      const focus = {
        top: Math.max(0, rect.top - 8),
        left: Math.max(0, rect.left - 8),
        right: Math.min(vw, rect.right + 8),
        bottom: Math.min(vh, rect.bottom + 8),
      };
      focus.width = focus.right - focus.left;
      focus.height = focus.bottom - focus.top;
      const width = Math.min(360, vw - 32);
      const height = 230;
      if (step.tourFormMode && focus.right + width + 24 <= vw) {
        setPosition({ selector: step.selector, top: Math.max(16, Math.min(focus.top, vh - height - 16)), left: focus.right + 16, width, below: false });
        return;
      }
      const below = focus.bottom + 20 + height <= vh - 16;
      const top = below ? focus.bottom + 20 : Math.max(16, vh - height - 16);
      const left = Math.max(16, Math.min(focus.right - width, vw - width - 16));
      setPosition({ selector: step.selector, top, left, width, below });
    };
    const findTarget = () => {
      if (cancelled) return;
      target = document.querySelector(step.selector) || (step.fallbackSelector ? document.querySelector(step.fallbackSelector) : null);
      if (!target) {
        if (++attempts < 30) timer = window.setTimeout(findTarget, 100);
        else next(); // A permission or campaign state may hide this control.
        return;
      }
      originalScrollMarginTop = target.style.scrollMarginTop;
      target.style.scrollMarginTop = '96px';
      target.scrollIntoView({ block: 'start', behavior: 'auto' });
      target.classList.add('dashboard-tour-active-target');
      measureFrame = window.requestAnimationFrame(measure);
      settleTimer = window.setTimeout(measure, 180);
    };
    setPosition(null);
    findTarget();
    window.addEventListener('resize', measure);
    window.addEventListener('scroll', measure, true);
    return () => {
      cancelled = true;
      target?.classList.remove('dashboard-tour-active-target');
      if (target) target.style.scrollMarginTop = originalScrollMarginTop;
      window.cancelAnimationFrame(measureFrame);
      window.clearTimeout(settleTimer);
      window.clearTimeout(timer);
      window.removeEventListener('resize', measure);
      window.removeEventListener('scroll', measure, true);
    };
  // next is intentionally omitted: changing it would restart target discovery on every render.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [step, location.pathname]);

  useEffect(() => {
    const onKeyDown = (event) => {
      if (event.key === 'Escape') finish();
      if (event.key === 'ArrowRight') next();
      if (event.key === 'ArrowLeft') back();
    };
    document.addEventListener('keydown', onKeyDown);
    return () => document.removeEventListener('keydown', onKeyDown);
  });

  return createPortal(
    <div className="dashboard-tour" role="dialog" aria-labelledby="dashboard-tour-title" aria-describedby="dashboard-tour-description">
      <div className="dashboard-tour-scrim" />
      {isReady && step.preview && (step.preview.startsWith('product-')
        ? <ProductGuidePreview view={step.preview} />
        : step.preview.startsWith('analytics-')
          ? <TourExampleAnalytics view={step.preview} />
          : step.preview.startsWith('call-') && step.preview !== 'call-log'
            ? <MakeCallGuidePreview view={step.preview} />
            : <TourExampleCampaign view={step.preview} />)}
      {step && <section className={`dashboard-tour-card${isReady && position?.below && !centerCard && !fixedCard ? ' is-below' : ''}${fixedCard ? ' is-fixed-card' : ''}${step.preview ? ' is-example' : ''}${step.preview === 'call-log' ? ' is-demo-log' : ''}${step.preview?.startsWith('call-') && step.preview !== 'call-log' ? ' is-make-call-guide' : ''}${step.preview?.startsWith('analytics-') ? ' is-analytics-guide' : ''}${step.preview?.startsWith('product-') ? ' is-product-guide' : ''}${['call-example', 'call-actions'].includes(step.preview) ? ' is-make-call-row' : ''}${step.preview === 'call-actions' ? ' is-make-call-actions' : ''}`} style={fixedCard ? { width: 'min(360px, calc(100vw - 32px))' } : isReady && !centerCard ? { top: position.top, left: position.left, width: position.width } : { top: '50%', left: '50%', width: 'min(360px, calc(100vw - 32px))', transform: 'translate(-50%, -50%)' }}>
        <div className="dashboard-tour-heading">
          <span className="dashboard-tour-label">{tourLabel} · {index + 1}/{steps.length}</span>
          <button type="button" className="dashboard-tour-close" onClick={finish} aria-label="Close tour">×</button>
        </div>
        <h2 id="dashboard-tour-title">{step.title}</h2>
        <p id="dashboard-tour-description">{isReady ? step.body : 'Opening this section…'}</p>
        {isReady && step.preview === 'call-actions' && <div className="dashboard-tour-lead-actions" aria-label="Lead action icons explained">
          {leadActions.map(action => <div className="dashboard-tour-lead-action" key={action.label}>
            <span className={`dashboard-tour-lead-icon ${action.className}`}>{action.icon}</span>
            <span><strong>{action.label}</strong><small>{action.detail}</small></span>
          </div>)}
        </div>}
        <div className="dashboard-tour-actions">
          <button type="button" className="dashboard-tour-skip" onClick={finish}>Skip tour</button>
          <div>
            {index > 0 && <button type="button" className="dashboard-tour-back" onClick={back}>Back</button>}
            <button type="button" className="dashboard-tour-next" onClick={next} disabled={!isReady} autoFocus>{index + 1 === steps.length ? 'Finish' : 'Next'}</button>
          </div>
        </div>
      </section>}
    </div>, document.body
  );
}
