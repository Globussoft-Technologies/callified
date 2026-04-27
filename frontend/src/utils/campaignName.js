// Defense-in-depth validation for the Campaign Name field.
//
// Why this exists: React already escapes JSX text content, so storing a
// payload like `<script>` is not directly exploitable in the UI. But the
// same string can leak into less-defended surfaces (emails, CSV exports,
// plain-text logs, third-party webhook consumers) where it would cause
// trouble. Cheaper to reject at the input boundary.
//
// Rules: 1–100 chars; no `<` or `>` (the only chars that matter for HTML
// injection). Everything else — letters, digits, spaces, punctuation,
// non-Latin scripts, emoji — is allowed so this doesn't get in the way of
// real-world campaign names ("Tom's Q1/Q2 — Bharat E Seva", etc.).

export const CAMPAIGN_NAME_MAX_LEN = 100;

const FORBIDDEN_CHARS = /[<>]/;

export function validateCampaignName(name) {
  const trimmed = (name || '').trim();
  if (!trimmed) return 'Campaign name is required.';
  if (trimmed.length > CAMPAIGN_NAME_MAX_LEN) {
    return `Campaign name must be ${CAMPAIGN_NAME_MAX_LEN} characters or fewer.`;
  }
  if (FORBIDDEN_CHARS.test(trimmed)) {
    return 'Campaign name cannot contain < or > characters.';
  }
  return null;
}
