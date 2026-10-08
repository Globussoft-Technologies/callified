import React, { useEffect, useRef } from 'react';
import ProductsTab from './tabs/ProductsTab';

const exampleProduct = {
  id: 'tour-example-product',
  name: 'Example product',
  website_url: 'https://example.com',
  scraped_info: 'Example website details: a customer-focused service with flexible plans and a dedicated support team. Verify every claim against your real website before calling.',
  manual_notes: 'Example note: Ask the customer about their needs before discussing a plan.',
  image_urls: [],
  manual_images: [],
};

export default function ProductGuidePreview({ view }) {
  const pageRef = useRef(null);
  useEffect(() => {
    const page = pageRef.current;
    const target = page?.querySelector(`[data-product-tour="${view.replace('product-', '')}"]`);
    if (page && target) page.scrollTop += target.getBoundingClientRect().top - page.getBoundingClientRect().top - 105;
  }, [view]);

  return <div ref={pageRef} className="product-guide-preview" data-view={view} aria-label="Tour-only example product" inert>
    <ProductsTab
      tourExample
      orgProducts={[exampleProduct]}
      selectedOrg={{ id: 'tour-example-org', name: 'Example organization' }}
      newProductName=""
      setNewProductName={() => {}}
      showProductInput={view === 'product-name'}
      setShowProductInput={() => {}}
      handleAddProduct={() => {}}
      handleDeleteProduct={() => {}}
      handleSaveProduct={() => {}}
      handleScrapeProduct={() => {}}
      scraping={null}
      scrapeError={{}}
      apiFetch={() => Promise.reject(new Error('Tour preview cannot use the API'))}
      API_URL="/api"
    />
  </div>;
}
