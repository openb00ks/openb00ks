ALTER TABLE receipt_suggestions
ADD COLUMN prompt_tokens INT,
ADD COLUMN completion_tokens INT,
ADD COLUMN total_tokens INT,
ADD COLUMN cost_cents BIGINT;
