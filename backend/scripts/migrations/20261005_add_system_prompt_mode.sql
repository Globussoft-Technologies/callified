ALTER TABLE organizations
    ADD COLUMN system_prompt_mode VARCHAR(16) NOT NULL DEFAULT 'replace'
    AFTER custom_system_prompt;

-- Existing organizations retain replacement behavior through the default.
-- Application-created organizations explicitly opt into extend mode.
