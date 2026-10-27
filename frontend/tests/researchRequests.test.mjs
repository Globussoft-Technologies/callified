import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createResearchRequests, finishResearchRequest } from '../src/utils/researchRequests.js';

test('late restore response cannot overwrite a newly started job', () => {
  const requests = createResearchRequests();
  const restore = requests.begin('org:product');
  const start = requests.begin('org:product');
  assert.equal(requests.isCurrent('org:product', restore), false);
  assert.equal(requests.isCurrent('org:product', start), true);
});

test('organization switch invalidates requests even after returning to the same org', () => {
  const requests = createResearchRequests();
  const old = requests.begin('org:product');
  requests.invalidate();
  const next = requests.begin('org:product');
  assert.notEqual(old, next);
  assert.equal(requests.isCurrent('org:product', old), false);
  assert.equal(requests.isCurrent('org:product', next), true);
});

test('requests for separate products do not invalidate each other', () => {
  const requests = createResearchRequests();
  const first = requests.begin('org:first');
  const second = requests.begin('org:second');
  assert.equal(requests.isCurrent('org:first', first), true);
  assert.equal(requests.isCurrent('org:second', second), true);
});

test('finally clears loading even when the request returns early', async () => {
  const active = { current: 1 };
  let loading = true;
  await (async () => {
    try { return; }
    finally { finishResearchRequest(active, 1, () => { loading = false; }); }
  })();
  assert.equal(loading, false);
  assert.equal(active.current, null);
});

test('old request completion cannot clear a newer request loading state', () => {
  const active = { current: 2 };
  finishResearchRequest(active, 1, () => assert.fail('cleared newer loading state'));
  assert.equal(active.current, 2);
});
