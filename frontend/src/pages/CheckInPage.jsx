import React, { useState, useEffect } from 'react';
import CheckInTab from '../components/tabs/CheckInTab';
import { useToast } from '../contexts/UIContext';

export default function CheckInPage({ apiFetch, API_URL }) {
  const toast = useToast();
  const [fieldOpsData, setFieldOpsData] = useState({ agent_name: '', site_id: '' });
  const [punchStatus, setPunchStatus] = useState(null);
  const [punching, setPunching] = useState(false);
  const [sites, setSites] = useState([]);

  const fetchSites = async () => {
    try {
      const res = await apiFetch(`${API_URL}/sites`);
      setSites(await res.json());
    } catch (e) {
      console.error("Could not fetch sites:", e);
    }
  };

  useEffect(() => {
    fetchSites();
  }, []);

  const handlePunchIn = () => {
    if (!fieldOpsData.agent_name || !fieldOpsData.site_id) {
      toast('Please enter your name and select a site.', 'warn');
      return;
    }
    setPunching(true);
    setPunchStatus(null);
    if (!navigator.geolocation) {
      toast('Geolocation is not supported by your browser', 'error');
      setPunching(false);
      return;
    }
    navigator.geolocation.getCurrentPosition(async (position) => {
      try {
        const response = await apiFetch(`${API_URL}/punch`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            agent_name: fieldOpsData.agent_name,
            site_id: parseInt(fieldOpsData.site_id),
            lat: position.coords.latitude,
            lon: position.coords.longitude
          })
        });
        const data = await response.json();
        setPunchStatus(data);
      } catch (e) {
        setPunchStatus({ status: 'error', message: 'Network error checking in.' });
      } finally {
        setPunching(false);
      }
    }, (error) => {
      toast(`Error fetching location: ${error.message}`, 'error');
      setPunching(false);
    });
  };

  return (
    <CheckInTab
      fieldOpsData={fieldOpsData} setFieldOpsData={setFieldOpsData}
      sites={sites} handlePunchIn={handlePunchIn} punching={punching}
      punchStatus={punchStatus}
    />
  );
}
