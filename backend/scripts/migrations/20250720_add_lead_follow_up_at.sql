-- Add follow_up_at to leads for post-call disposition gating.
ALTER TABLE leads ADD COLUMN follow_up_at DATETIME NULL AFTER follow_up_note;
