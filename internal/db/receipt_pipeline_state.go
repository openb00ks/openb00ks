package db

import "context"

// ReceiptPipelineState is a receipt's position in the batched pipeline, with the intermediate stage
// outputs it accumulates.
type ReceiptPipelineState struct {
	ReceiptID    string
	Stage        string
	Status       string
	ExtractJSON  []byte
	VendorJSON   []byte
	ClassifyJSON []byte
}

// ReceiptPipelineStateStore backs receipt_pipeline_state.
type ReceiptPipelineStateStore struct {
	db *DB
}

func NewReceiptPipelineStateStore(db *DB) *ReceiptPipelineStateStore {
	return &ReceiptPipelineStateStore{db: db}
}

// Ensure starts a receipt at stage 'extract' (idempotent) — called when a receipt enters the batched
// pipeline.
func (s *ReceiptPipelineStateStore) Ensure(ctx context.Context, receiptID string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO receipt_pipeline_state (receipt_id) VALUES ($1)
		ON CONFLICT (receipt_id) DO NOTHING`, receiptID)
	return err
}

type receiptPipelineStateRow struct {
	ReceiptID    string `db:"receipt_id"`
	Stage        string `db:"stage"`
	Status       string `db:"status"`
	ExtractJSON  []byte `db:"extract_json"`
	VendorJSON   []byte `db:"vendor_json"`
	ClassifyJSON []byte `db:"classify_json"`
}

// ClaimPending leases up to limit receipts pending at the given stage (multi-worker safe via FOR
// UPDATE SKIP LOCKED), marking them in_flight, and returns them with their accumulated outputs.
func (s *ReceiptPipelineStateStore) ClaimPending(ctx context.Context, stage string, limit int) ([]ReceiptPipelineState, error) {
	rows := []receiptPipelineStateRow{}
	err := s.db.SelectContext(ctx, &rows, `
		UPDATE receipt_pipeline_state
		SET status = 'in_flight', running_since = now(), updated_at = now()
		WHERE receipt_id IN (
			SELECT receipt_id FROM receipt_pipeline_state
			WHERE stage = $1 AND status = 'pending'
			ORDER BY updated_at
			FOR UPDATE SKIP LOCKED
			LIMIT $2
		)
		RETURNING receipt_id::text AS receipt_id, stage, status, extract_json, vendor_json, classify_json
	`, stage, limit)
	if err != nil {
		return nil, err
	}
	return toStates(rows), nil
}

// Get returns one receipt's pipeline state.
func (s *ReceiptPipelineStateStore) Get(ctx context.Context, receiptID string) (ReceiptPipelineState, error) {
	var r receiptPipelineStateRow
	err := s.db.GetContext(ctx, &r, `
		SELECT receipt_id::text AS receipt_id, stage, status, extract_json, vendor_json, classify_json
		FROM receipt_pipeline_state WHERE receipt_id = $1`, receiptID)
	if err != nil {
		return ReceiptPipelineState{}, err
	}
	return toStates([]receiptPipelineStateRow{r})[0], nil
}

// Reset returns leased receipts to pending at their current stage (a batch failed/expired).
func (s *ReceiptPipelineStateStore) Reset(ctx context.Context, receiptIDs []string) error {
	for _, id := range receiptIDs {
		if _, err := s.db.ExecContext(ctx, `
			UPDATE receipt_pipeline_state SET status = 'pending', running_since = NULL, updated_at = now()
			WHERE receipt_id = $1 AND status = 'in_flight'`, id); err != nil {
			return err
		}
	}
	return nil
}

// SaveExtractAndAdvance stores the extract output and moves to the next stage (pending).
func (s *ReceiptPipelineStateStore) SaveExtractAndAdvance(ctx context.Context, receiptID string, extractJSON []byte, next string) error {
	return s.advance(ctx, receiptID, "extract_json", extractJSON, next)
}

// SaveVendorAndAdvance stores the vendor-match output and moves to the next stage (pending).
func (s *ReceiptPipelineStateStore) SaveVendorAndAdvance(ctx context.Context, receiptID string, vendorJSON []byte, next string) error {
	return s.advance(ctx, receiptID, "vendor_json", vendorJSON, next)
}

// SaveClassifyAndFinish stores the classify output and moves to 'done' (build-entry runs inline).
func (s *ReceiptPipelineStateStore) SaveClassifyAndFinish(ctx context.Context, receiptID string, classifyJSON []byte) error {
	return s.advance(ctx, receiptID, "classify_json", classifyJSON, "done")
}

// advance is shared by the SaveXAndAdvance methods; the column is a fixed literal from those callers
// (never user input).
func (s *ReceiptPipelineStateStore) advance(ctx context.Context, receiptID, col string, jsonVal []byte, next string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE receipt_pipeline_state
		SET `+col+` = $2, stage = $3, status = 'pending', running_since = NULL, updated_at = now()
		WHERE receipt_id = $1`, receiptID, jsonVal, next)
	return err
}

// Park moves a receipt to 'review' with the failing stage + issues (never auto-posted).
func (s *ReceiptPipelineStateStore) Park(ctx context.Context, receiptID, failedStage, issues string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE receipt_pipeline_state
		SET stage = 'review', status = 'pending', failed_stage = $2, issues = NULLIF($3, ''),
		    running_since = NULL, updated_at = now()
		WHERE receipt_id = $1`, receiptID, failedStage, issues)
	return err
}

func toStates(rows []receiptPipelineStateRow) []ReceiptPipelineState {
	out := make([]ReceiptPipelineState, 0, len(rows))
	for _, r := range rows {
		out = append(out, ReceiptPipelineState(r))
	}
	return out
}
