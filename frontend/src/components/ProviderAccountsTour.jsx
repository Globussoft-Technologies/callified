import React from 'react';
import DashboardTour from './DashboardTour';

const steps = [
  { path: '/crm', selector: '[data-tour="more-menu-trigger"]', title: 'Open More', body: 'Provider Accounts lives in the More menu at the top.' },
  { path: '/crm', selector: '[data-tour="provider-account-link"]', tourMoreOpen: true, title: 'Provider Accounts', body: 'Open Provider Accounts to manage the calling credentials used by campaigns.' },
  { path: '/exotel-accounts', selector: '[data-tour="provider-account-add"]', title: 'Add an account', body: 'Start a new provider account here. This tour will show the form without creating one.' },
  { path: '/exotel-accounts', selector: '[data-tour="provider-account-usage"]', tourFormMode: 'exotel', title: 'Usage and provider', body: 'Choose Outbound or Inbound, then a provider. Inbound currently supports Tata Tele; outbound supports Exotel or Tata Tele.' },
  { path: '/exotel-accounts', selector: '[data-tour="provider-account-name"]', tourFormMode: 'exotel', title: 'Account name', body: 'Give each account a recognizable name so you can select the right one for a campaign.' },
  { path: '/exotel-accounts', selector: '[data-tour="provider-account-exotel-fields"]', tourFormMode: 'exotel', title: 'Exotel settings', body: 'Enter the API Key, API Token, Account SID and Caller ID. App ID, App Type, Region and Subdomain can be changed here too.' },
  { path: '/exotel-accounts', selector: '[data-tour="provider-account-tata-fields"]', tourFormMode: 'tata', title: 'Tata Tele settings', body: 'For Tata outbound, enter the API Token, Caller ID and Agent Number. The Click-to-Call endpoint is optional; inbound uses a DID instead.' },
  { path: '/exotel-accounts', selector: '[data-tour="provider-account-save"]', tourFormMode: 'tata', title: 'Save your account', body: 'After the tour, enter your real credentials and Save. The tour never submits or stores the example form.' },
  { path: '/exotel-accounts', selector: '[data-tour="provider-account-edit"]', fallbackSelector: '[data-tour="provider-account-list"]', title: 'Change a saved account', body: 'Saved accounts appear here. Use Edit to update provider details, or Delete to remove an account you no longer use.' },
];

export default function ProviderAccountsTour({ onStepChange, onClose }) {
  return <DashboardTour stepsOverride={steps} tourLabel="PROVIDER ACCOUNT GUIDE" fixedCard onStepChange={onStepChange} onClose={onClose} />;
}
