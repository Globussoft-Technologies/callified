import React, { useState, useEffect } from 'react';
import AnalyticsTab from '../components/tabs/AnalyticsTab';

export default function AnalyticsPage({ apiFetch, API_URL }) {
  const [analyticsData, setAnalyticsData] = useState([]);

  const fetchAnalytics = async () => {
    try { const res = await apiFetch(`${API_URL}/analytics`); setAnalyticsData(await res.json()); } catch(e){}
  };

  useEffect(() => {
    fetchAnalytics();
  }, []);

  return <AnalyticsTab analyticsData={analyticsData} />;
}
