ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS beyond_limits_enabled BOOLEAN NOT NULL DEFAULT false;

COMMENT ON COLUMN groups.beyond_limits_enabled IS
    'Whether the beyond-limits (breakthrough) pipeline is enabled for this group: bridge prompt injection + refusal tampering apply to LLM text requests routed through this group';
