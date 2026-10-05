const CUSTOMER_PRODUCTION_HOST = 'app.callified.ai';

export function isCustomerProductionDomain(hostname = globalThis.location?.hostname || '') {
  return String(hostname || '').trim().toLowerCase() === CUSTOMER_PRODUCTION_HOST;
}
