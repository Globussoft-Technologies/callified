import test from 'node:test';
import assert from 'node:assert/strict';
import { claimFirstLoginTour, dashboardTourKey } from './dashboardTour.js';

test('only a first login can automatically start a tour, even after browser data is cleared', () => {
  const values = new Map();
  const storage = { getItem: key => values.get(key), setItem: (key, value) => values.set(key, value) };
  const key = dashboardTourKey({ id: 7, org_id: 9 });
  assert.equal(claimFirstLoginTour(storage, key, false), false);
  assert.equal(claimFirstLoginTour(storage, key, true), true);
  assert.equal(claimFirstLoginTour(storage, key, true), false);
  values.clear();
  assert.equal(claimFirstLoginTour(storage, key, false), false);
});

test('first-login tours tolerate blocked browser storage', () => {
  const blocked = { getItem() { throw new Error('blocked'); } };
  assert.equal(claimFirstLoginTour(blocked, 'tour', true), true);
  assert.equal(claimFirstLoginTour(blocked, 'tour', false), false);
  assert.equal(claimFirstLoginTour(blocked, null, true), false);
});
