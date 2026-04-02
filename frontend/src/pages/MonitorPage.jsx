import React from 'react';
import CallMonitor from '../CallMonitor';

export default function MonitorPage({ API_URL }) {
  return (
    <div style={{padding: '1rem'}}>
      <CallMonitor apiUrl={API_URL} />
    </div>
  );
}
