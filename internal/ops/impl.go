package ops

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// PgDumpDumper produces a plain-SQL dump via the pg_dump binary (bundled in the ops-scheduler image).
// --no-owner/--no-privileges keep the dump portable across role setups on restore.
type PgDumpDumper struct {
	DSN  string
	Path string // pg_dump path; empty = "pg_dump" on PATH
}

func (d PgDumpDumper) Dump(ctx context.Context, w io.Writer) error {
	bin := d.Path
	if bin == "" {
		bin = "pg_dump"
	}
	cmd := exec.CommandContext(ctx, bin, "--no-owner", "--no-privileges", d.DSN)
	cmd.Stdout = w
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pg_dump: %w: %s", err, stderr.String())
	}
	return nil
}

// S3Sink is an R2/S3 Sink for backups. Separate from internal/storage (that one is receipt-oriented:
// date-keyed writes + presigned reads); backups need arbitrary keys + list/delete for retention.
type S3Sink struct {
	client *s3.Client
	bucket string
}

// S3SinkConfig configures the backups object store.
type S3SinkConfig struct {
	Bucket          string
	Endpoint        string
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	ForcePathStyle  bool
}

func NewS3Sink(cfg S3SinkConfig) (*S3Sink, error) {
	if cfg.Bucket == "" || cfg.Endpoint == "" || cfg.AccessKeyID == "" || cfg.SecretAccessKey == "" {
		return nil, fmt.Errorf("s3 backup sink: bucket, endpoint, and access keys are required")
	}
	region := cfg.Region
	if region == "" {
		region = "auto"
	}
	client := s3.New(s3.Options{
		Region:       region,
		BaseEndpoint: aws.String(cfg.Endpoint),
		UsePathStyle: cfg.ForcePathStyle,
		Credentials:  credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
	})
	return &S3Sink{client: client, bucket: cfg.Bucket}, nil
}

func (s *S3Sink) Put(ctx context.Context, key string, r io.ReadSeeker) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Body:   r,
	})
	return err
}

func (s *S3Sink) List(ctx context.Context, prefix string) ([]string, error) {
	var keys []string
	p := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(prefix),
	})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, obj := range page.Contents {
			if obj.Key != nil {
				keys = append(keys, *obj.Key)
			}
		}
	}
	return keys, nil
}

func (s *S3Sink) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	return err
}
