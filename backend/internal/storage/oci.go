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

// OCIClient wraps an S3-compatible client for Oracle Cloud Infrastructure
// Object Storage. OCI's Amazon S3 Compatibility API lets us reuse the AWS
// SDK v2 S3 client with a custom endpoint and path-style URLs.
type OCIClient struct {
	client    *s3.Client
	bucket    string
	namespace string
	region    string
}

// NewOCIClient creates an OCIClient using the S3 Compatibility API.
// The endpoint is constructed as:
//
//	https://<namespace>.compat.objectstorage.<region>.oraclecloud.com
func NewOCIClient(region, namespace, bucket, accessKeyID, secretAccessKey string) *OCIClient {
	endpoint := fmt.Sprintf("https://%s.compat.objectstorage.%s.oraclecloud.com", namespace, region)
	cfg := aws.Config{
		Region:      region,
		Credentials: credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, ""),
	}
	return &OCIClient{
		client: s3.NewFromConfig(cfg, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(endpoint)
			o.UsePathStyle = true
		}),
		bucket:    bucket,
		namespace: namespace,
		region:    region,
	}
}

// UploadPublic uploads data to OCI Object Storage and returns the public
// native OCI URL for the object. The caller is responsible for ensuring the
// bucket/object is publicly readable (e.g., via a bucket policy).
// key is the object key, e.g. "recordings/recording_abc123.wav".
func (c *OCIClient) UploadPublic(ctx context.Context, key string, data []byte) (string, error) {
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
		return "", fmt.Errorf("oci upload %s: %w", key, err)
	}
	// Native OCI Object Storage URL format for public objects.
	publicURL := fmt.Sprintf("https://objectstorage.%s.oraclecloud.com/n/%s/b/%s/o/%s", c.region, c.namespace, c.bucket, key)
	return publicURL, nil
}
