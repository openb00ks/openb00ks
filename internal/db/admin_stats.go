package db

import "context"

type AdminStats struct {
	JobsByStatus     map[string]int `json:"jobs_by_status"`
	JobsByStage      map[string]int `json:"jobs_by_stage"`
	UnresolvedErrors int            `json:"unresolved_errors"`
	ReceiptsByStatus map[string]int `json:"receipts_by_status"`
}

type AdminStatsStore struct {
	db *DB
}

func NewAdminStatsStore(db *DB) *AdminStatsStore {
	return &AdminStatsStore{db: db}
}

func (s *AdminStatsStore) Query(ctx context.Context) (AdminStats, error) {
	stats := AdminStats{
		JobsByStatus:     make(map[string]int),
		JobsByStage:      make(map[string]int),
		ReceiptsByStatus: make(map[string]int),
	}

	type countRow struct {
		Key   string `db:"key"`
		Count int    `db:"count"`
	}

	var jobStatusRows []countRow
	if err := s.db.SelectContext(ctx, &jobStatusRows, `
		SELECT status AS key, COUNT(*)::int AS count
		FROM receipt_jobs
		GROUP BY status
	`); err != nil {
		return AdminStats{}, err
	}
	for _, r := range jobStatusRows {
		stats.JobsByStatus[r.Key] = r.Count
	}

	var jobStageRows []countRow
	if err := s.db.SelectContext(ctx, &jobStageRows, `
		SELECT stage AS key, COUNT(*)::int AS count
		FROM receipt_jobs
		WHERE status NOT IN ('done')
		GROUP BY stage
	`); err != nil {
		return AdminStats{}, err
	}
	for _, r := range jobStageRows {
		stats.JobsByStage[r.Key] = r.Count
	}

	if err := s.db.GetContext(ctx, &stats.UnresolvedErrors, `
		SELECT COUNT(*)::int FROM processing_errors WHERE resolved_at IS NULL
	`); err != nil {
		return AdminStats{}, err
	}

	var receiptRows []countRow
	if err := s.db.SelectContext(ctx, &receiptRows, `
		SELECT status AS key, COUNT(*)::int AS count
		FROM receipts
		GROUP BY status
	`); err != nil {
		return AdminStats{}, err
	}
	for _, r := range receiptRows {
		stats.ReceiptsByStatus[r.Key] = r.Count
	}

	return stats, nil
}
