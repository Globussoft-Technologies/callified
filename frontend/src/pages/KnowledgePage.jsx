import React from 'react';
import KnowledgeBase from '../KnowledgeBase';

export default function KnowledgePage({ API_URL }) {
  return (
    <div style={{padding: '1rem'}}>
      <KnowledgeBase apiUrl={API_URL} />
    </div>
  );
}
