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
	client    *s3.Client
	bucket    string
	region    string
	namespace string // Oracle Cloud namespace (empty for AWS)
	endpoint  string // custom endpoint (empty for AWS)
}

// NewS3Client creates an S3Client using static credentials (AWS).
func NewS3Client(region, bucket, accessKeyID, secretAccessKey string) *S3Client {
	return newClient(region, bucket, accessKeyID, secretAccessKey, "", "")
}

// NewOracleClient creates an S3Client pointed at Oracle Cloud Object Storage.
// namespace is the OCI tenancy object-storage namespace.
// endpoint is e.g. "https://objectstorage.ap-mumbai-1.oraclecloud.com"
func NewOracleClient(region, bucket, accessKeyID, secretAccessKey, namespace, endpoint string) *S3Client {
	return newClient(region, bucket, accessKeyID, secretAccessKey, namespace, endpoint)
}

func newClient(region, bucket, accessKeyID, secretAccessKey, namespace, endpoint string) *S3Client {
	cfg := aws.Config{
		Region:      region,
		Credentials: credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, ""),
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		if endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
			o.UsePathStyle = true // Oracle requires path-style URLs
		}
	})
	return &S3Client{
		client:    client,
		bucket:    bucket,
		region:    region,
		namespace: namespace,
		endpoint:  endpoint,
	}
}

// UploadPublic uploads data and returns the public URL.
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

	var publicURL string
	if c.namespace != "" {
		// Oracle Cloud Object Storage public URL format
		publicURL = fmt.Sprintf("https://objectstorage.%s.oraclecloud.com/n/%s/b/%s/o/%s",
			c.region, c.namespace, c.bucket, key)
	} else {
		// AWS S3 public URL format
		publicURL = fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", c.bucket, c.region, key)
	}
	return publicURL, nil
}
