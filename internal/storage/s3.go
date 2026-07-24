package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Config configures an S3-compatible receipt store. It targets any S3 API — the app is deployed
// against Cloudflare R2 (Region "auto", Endpoint = the account's r2.cloudflarestorage.com URL).
type S3Config struct {
	Bucket          string
	Endpoint        string
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	ForcePathStyle  bool
	PresignTTL      time.Duration
}

// S3Store persists receipts as objects in an S3-compatible bucket and hands the browser a short-lived
// presigned URL to read them back (the bucket is private — no public object access).
type S3Store struct {
	client     *s3.Client
	presign    *s3.PresignClient
	bucket     string
	presignTTL time.Duration
}

// NewS3Store validates the config and builds an S3 client with static credentials. It does not make a
// network call — use Ready to verify connectivity at startup.
func NewS3Store(cfg S3Config) (*S3Store, error) {
	switch {
	case cfg.Bucket == "":
		return nil, errors.New("s3 storage: RECEIPT_S3_BUCKET is required")
	case cfg.Endpoint == "":
		return nil, errors.New("s3 storage: RECEIPT_S3_ENDPOINT is required")
	case cfg.AccessKeyID == "" || cfg.SecretAccessKey == "":
		return nil, errors.New("s3 storage: RECEIPT_S3_ACCESS_KEY_ID and RECEIPT_S3_SECRET_ACCESS_KEY are required")
	}
	region := cfg.Region
	if region == "" {
		region = "auto"
	}
	ttl := cfg.PresignTTL
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}

	client := s3.New(s3.Options{
		Region:       region,
		BaseEndpoint: aws.String(cfg.Endpoint),
		UsePathStyle: cfg.ForcePathStyle,
		Credentials:  credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
	})
	return &S3Store{
		client:     client,
		presign:    s3.NewPresignClient(client),
		bucket:     cfg.Bucket,
		presignTTL: ttl,
	}, nil
}

// Put uploads the receipt under a date-prefixed key (mirrors the local store's layout) and returns
// that key. S3 keys always use forward slashes.
func (s *S3Store) Put(ctx context.Context, name string, contentType string, size int64, r Reader) (string, error) {
	_ = size
	// SigV4 hashes the payload, which requires a seekable body. Receipts are small (handler-capped at
	// RECEIPT_MAX_BYTES, ~10MB), so buffer into a bytes.Reader — this also makes any source Reader work.
	buf, err := io.ReadAll(readerAdapter{r: r})
	if err != nil {
		return "", err
	}
	key := path.Join(time.Now().UTC().Format("20060102"), name)
	in := &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(key),
		Body:          bytes.NewReader(buf),
		ContentLength: aws.Int64(int64(len(buf))),
	}
	if contentType != "" {
		in.ContentType = aws.String(contentType)
	}
	if _, err := s.client.PutObject(ctx, in); err != nil {
		return "", err
	}
	return key, nil
}

// GetURL returns a short-lived presigned GET URL for the object (the bucket is private).
func (s *S3Store) GetURL(ctx context.Context, key string) (string, error) {
	req, err := s.presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(s.presignTTL))
	if err != nil {
		return "", err
	}
	return req.URL, nil
}

// Ready confirms the bucket is reachable with the configured credentials (for /readyz).
func (s *S3Store) Ready(ctx context.Context) error {
	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(s.bucket)})
	return err
}
