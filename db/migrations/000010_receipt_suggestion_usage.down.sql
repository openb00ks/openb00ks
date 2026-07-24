ALTER TABLE receipt_suggestions
DROP COLUMN IF EXISTS cost_cents,
DROP COLUMN IF EXISTS total_tokens,
DROP COLUMN IF EXISTS completion_tokens,
DROP COLUMN IF EXISTS prompt_tokens;
