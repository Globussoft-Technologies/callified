export async function fetchProductPrompt(apiFetch, url) {
  const response = await apiFetch(url);
  if (!response.ok) throw new Error(`Could not load product settings (HTTP ${response.status}).`);
  const data = await response.json();
  if (!data || typeof data !== 'object' || Array.isArray(data) ||
      typeof data.agent_persona !== 'string' || typeof data.call_flow_instructions !== 'string') {
    throw new Error('Invalid product settings response.');
  }
  return data;
}
