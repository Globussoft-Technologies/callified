# Tata Smartflo Credential Rollout

Outbound Tata calls require both the Click-to-Call Support API key and the
Smartflo account token. Existing account records may contain an empty token or
a copy of the former single credential; a database migration cannot recover a
separate token. Do not copy one credential to the other or print secrets.

Before deploying the backend:

1. Arrange a maintenance window and verify that no calls are in progress.
2. Back up the database and apply `backend/scripts/migrations/20261028_expand_provider_api_tokens.sql` to the target database. The table creation code alone does not widen an existing column.
3. Inventory existing outbound Tata accounts that need review without exposing credentials:

   ```sql
   SELECT id, org_id, user_id, provider, name,
          CASE WHEN api_token IS NULL OR TRIM(api_token) = '' THEN 'missing token'
               WHEN BINARY api_key = BINARY api_token THEN 'matching credentials: verify'
               ELSE 'review with account owner' END AS review_reason
   FROM org_exotel_accounts
   WHERE provider IN ('tata', 'smartflo', 'tata_tele')
     AND COALESCE(direction, 'outbound') <> 'inbound';
   ```

4. Obtain the distinct account token and support key securely for every active
   account, including personal accounts. Configure them during the maintenance
   window in the provider account UI after deploying the updated frontend, or
   through an authorized secure process. Matching values indicate a possible
   legacy copy, not proof that credentials are invalid; verify with Tata.
5. If environment fallback is used, configure both `TATA_API_KEY` and
   `TATA_API_TOKEN` without logging their values. Deploy the backend and
   frontend, then run a controlled outbound call and verify answer, audio,
   hangup and outcome. Repeat for relevant accounts.

Outbound calls will fail until each account has the required credentials.
Keep the previous application artifact for rollback, but retain the widened
columns: narrowing them can truncate credentials. Never publish tokens in
deployment logs or tickets.
