import React, { useState, useEffect, useRef } from 'react';
import ProductsTab from '../components/tabs/ProductsTab';

async function readResearch(response) {
  const data = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(data.detail || data.error || `Research failed (${response.status})`);
  if (!data.job?.id) throw new Error('The server returned an invalid research response');
  return data;
}

export default function ProductsPage({ apiFetch, API_URL, selectedOrg, orgs, orgProducts, fetchOrgProducts }) {
  const [newProductName, setNewProductName] = useState('');
  const [showProductInput, setShowProductInput] = useState(false);
  const [scraping, setScraping] = useState(null);
  const [scrapeError, setScrapeError] = useState({});
  const [research, setResearch] = useState({});
  const loadedResearch = useRef(new Set());
  const activeOrg = useRef(selectedOrg?.id);
  useEffect(() => { activeOrg.current = selectedOrg?.id; }, [selectedOrg?.id]);

  useEffect(() => {
    let active = true;
    const loaded = loadedResearch.current;
    const pending = new Set();
    for (const product of orgProducts || []) {
      const key = `${selectedOrg?.id}:${product.id}`;
      if (loaded.has(key)) continue;
      loaded.add(key);
      pending.add(key);
      apiFetch(`${API_URL}/products/${product.id}/research/latest`).then(async response => {
        if (response.status === 404) { pending.delete(key); return; }
        const data = await readResearch(response);
        if (active) { setResearch(previous => ({ ...previous, [product.id]: data })); pending.delete(key); }
      }).catch(() => { loaded.delete(key); pending.delete(key); });
    }
    return () => { active = false; pending.forEach(key => loaded.delete(key)); };
  }, [orgProducts, selectedOrg?.id, apiFetch, API_URL]);

  useEffect(() => {
    let active = true;
    const orgID = selectedOrg?.id;
    const productIDs = new Set((orgProducts || []).map(product => String(product.id)));
    const jobs = Object.entries(research).filter(([id, data]) => productIDs.has(id) && ['queued', 'fetching', 'summarizing'].includes(data.job.status));
    if (!jobs.length) return;
    const timer = setTimeout(() => {
      Promise.all(jobs.map(async ([id, data]) => {
        try { return { id, data: await readResearch(await apiFetch(`${API_URL}/products/${id}/research/${data.job.id}`)) }; }
        catch (error) { return { id, data: { ...data, job: { ...data.job, status: 'poll-error' } }, error: `${error.message}. Retry research to reconnect to this job.` }; }
      })).then(results => {
        if (!active || activeOrg.current !== orgID) return;
        setResearch(previous => ({ ...previous, ...Object.fromEntries(results.map(result => [result.id, result.data])) }));
        setScrapeError(previous => ({ ...previous, ...Object.fromEntries(results.map(result => [result.id, result.error || (result.data.job.status === 'failed' ? result.data.job.error : null)])) }));
      });
    }, 1500);
    return () => { active = false; clearTimeout(timer); };
  }, [research, orgProducts, selectedOrg?.id, apiFetch, API_URL]);

  const handleAddProduct = async () => {
    if (!selectedOrg || !newProductName.trim()) return;
    await apiFetch(`${API_URL}/organizations/${selectedOrg.id}/products`, {
      method: 'POST', headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({ name: newProductName.trim() })
    });
    setNewProductName(''); setShowProductInput(false);
    fetchOrgProducts(selectedOrg.id);
  };

  const handleScrapeProduct = async (productId, refresh = false) => {
    const orgID = selectedOrg?.id;
    setScraping(productId);
    setScrapeError(prev => ({ ...prev, [productId]: null }));
    try {
      const res = await apiFetch(`${API_URL}/products/${productId}/scrape`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ refresh }) });
      const data = await readResearch(res);
      if (activeOrg.current !== orgID) return;
      setResearch(previous => ({ ...previous, [productId]: data }));
      if (data.job.status === 'failed') setScrapeError(previous => ({ ...previous, [productId]: data.job.error }));
    } catch(e) {
      if (activeOrg.current !== orgID) return;
      console.error(e);
      setScrapeError(prev => ({ ...prev, [productId]: e.message || 'Network error — check your connection.' }));
    }
    setScraping(null);
  };

  const handleApplyResearch = async productId => {
    const orgID = selectedOrg?.id;
    const data = research[productId];
    if (!data?.job?.id) return;
    setResearch(previous => ({ ...previous, [productId]: { ...data, applying: true } }));
    try {
      const result = await readResearch(await apiFetch(`${API_URL}/products/${productId}/research/${data.job.id}/apply`, { method: 'POST' }));
      if (activeOrg.current !== orgID) return;
      setResearch(previous => ({ ...previous, [productId]: result }));
      setScrapeError(previous => ({ ...previous, [productId]: null }));
      fetchOrgProducts(selectedOrg.id);
    } catch (error) {
      if (activeOrg.current !== orgID) return;
      setResearch(previous => ({ ...previous, [productId]: { ...data, applying: false } }));
      setScrapeError(previous => ({ ...previous, [productId]: error.message }));
    }
  };

  const handleSaveProduct = async (productId, updates) => {
    const response = await apiFetch(`${API_URL}/products/${productId}`, {
      method: 'PUT', headers: {'Content-Type': 'application/json'},
      body: JSON.stringify(updates)
    });
    if (!response.ok) {
      const data = await response.json().catch(() => ({}));
      throw new Error(data.detail || data.error || 'Could not save product details');
    }
    fetchOrgProducts(selectedOrg.id);
  };

  const handleDeleteProduct = async (productId) => {
    await apiFetch(`${API_URL}/products/${productId}`, { method: 'DELETE' });
    fetchOrgProducts(selectedOrg.id);
  };

  return (
    <ProductsTab
      orgProducts={orgProducts || []}
      selectedOrg={selectedOrg}
      orgs={orgs}
      newProductName={newProductName}
      setNewProductName={setNewProductName}
      showProductInput={showProductInput}
      setShowProductInput={setShowProductInput}
      handleAddProduct={handleAddProduct}
      handleDeleteProduct={handleDeleteProduct}
      handleSaveProduct={handleSaveProduct}
      handleScrapeProduct={handleScrapeProduct}
      scraping={scraping}
      scrapeError={scrapeError}
      research={research}
      handleApplyResearch={handleApplyResearch}
      apiFetch={apiFetch}
      API_URL={API_URL}
      onProductsRefresh={() => fetchOrgProducts(selectedOrg?.id)}
    />
  );
}
