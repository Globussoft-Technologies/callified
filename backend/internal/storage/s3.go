package storage

import (
	"bytes"
	"context"
	"fmt"
	"mime"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Client wraps the AWS S3 client and bucket config.
type S3Client struct {
	client *s3.Client
	bucket string
	region string
}

// NewS3Client creates an S3Client using static credentials.
func NewS3Client(region, bucket, accessKeyID, secretAccessKey string) *S3Client {
	cfg := aws.Config{
		Region: region,
		Credentials: credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, ""),
	}
	return &S3Client{
		client: s3.NewFromConfig(cfg),
		bucket: bucket,
		region: region,
	}
}

// UploadPublic uploads data to S3 with public-read ACL and returns the public URL.
// key is the S3 object key, e.g. "recordings/recording_abc123.mp3".
func (c *S3Client) UploadPublic(ctx context.Context, key string, data []byte) (string, error) {
	ct := mime.TypeByExtension(filepath.Ext(key))
	if ct == "" {
		ct = "application/octet-stream"
	}
	_, err := c.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(c.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String(ct),
	})
	if err != nil {
		return "", fmt.Errorf("s3 upload %s: %w", key, err)
	}
	publicURL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", c.bucket, c.region, key)
	return publicURL, nil
}
