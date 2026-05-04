import React, { useState } from 'react';
import ProductsTab from '../components/tabs/ProductsTab';
import { useToast } from '../contexts/ToastContext';

export default function ProductsPage({ apiFetch, API_URL, selectedOrg, orgs, orgProducts, fetchOrgProducts }) {
  const { showToast } = useToast();
  const [newProductName, setNewProductName] = useState('');
  const [showProductInput, setShowProductInput] = useState(false);
  const [scraping, setScraping] = useState(null);

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
    try {
      const res = await apiFetch(`${API_URL}/products/${productId}/scrape`, { method: 'POST' });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      await res.json();
      fetchOrgProducts(selectedOrg.id);
      showToast('Website scraped. Knowledge updated.', 'success');
    } catch (e) {
      console.error(e);
      showToast('Could not scrape website. Please check the URL.', 'error');
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
      apiFetch={apiFetch}
      API_URL={API_URL}
    />
  );
}
