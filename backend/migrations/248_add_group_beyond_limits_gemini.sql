ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS beyond_limits_gemini_enabled BOOLEAN NOT NULL DEFAULT false;

COMMENT ON COLUMN groups.beyond_limits_gemini_enabled IS
    'Whether the beyond-limits (breakthrough) pipeline is enabled for gemini-* models routed through this group';
