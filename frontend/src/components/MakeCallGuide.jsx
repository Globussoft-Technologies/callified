import React from 'react';
import DashboardTour from './DashboardTour';

const steps = [
  { path: '/crm', preview: 'call-voice', title: 'Choose the AI voice', body: 'In a voice campaign, choose Gemini Live, the agent voice and the language. An admin can change these settings.' },
  { path: '/crm', preview: 'call-save', title: 'Save voice settings', body: 'Save the settings before calling. The example Save button is for demonstration only; nothing is changed by this guide.' },
  { path: '/crm', preview: 'call-account', title: 'Select a browser-call account', body: 'Choose the provider account for this browser. An admin must add a real account first; this example account is not connected.' },
  { path: '/crm', preview: 'call-example', title: 'Find the customer row', body: 'Add a real customer to the campaign first. Their name and phone then appear in the Leads table like this example row. This sample number is not saved.' },
  { path: '/crm', preview: 'call-actions', title: 'Lead action icons', body: 'Use the icons in a customer’s row for these actions. This example is a preview only; it cannot call or change data.' },
];

export default function MakeCallGuide({ onClose }) {
  return <DashboardTour stepsOverride={steps} tourLabel="MAKE A CALL GUIDE" fixedCard onClose={onClose} />;
}
