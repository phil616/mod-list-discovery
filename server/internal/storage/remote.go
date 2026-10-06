package storage

import (
	"context"
	"errors"
	"fmt"
	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"io"
	"mod-list-discovery/server/internal/config"
	"net/http"
	"os"
	"strings"
	"time"
)

type S3 struct {
	client         *s3.Client
	bucket, prefix string
}

func NewS3(ctx context.Context, c config.Remote) (*S3, error) {
	if c.Bucket == "" {
		return nil, fmt.Errorf("S3 bucket required")
	}
	cfg, e := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(c.Region), awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(os.Getenv(c.AccessKeyEnv), os.Getenv(c.SecretKeyEnv), "")))
	if e != nil {
		return nil, e
	}
	cl := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = c.ForcePathStyle
		if c.Endpoint != "" {
			o.BaseEndpoint = aws.String(c.Endpoint)
		}
	})
	return &S3{cl, c.Bucket, c.Prefix}, nil
}
func (s *S3) key(k string) (*string, error) {
	if !validKey(k) {
		return nil, fmt.Errorf("invalid object key")
	}
	return aws.String(s.prefix + k), nil
}
func (s *S3) Put(ctx context.Context, k string, r io.Reader, n int64) error {
	key, e := s.key(k)
	if e != nil {
		return e
	}
	_, e = s.client.PutObject(ctx, &s3.PutObjectInput{Bucket: &s.bucket, Key: key, Body: r, ContentLength: &n, ContentType: aws.String("application/java-archive"), IfNoneMatch: aws.String("*")})
	return e
}
func (s *S3) Open(context.Context, string) (ReadSeekCloser, error) {
	return nil, fmt.Errorf("use signed download URL")
}
func (s *S3) Exists(ctx context.Context, k string) (bool, error) {
	key, e := s.key(k)
	if e != nil {
		return false, e
	}
	_, e = s.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: &s.bucket, Key: key})
	var api smithy.APIError
	if errors.As(e, &api) && (api.ErrorCode() == "NotFound" || api.ErrorCode() == "NoSuchKey") {
		return false, nil
	}
	return e == nil, e
}
func (s *S3) Delete(ctx context.Context, k string) error {
	key, e := s.key(k)
	if e != nil {
		return e
	}
	_, e = s.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: &s.bucket, Key: key})
	return e
}
func (s *S3) SignedDownloadURL(ctx context.Context, k string, ttl time.Duration) (string, error) {
	key, e := s.key(k)
	if e != nil {
		return "", e
	}
	out, e := s3.NewPresignClient(s.client).PresignGetObject(ctx, &s3.GetObjectInput{Bucket: &s.bucket, Key: key}, s3.WithPresignExpires(ttl))
	if e != nil {
		return "", e
	}
	return out.URL, nil
}

type OSS struct {
	bucket *oss.Bucket
	prefix string
}

func NewOSS(c config.Remote) (*OSS, error) {
	if c.Bucket == "" || !strings.HasPrefix(c.Endpoint, "https://") {
		return nil, fmt.Errorf("OSS requires HTTPS endpoint and bucket")
	}
	cl, e := oss.New(c.Endpoint, os.Getenv(c.AccessKeyEnv), os.Getenv(c.SecretKeyEnv), oss.Timeout(10, 300))
	if e != nil {
		return nil, e
	}
	b, e := cl.Bucket(c.Bucket)
	return &OSS{b, c.Prefix}, e
}
func (s *OSS) key(k string) (string, error) {
	if !validKey(k) {
		return "", fmt.Errorf("invalid object key")
	}
	return s.prefix + k, nil
}
func (s *OSS) Put(ctx context.Context, k string, r io.Reader, n int64) error {
	key, e := s.key(k)
	if e != nil {
		return e
	}
	return s.bucket.PutObject(key, r, oss.WithContext(ctx), oss.ContentLength(n), oss.ForbidOverWrite(true))
}
func (s *OSS) Open(context.Context, string) (ReadSeekCloser, error) {
	return nil, fmt.Errorf("use signed download URL")
}
func (s *OSS) Exists(ctx context.Context, k string) (bool, error) {
	key, e := s.key(k)
	if e != nil {
		return false, e
	}
	_, e = s.bucket.GetObjectMeta(key, oss.WithContext(ctx))
	var se oss.ServiceError
	if errors.As(e, &se) && se.StatusCode == http.StatusNotFound {
		return false, nil
	}
	return e == nil, e
}
func (s *OSS) Delete(ctx context.Context, k string) error {
	key, e := s.key(k)
	if e != nil {
		return e
	}
	return s.bucket.DeleteObject(key, oss.WithContext(ctx))
}
func (s *OSS) SignedDownloadURL(ctx context.Context, k string, ttl time.Duration) (string, error) {
	if e := ctx.Err(); e != nil {
		return "", e
	}
	key, e := s.key(k)
	if e != nil {
		return "", e
	}
	return s.bucket.SignURL(key, oss.HTTPGet, int64(ttl.Seconds()))
}
