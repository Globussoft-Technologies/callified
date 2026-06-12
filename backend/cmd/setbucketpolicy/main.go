package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func main() {
	bucket := os.Getenv("S3_BUCKET")
	region := os.Getenv("S3_REGION")
	accessKey := os.Getenv("AWS_ACCESS_KEY_ID")
	secretKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	if bucket == "" || accessKey == "" || secretKey == "" {
		log.Fatal("S3_BUCKET, AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY must be set in environment")
	}
	if region == "" {
		region = "ap-south-1"
	}

	cfg := aws.Config{
		Region:      region,
		Credentials: credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""),
	}
	client := s3.NewFromConfig(cfg)
	ctx := context.Background()

	// 1. Disable block public access
	_, err := client.PutPublicAccessBlock(ctx, &s3.PutPublicAccessBlockInput{
		Bucket: aws.String(bucket),
		PublicAccessBlockConfiguration: &types.PublicAccessBlockConfiguration{
			BlockPublicAcls:       aws.Bool(false),
			IgnorePublicAcls:      aws.Bool(false),
			BlockPublicPolicy:     aws.Bool(false),
			RestrictPublicBuckets: aws.Bool(false),
		},
	})
	if err != nil {
		log.Fatalf("PutPublicAccessBlock: %v", err)
	}
	fmt.Println("✓ Block public access disabled")

	// 2. Apply bucket policy for public read on recordings/*
	policy := fmt.Sprintf(`{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "PublicReadRecordings",
      "Effect": "Allow",
      "Principal": "*",
      "Action": "s3:GetObject",
      "Resource": "arn:aws:s3:::%s/recordings/*"
    }
  ]
}`, bucket)
	_, err = client.PutBucketPolicy(ctx, &s3.PutBucketPolicyInput{
		Bucket: aws.String(bucket),
		Policy: aws.String(policy),
	})
	if err != nil {
		log.Fatalf("PutBucketPolicy: %v", err)
	}
	fmt.Println("✓ Bucket policy applied — recordings/* is now public")
}
