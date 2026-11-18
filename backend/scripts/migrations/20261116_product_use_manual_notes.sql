-- Existing and new products use manual notes until explicitly disabled.
-- Startup also ensures this column; apply manually only if it is absent.
ALTER TABLE products ADD COLUMN use_manual_notes BOOLEAN NOT NULL DEFAULT TRUE;
