package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Addr                     string
	MetricsAddr              string
	DatabaseURL              string
	JWTSecret                string
	JWTTTLSeconds            int64
	RefreshTTLSeconds        int64
	EnablePublicRegistration bool
	CORSAllowedOrigins       []string
	LogLevel                 string
	LogFormat                string
	ReceiptStorage           string
	ReceiptLocalDir          string
	ReceiptMaxBytes          int64
	ReceiptS3Bucket          string
	ReceiptS3Endpoint        string
	ReceiptS3Region          string
	ReceiptS3AccessKeyID     string
	ReceiptS3SecretAccessKey string
	ReceiptS3PresignTTLSecs  int64
	ReceiptS3ForcePathStyle  bool
	// Ops scheduler (cmd/ops-scheduler)
	SchedulerTickSeconds  int64
	SchedulerLeaseSeconds int64
	// db-backup task → R2/S3 (its own bucket-scoped creds; empty BACKUP_S3_BUCKET disables the task)
	BackupS3Bucket          string
	BackupS3Endpoint        string
	BackupS3Region          string
	BackupS3AccessKeyID     string
	BackupS3SecretAccessKey string
	BackupS3ForcePathStyle  bool
	BackupS3Prefix          string
	BackupDBLabel           string
	BackupRetention         int64
	BackupIntervalSeconds   int64
	AIProvider              string
	OpenAIAPIKey            string
	OpenAIModel             string
	// OCR / receipt transcription (Phase 3 pipeline stage 1). Provider: none | llm-vision.
	// llm-vision sends the receipt image (via a presigned object-storage URL) to a vision model
	// through the shared AI driver; it requires RECEIPT_STORAGE=s3 (a fetchable URL).
	OCRProvider  string
	OCRModel     string
	OCRMaxTokens int64
	PipelineMode string
	// AI batch (PIPELINE_MODE=decomposed-batch): the ops-scheduler submits/collects provider batches
	// for the receipt stages; the worker hands receipts off to receipt_pipeline_state instead of
	// running the pipeline synchronously. Batch uses the system OpenAI credentials.
	AIBatchWindow             string
	AIBatchSubmitSeconds      int64
	AIBatchCollectSeconds     int64
	AIBatchStaleHours         int64
	AIInputCentsPer1KTokens   int64
	AIOutputCentsPer1KTokens  int64
	SearchProvider            string
	TypesenseURL              string
	TypesenseAPIKey           string
	TypesenseCollectionPrefix string
	// SearchReconcileSeconds is the interval for the ops-scheduler's full search reindex (drift healer
	// for docs that missed their index-on-write). 0 disables the task.
	SearchReconcileSeconds int64
}

