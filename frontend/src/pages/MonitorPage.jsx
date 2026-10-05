import React from 'react';
import CallMonitor from '../CallMonitor';

export default function MonitorPage({ API_URL, apiFetch }) {
  return <CallMonitor apiUrl={API_URL} apiFetch={apiFetch} />;
}
