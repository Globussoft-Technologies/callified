import React from 'react';
import LogsTab from '../components/tabs/LogsTab';

export default function LogsPage({ API_URL, authToken }) {
  return <LogsTab API_URL={API_URL} authToken={authToken} />;
}
