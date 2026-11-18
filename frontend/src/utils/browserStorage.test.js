import test from 'node:test';
import assert from 'node:assert/strict';
import { clearSessionStorageData } from './browserStorage.js';

function storage(entries) {
  const values = new Map(Object.entries(entries));
  return { get length() { return values.size; }, key: index => [...values.keys()][index],
    getItem: key => values.get(key) ?? null, setItem: (key, value) => values.set(key, value), clear: () => values.clear() };
}

test('logout keeps only the selected theme and completed tours', () => {
  const local = storage({ callified_theme: 'light', authToken: 'secret', currentUser: 'private',
    'callified.dashboard-tour.v1.1.1': 'done', otherPreference: 'cached' });
  const session = storage({ token: 'secret' });
  clearSessionStorageData(local, session);
  assert.equal(local.getItem('callified_theme'), 'light');
  assert.equal(local.getItem('callified.dashboard-tour.v1.1.1'), 'done');
  assert.equal(local.getItem('authToken'), null);
  assert.equal(local.getItem('currentUser'), null);
  assert.equal(local.length, 2);
  assert.equal(session.length, 0);
});

test('blocked local storage still allows session credentials to be cleared', () => {
  const session = storage({ token: 'secret' });
  clearSessionStorageData({ get length() { throw new Error('blocked'); } }, session);
  assert.equal(session.length, 0);
});
