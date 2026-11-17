import test from 'node:test';
import assert from 'node:assert/strict';
import { reportFilename } from './reportFilename.js';

test('individual downloads identify the person, role and date range in either format', () => {
  const input = { name: 'Test Manual', role: 'Admin', from: '2026-11-01', to: '2026-11-17' };
  assert.equal(reportFilename({ ...input, format: 'pdf' }), 'Test_Manual_Admin_Report_2026-11-01_to_2026-11-17.pdf');
  assert.equal(reportFilename({ ...input, format: 'xlsx' }), 'Test_Manual_Admin_Report_2026-11-01_to_2026-11-17.xlsx');
});

test('filenames remove path characters and preserve Indian names', () => {
  assert.equal(reportFilename({ name: '../श्री / Test', campaign: 'Sales:West', from: '2026-11-17', to: '2026-11-17' }), 'श्री_Test_Report_Sales_West_2026-11-17.xlsx');
  assert.equal(reportFilename({}), 'Performance_Report_All_Dates.xlsx');
});