func Load() Config {
	return Config{
		Addr:                     envOr("API_ADDR", ":8080"),
		MetricsAddr:              envOr("METRICS_ADDR", ":9090"),
		DatabaseURL:              envOr("DATABASE_URL", ""),
		JWTSecret:                envOr("JWT_SECRET", ""),
		JWTTTLSeconds:            envOrInt64("JWT_TTL_SECONDS", 86400),
		RefreshTTLSeconds:        envOrInt64("REFRESH_TTL_SECONDS", 2592000),
		EnablePublicRegistration: envOrBool("ENABLE_PUBLIC_REGISTRATION", false),
		CORSAllowedOrigins:       envOrCSV("CORS_ALLOWED_ORIGINS", "http://localhost:5173"),
		LogLevel:                 envOr("LOG_LEVEL", "info"),
		LogFormat:                envOr("LOG_FORMAT", "json"),
		ReceiptStorage:           envOr("RECEIPT_STORAGE", "local"),
		ReceiptLocalDir:          envOr("RECEIPT_LOCAL_DIR", "./.data/receipts"),
		ReceiptMaxBytes:          envOrInt64("RECEIPT_MAX_BYTES", 10*1024*1024),
		// S3-compatible receipt storage (RECEIPT_STORAGE=s3), e.g. Cloudflare R2. Region is "auto"
		// for R2; endpoint is the account's S3 API URL. Access keys come from the app Secret.
		ReceiptS3Bucket:           envOr("RECEIPT_S3_BUCKET", ""),
		ReceiptS3Endpoint:         envOr("RECEIPT_S3_ENDPOINT", ""),
		ReceiptS3Region:           envOr("RECEIPT_S3_REGION", "auto"),
		ReceiptS3AccessKeyID:      envOr("RECEIPT_S3_ACCESS_KEY_ID", ""),
		ReceiptS3SecretAccessKey:  envOr("RECEIPT_S3_SECRET_ACCESS_KEY", ""),
		ReceiptS3PresignTTLSecs:   envOrInt64("RECEIPT_S3_PRESIGN_TTL_SECONDS", 900),
		ReceiptS3ForcePathStyle:   envOrBool("RECEIPT_S3_FORCE_PATH_STYLE", true),
		SchedulerTickSeconds:      envOrInt64("SCHEDULER_TICK_SECONDS", 30),
		SchedulerLeaseSeconds:     envOrInt64("SCHEDULER_LEASE_SECONDS", 3600),
		BackupS3Bucket:            envOr("BACKUP_S3_BUCKET", ""),
		BackupS3Endpoint:          envOr("BACKUP_S3_ENDPOINT", ""),
		BackupS3Region:            envOr("BACKUP_S3_REGION", "auto"),
		BackupS3AccessKeyID:       envOr("BACKUP_S3_ACCESS_KEY_ID", ""),
		BackupS3SecretAccessKey:   envOr("BACKUP_S3_SECRET_ACCESS_KEY", ""),
		BackupS3ForcePathStyle:    envOrBool("BACKUP_S3_FORCE_PATH_STYLE", true),
		BackupS3Prefix:            envOr("BACKUP_S3_PREFIX", "backups"),
		BackupDBLabel:             envOr("BACKUP_DB_LABEL", "openbooks"),
		BackupRetention:           envOrInt64("BACKUP_RETENTION", 14),
		BackupIntervalSeconds:     envOrInt64("BACKUP_INTERVAL_SECONDS", 86400),
		AIProvider:                envOr("AI_PROVIDER", "none"),
		OpenAIAPIKey:              envOr("OPENAI_API_KEY", ""),
		OpenAIModel:               envOr("OPENAI_MODEL", ""),
		OCRProvider:               envOr("OCR_PROVIDER", "none"),
		OCRModel:                  envOr("OCR_MODEL", ""),
		OCRMaxTokens:              envOrInt64("OCR_MAX_TOKENS", 4096),
		PipelineMode:              envOr("PIPELINE_MODE", ""),
		AIBatchWindow:             envOr("AIBATCH_WINDOW", "24h"),
		AIBatchSubmitSeconds:      envOrInt64("AIBATCH_SUBMIT_SECONDS", 1800),
		AIBatchCollectSeconds:     envOrInt64("AIBATCH_COLLECT_SECONDS", 900),
		AIBatchStaleHours:         envOrInt64("AIBATCH_STALE_HOURS", 26),
		AIInputCentsPer1KTokens:   envOrInt64("AI_INPUT_CENTS_PER_1K_TOKENS", 0),
		AIOutputCentsPer1KTokens:  envOrInt64("AI_OUTPUT_CENTS_PER_1K_TOKENS", 0),
		SearchProvider:            envOr("SEARCH_PROVIDER", "none"),
		TypesenseURL:              envOr("TYPESENSE_URL", ""),
		TypesenseAPIKey:           envOr("TYPESENSE_API_KEY", ""),
		TypesenseCollectionPrefix: envOr("TYPESENSE_COLLECTION_PREFIX", "openb00ks"),
		SearchReconcileSeconds:    envOrInt64("SEARCH_RECONCILE_SECONDS", 21600),
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envOrInt64(key string, fallback int64) int64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return fallback
}

func envOrCSV(key, fallback string) []string {
	raw := strings.TrimSpace(envOr(key, fallback))
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		val := strings.TrimSpace(part)
		if val != "" {
			out = append(out, val)
		}
	}
	return out
}

func envOrBool(key string, fallback bool) bool {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		switch strings.ToLower(v) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		}
	}
	return fallback
}
