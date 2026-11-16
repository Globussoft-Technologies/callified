-- Reset readiness on rotation; verify the current secret before outbound dialing.
ALTER TABLE org_exotel_accounts ADD COLUMN tata_stream_verified_hash CHAR(64) NULL;
