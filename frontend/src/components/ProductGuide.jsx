import React from 'react';
import DashboardTour from './DashboardTour';

const steps = [
  { path: '/products', preview: 'product-add', title: 'Add a product', body: 'Start by adding the product you want the AI to talk about. This guide shows an example product without saving one.' },
  { path: '/products', preview: 'product-name', title: 'Name your product', body: 'Use a recognizable name so you can select the right product when creating a campaign.' },
  { path: '/products', preview: 'product-website', title: 'Search the website', body: 'Enter the product website, then choose Scrape Website. Callified reads accessible pages and extracts details for the AI to use.' },
  { path: '/products', preview: 'product-details', title: 'Review the details', body: 'Open this section to check the extracted information. You can add manual notes if the website is incomplete or blocks access.' },
  { path: '/products', preview: 'product-generate', title: 'Auto-generate the call flow', body: 'After website details or manual notes are available, generate a draft agent persona and call flow. Review the draft before using it.' },
  { path: '/products', preview: 'product-persona', title: 'Check the agent persona', body: 'Edit how the AI should introduce itself and speak to customers for this product.' },
  { path: '/products', preview: 'product-flow', title: 'Check the call flow', body: 'Edit the conversation steps, such as greeting, qualification, product explanation and follow-up.' },
  { path: '/products', preview: 'product-save', title: 'Save persona and flow', body: 'Save the reviewed persona and call flow for this product. The example button in this guide does not save anything.' },
];

export default function ProductGuide({ onClose }) {
  return <DashboardTour stepsOverride={steps} tourLabel="PRODUCT GUIDE" fixedCard onClose={onClose} />;
}
