package minio

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/thanhtai9606/DocForge/apps/api/internal/config"
	"github.com/thanhtai9606/DocForge/apps/api/internal/domain"
	"github.com/thanhtai9606/DocForge/apps/api/internal/storage"
)

// Store is a MinIO-backed ObjectStore (MinIO speaks the S3 API).
type Store struct {
	client *s3.Client
	bucket string
}

var _ storage.ObjectStore = (*Store)(nil)

func New(cfg config.Config) *Store {
	resolver := s3.EndpointResolverFromURL(cfg.MinIOEndpoint)
	client := s3.New(s3.Options{
		Region:           cfg.MinIORegion,
		Credentials:      credentials.NewStaticCredentialsProvider(cfg.MinIOAccessKey, cfg.MinIOSecretKey, ""),
		EndpointResolver: resolver,
		UsePathStyle:     cfg.MinIOUsePathStyle,
	})
	return &Store{client: client, bucket: cfg.MinIOBucket}
}

func (s *Store) EnsureBucket(ctx context.Context) error {
	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(s.bucket)})
	if err == nil {
		return nil
	}
	_, err = s.client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(s.bucket)})
	return err
}

func (s *Store) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	input := &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        r,
		ContentType: aws.String(contentType),
	}
	if size >= 0 {
		input.ContentLength = aws.Int64(size)
	}
	_, err := s.client.PutObject(ctx, input)
	if err != nil {
		return domain.NewAppError(domain.CodeStorageError, fmt.Sprintf("minio put object: %v", err), true)
	}
	return nil
}

func (s *Store) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "nosuchkey") || strings.Contains(strings.ToLower(err.Error()), "not found") {
			return nil, domain.NewAppError(domain.CodeNotFound, "object not found", false)
		}
		return nil, domain.NewAppError(domain.CodeStorageError, fmt.Sprintf("minio get object: %v", err), true)
	}
	return out.Body, nil
}

func (s *Store) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return domain.NewAppError(domain.CodeStorageError, fmt.Sprintf("minio delete object: %v", err), true)
	}
	return nil
}
