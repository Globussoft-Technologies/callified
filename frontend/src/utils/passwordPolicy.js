// Shared password policy — applied on signup, invite, and reset-password.
// Policy (NIST SP 800-63B aligned):
//   1. Min 8 characters
//   2. HIBP k-anonymity breach check  ← real-time, privacy-safe
//   3. Static top-common-password blocklist (fast, offline fallback)
//   4. No mandatory uppercase/symbol requirements (passphrases allowed)

// ── Top common passwords (blocked regardless of HIBP result) ──────────────
const BLOCKED = new Set([
  // Classic top-10 (shorter than 8 but worth listing for the block message)
  'password','123456','12345678','qwerty','abc123','letmein','monkey','1234567',
  // 8+ char commons
  'password1','password123','passw0rd','p@ssword','p@ssw0rd',
  '123456789','1234567890','12345678910','87654321',
  '11111111','00000000','99999999','12341234',
  'qwerty123','qwertyui','qwertyuiop','qwerty12',
  '1q2w3e4r','1q2w3e4r5t','zxcvbnm1','zxcvbnm',
  'iloveyou','iloveyou1','iloveyou!',
  'trustno1','welcome1','welcome123',
  'changeme','changeme1','changeit',
  'admin123','admin1234','administrator',
  'dragon123','sunshine1','sunshine',
  'baseball1','football1','basketball',
  'superman1','batman123','spiderman',
  'michael1','jennifer1','charlie1','jessica1',
  'abc12345','abcdefgh','abcd1234',
  'master123','shadow123','hello123','hello1234',
  'login123','default1','test1234','test123',
  'pass1234','pass12345','secret123',
  'correct1','battery1','staple123', // common passphrase components used as passwords
]);

// ── HIBP k-anonymity check (privacy-safe: only first 5 SHA-1 hex chars sent) ─
async function sha1Hex(str) {
  const buf = await crypto.subtle.digest('SHA-1', new TextEncoder().encode(str));
  return Array.from(new Uint8Array(buf))
    .map(b => b.toString(16).padStart(2, '0'))
    .join('')
    .toUpperCase();
}

/**
 * Check password against the Have I Been Pwned database.
 * Uses k-anonymity: only the first 5 chars of the SHA-1 hash are sent.
 * @returns {{ pwned: boolean, count: number }}  — fails open on network error.
 */
export async function checkHIBP(password) {
  try {
    const hash   = await sha1Hex(password);
    const prefix = hash.slice(0, 5);
    const suffix = hash.slice(5);
    const res = await fetch(`https://api.pwnedpasswords.com/range/${prefix}`, {
      headers: { 'Add-Padding': 'true' },
    });
    if (!res.ok) return { pwned: false, count: 0 }; // fail open
    const text = await res.text();
    for (const line of text.split('\n')) {
      const [h, c] = line.split(':');
      if (h && h.trim().toUpperCase() === suffix) {
        return { pwned: true, count: parseInt(c.trim(), 10) || 1 };
      }
    }
    return { pwned: false, count: 0 };
  } catch {
    return { pwned: false, count: 0 }; // fail open — don't block on network issues
  }
}

// ── Sync validation (length + blocklist) ──────────────────────────────────
/**
 * Synchronous check — run first before the async HIBP call.
 * @returns {{ valid: boolean, error: string }}
 */
export function validatePassword(pwd) {
  if (!pwd || pwd.length < 8) {
    return { valid: false, error: 'Password must be at least 8 characters.' };
  }
  if (BLOCKED.has(pwd.toLowerCase())) {
    return { valid: false, error: 'This password is too common. Please choose a more unique password.' };
  }
  return { valid: true, error: '' };
}

/**
 * Full async validation: sync checks + HIBP breach lookup.
 * @returns {Promise<{ valid: boolean, error: string }>}
 */
export async function validatePasswordFull(pwd) {
  const sync = validatePassword(pwd);
  if (!sync.valid) return sync;

  const hibp = await checkHIBP(pwd);
  if (hibp.pwned) {
    return {
      valid: false,
      error: `This password has appeared in ${hibp.count.toLocaleString()} known data breaches. Please choose a different password.`,
    };
  }
  return { valid: true, error: '' };
}

// ── Strength meter (length-based, no complexity nonsense) ─────────────────
export function passwordStrength(pwd) {
  if (!pwd) return { score: 0, label: '', color: '' };
  const len = pwd.length;
  if (len < 8)  return { score: 1, label: 'Too short', color: '#ef4444' };
  if (len < 10) return { score: 2, label: 'Weak',      color: '#f97316' };
  if (len < 14) return { score: 3, label: 'Good',      color: '#eab308' };
  return           { score: 4, label: 'Strong',    color: '#22c55e' };
}
