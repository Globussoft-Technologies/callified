-- Run once before enabling authenticated dynamic Tata streaming.
ALTER TABLE org_exotel_accounts ADD COLUMN tata_stream_secret_hash CHAR(64) NULL UNIQUE;
