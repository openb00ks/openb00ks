package queue

import "context"

type JobStatus string

const (
	JobQueued     JobStatus = "queued"
	JobProcessing JobStatus = "processing"
	JobDone       JobStatus = "done"
	JobFailed     JobStatus = "failed"
	JobDead       JobStatus = "dead"
)

type JobStage string

const (
	StageOCR     JobStage = "ocr"
	StageSuggest JobStage = "suggest"
	StageDraft   JobStage = "draft"
)

type Job struct {
	ID          string
	ReceiptID   string
	Stage       JobStage
	Status      JobStatus
	Attempts    int
	MaxAttempts int
	LockedBy    string
	LastError   string
}

type EnqueueRequest struct {
	ReceiptID string
	Stage     JobStage
}

type ClaimRequest struct {
	WorkerID    string
	MaxAttempts int
	LockSeconds int
	BatchSize   int
	Stage       JobStage
}

type Queue interface {
	Enqueue(ctx context.Context, req EnqueueRequest) (Job, error)
	Claim(ctx context.Context, req ClaimRequest) ([]Job, error)
	Ack(ctx context.Context, jobID string) error
	Fail(ctx context.Context, jobID string, errMsg string, retry bool) error
	Requeue(ctx context.Context, jobID string) error
}
