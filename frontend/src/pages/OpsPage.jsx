import React, { useState, useEffect } from 'react';
import OpsTab from '../components/tabs/OpsTab';

export default function OpsPage({ apiFetch, API_URL }) {
  const [tasks, setTasks] = useState([]);
  const [reports, setReports] = useState(null);

  const fetchTasks = async () => {
    try { const res = await apiFetch(`${API_URL}/tasks`); setTasks(await res.json()); } catch { /* ignore */ }
  };

  const fetchReports = async () => {
    try { const res = await apiFetch(`${API_URL}/reports`); setReports(await res.json()); } catch { /* ignore */ }
  };

  useEffect(() => {
     
    fetchTasks();
    fetchReports();
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const handleCompleteTask = async (taskId) => {
    try {
      await apiFetch(`${API_URL}/tasks/${taskId}/complete`, { method: 'PUT' });
      fetchTasks();
      fetchReports();
    } catch (e) { console.error(e); }
  };

  return <OpsTab reports={reports} tasks={tasks} handleCompleteTask={handleCompleteTask} />;
}
