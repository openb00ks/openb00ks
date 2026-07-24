-- A compact, display-oriented summary of what the AI pipeline decided for a receipt (the matched/proposed
-- vendor and the classified account, each with confidence + reason). Persisted so the review UI can
-- explain a suggestion to the reviewer before they approve — turning a black-box draft into a reviewable
-- one. JSONB (shape = models.ReceiptAISummary); null for receipts processed before this column or by the
-- legacy suggest path.
ALTER TABLE receipts ADD COLUMN ai_summary JSONB;
