import React from 'react';
import Sandbox from '../Sandbox';

export default function SandboxPage({ API_URL }) {
  return (
    <div style={{padding: '1rem'}}>
      <Sandbox apiUrl={API_URL} />
    </div>
  );
}
