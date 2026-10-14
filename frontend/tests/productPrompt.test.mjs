import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fetchProductPrompt } from '../src/utils/productPrompt.js';

test('loads existing settings and accepts intentionally empty settings', async () => {
  for (const data of [
    { agent_persona: 'Meera', call_flow_instructions: 'Greet and qualify' },
    { agent_persona: '', call_flow_instructions: '' },
  ]) {
    assert.deepEqual(await fetchProductPrompt(async () => ({ ok: true, json: async () => data }), '/prompt'), data);
  }
});

test('HTTP failure does not masquerade as empty settings', async () => {
  await assert.rejects(fetchProductPrompt(async () => ({ ok: false, status: 503 }), '/prompt'), /HTTP 503/);
});

test('network and malformed responses fail safely', async () => {
  await assert.rejects(fetchProductPrompt(async () => { throw new Error('Network failure'); }, '/prompt'), /Network failure/);
  await assert.rejects(fetchProductPrompt(async () => ({ ok: true, json: async () => { throw new SyntaxError('Invalid JSON'); } }), '/prompt'), /Invalid JSON/);
  for (const data of [null, [], {}, { agent_persona: null, call_flow_instructions: '' }]) {
    await assert.rejects(fetchProductPrompt(async () => ({ ok: true, json: async () => data }), '/prompt'), /Invalid product settings/);
  }
});

test('a failed request can be retried successfully', async () => {
  let attempts = 0;
  const data = { agent_persona: 'Existing persona', call_flow_instructions: 'Existing flow' };
  const apiFetch = async () => ++attempts === 1
    ? { ok: false, status: 500 }
    : { ok: true, json: async () => data };
  await assert.rejects(fetchProductPrompt(apiFetch, '/prompt'));
  assert.deepEqual(await fetchProductPrompt(apiFetch, '/prompt'), data);
  assert.equal(attempts, 2);
});
