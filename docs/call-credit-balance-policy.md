# Call Credit Balance Policy

- New billed calls require a positive organization balance. Zero or negative balances are blocked. Credit lookup failures must not allow outgoing calls.
- Calls already started are charged their full duration at the existing organization rate, rounded up to the nearest paise. Charges are not capped at the remaining balance.
- A negative balance represents outstanding usage. Recharges add to that balance; calling resumes only after the balance becomes positive.
- Deductions lock the organization's credit row before checking the organization-scoped call reference. Duplicate callbacks are no-ops, while different completed calls are fully charged even if another call has exhausted the balance.
- The balance update and ledger entry commit together. Absolute administrative balance adjustments also lock the credit row to avoid races with deductions.
- The API retains the negative currency balance but reports zero available minutes while in debt.

Example: 100 paise available minus a 500-paise completed call leaves -400 paise. A 300-paise recharge leaves -100 paise, so new calls remain blocked.

This policy intentionally permits outstanding usage from calls admitted before earlier calls were billed. It does not reserve credit or terminate connected calls when the balance reaches zero. Existing maximum call-duration limits and intentionally unbilled simulation paths are unchanged. Provider rates and the application rate are separate; this change does not automatically double the rate for forwarded calls.

## Database Compatibility

The repository schema already uses signed `BIGINT` columns for `org_credits.balance_paise`, `credit_transactions.delta_paise`, and `credit_transactions.balance_after_paise`. Deployments with schema drift must verify these are signed before rollout. No live database or provider configuration is modified by this code change.

## Tests

Run the unit suite with `cd backend && go test ./... -timeout 90s`.

The opt-in MySQL test exercises concurrent duplicate and distinct deductions, tenant-scoped references, and partial/full recharges. Set `CALLIFIED_TEST_MYSQL_DSN` to a disposable MySQL server and run `go test ./internal/db -run TestCallCreditsConcurrentMySQL -count=1`. The test account needs create/drop database privileges; the test creates and removes its own isolated schema.
