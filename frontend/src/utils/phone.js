// Phone utilities for Indian mobile and landline numbers.

const PHONE_DIGITS_ONLY = /\D/g;

/**
 * Strip all non-digit characters from a phone string.
 */
export function stripPhoneDigits(phone) {
  return (phone || '').replace(PHONE_DIGITS_ONLY, '');
}

/**
 * Normalize an Indian phone number input to the canonical 10-digit form
 * stored in the database:
 *   - 10-digit mobile/landline returned as-is
 *   - 11-digit domestic number with leading 0 (landline) has the 0 stripped
 *   - 12-digit number starting with 91 / +91 has the country code stripped
 *
 * Returns '' if the input is not a valid Indian phone number.
 */
export function normalizePhone(phone) {
  let digits = stripPhoneDigits(phone);

  if (digits.startsWith('0')) {
    digits = digits.slice(1);
  }

  if (digits.startsWith('91') && digits.length === 12) {
    digits = digits.slice(2);
  }

  return digits.length === 10 ? digits : '';
}

/**
 * Validate that a phone string is a valid Indian mobile or landline number.
 * Accepts formats such as:
 *   9876543210, 01112345678, +919876543210, +91-11-1234-5678
 */
export function isValidPhone(phone) {
  return normalizePhone(phone) !== '';
}

export const PHONE_VALIDATION_MESSAGE =
  'Enter a valid Indian phone number (e.g. 9876543210 or 01112345678)';
