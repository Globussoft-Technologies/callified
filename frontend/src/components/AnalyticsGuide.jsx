import React from 'react';
import DashboardTour from './DashboardTour';

const steps = [
  { path: '/analytics', preview: 'analytics-summary', title: 'Your call metrics', body: 'In this example, 120 calls produced a 60% pickup rate and a 15% appointment rate. Your live dashboard shows your own results.' },
  { path: '/analytics', preview: 'analytics-daily', title: 'Daily call volume', body: 'Compare the last seven days to see when calling activity increased or slowed down.' },
  { path: '/analytics', preview: 'analytics-sentiment', title: 'Customer sentiment', body: 'See how connected customers responded: positive, neutral or negative.' },
  { path: '/analytics', preview: 'analytics-campaigns', title: 'Campaign performance', body: 'Compare campaigns by calls, appointments and average quality score.' },
  { path: '/analytics', preview: 'analytics-languages', title: 'Language performance', body: 'Compare calls, appointments, conversion and quality across languages.' },
  { path: '/analytics', preview: 'analytics-filters', title: 'Filter and export', body: 'Filter by agent or download a report. Exports use your real account data, not these example figures.' },
];

export default function AnalyticsGuide({ onClose }) {
  return <DashboardTour stepsOverride={steps} tourLabel="ANALYTICS GUIDE" fixedCard onClose={onClose} />;
}
