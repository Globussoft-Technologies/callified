import test from 'node:test';
import assert from 'node:assert/strict';
import { normalizePhone } from './phone.js';

test('accepts supported Indian mobile and landline formats', () => {
  for (const [input, expected] of [['9876543210', '9876543210'], ['+91 98765 43210', '9876543210'], ['01112345678', '1112345678'], ['+91-11-1234-5678', '1112345678']]) {
    assert.equal(normalizePhone(input), expected);
  }
});
test('rejects extra characters and invalid lengths rather than silently stripping them', () => {
  for (const input of ['123', '1234567890123', 'abc9876543210', '+449876543210', '98+76543210', '9876543210 ext 1', '9876543210<script>']) {
    assert.equal(normalizePhone(input), '', input);
  }
});
