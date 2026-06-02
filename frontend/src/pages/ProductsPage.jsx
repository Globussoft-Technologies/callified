import React, { useState } from 'react';
import ProductsTab from '../components/tabs/ProductsTab';

export default function ProductsPage({ apiFetch, API_URL, selectedOrg, orgs, orgProducts, fetchOrgProducts }) {
  const [newProductName, setNewProductName] = useState('');
  const [showProductInput, setShowProductInput] = useState(false);
  const [scraping, setScraping] = useState(null);
  const [scrapeError, setScrapeError] = useState({});

  const handleAddProduct = async () => {
    if (!selectedOrg || !newProductName.trim()) return;
    await apiFetch(`${API_URL}/organizations/${selectedOrg.id}/products`, {
      method: 'POST', headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({ name: newProductName.trim() })
    });
    setNewProductName(''); setShowProductInput(false);
    fetchOrgProducts(selectedOrg.id);
  };

  const handleScrapeProduct = async (productId) => {
    setScraping(productId);
    setScrapeError(prev => ({ ...prev, [productId]: null }));
    try {
      const res = await apiFetch(`${API_URL}/products/${productId}/scrape`, { method: 'POST' });
      const text = await res.text();
      let data;
      try { data = JSON.parse(text); } catch { data = { error: text || 'unexpected response' }; }
      if (data.error) {
        setScrapeError(prev => ({ ...prev, [productId]: data.error }));
      } else {
        fetchOrgProducts(selectedOrg.id);
      }
    } catch(e) {
      console.error(e);
      setScrapeError(prev => ({ ...prev, [productId]: 'Network error — check your connection.' }));
    }
    setScraping(null);
  };

  const handleSaveProduct = async (productId, updates) => {
    await apiFetch(`${API_URL}/products/${productId}`, {
      method: 'PUT', headers: {'Content-Type': 'application/json'},
      body: JSON.stringify(updates)
    });
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
      apiFetch={apiFetch}
      API_URL={API_URL}
    />
  );
}
