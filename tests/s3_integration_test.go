package tests

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

const (
	zs3Endpoint = "http://localhost:9000"
	zs3Region   = "us-east-1"
	zs3Bucket   = "test-bucket"
	zs3AccessKey = "minioadmin"
	zs3SecretKey = "minioadmin"
)

type S3TestClient struct {
	client *s3.Client
	bucket string
}

func NewS3TestClient() (*S3TestClient, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(zs3Region),
		config.WithCredentialsProvider(aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
			return aws.Credentials{
				AccessKeyID:     zs3AccessKey,
				SecretAccessKey: zs3SecretKey,
			}, nil
		})),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(zs3Endpoint)
		o.UsePathStyle = true
	})

	return &S3TestClient{
		client: client,
		bucket: zs3Bucket,
	}, nil
}

// setupBucket 创建测试bucket
func (c *S3TestClient) setupBucket(ctx context.Context) error {
	// Check if bucket exists
	_, err := c.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(c.bucket),
	})
	if err == nil {
		// Bucket exists, empty it
		return c.emptyBucket(ctx)
	}

	// Create bucket
	_, err = c.client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(c.bucket),
	})
	if err != nil {
		return fmt.Errorf("failed to create bucket: %w", err)
	}

	return nil
}

// emptyBucket 清空bucket
func (c *S3TestClient) emptyBucket(ctx context.Context) error {
	list, err := c.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(c.bucket),
	})
	if err != nil {
		return err
	}

	for _, obj := range list.Contents {
		_, err := c.client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(c.bucket),
			Key:    obj.Key,
		})
		if err != nil {
			return err
		}
	}

	return nil
}

// cleanupBucket 删除测试bucket
func (c *S3TestClient) cleanupBucket(ctx context.Context) error {
	// Empty bucket first
	_ = c.emptyBucket(ctx)

	// Delete bucket
	_, err := c.client.DeleteBucket(ctx, &s3.DeleteBucketInput{
		Bucket: aws.String(c.bucket),
	})
	return err
}

func TestS3Connection(t *testing.T) {
	ctx := context.Background()

	client, err := NewS3TestClient()
	if err != nil {
		t.Skipf("S3 server not available: %v", err)
	}

	// Test list buckets
	_, err = client.client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		t.Fatalf("ListBuckets failed: %v", err)
	}

	t.Log("S3 connection successful")
}

func TestS3BasicOperations(t *testing.T) {
	ctx := context.Background()

	client, err := NewS3TestClient()
	if err != nil {
		t.Skipf("S3 server not available: %v", err)
	}

	if err := client.setupBucket(ctx); err != nil {
		t.Fatalf("Setup bucket failed: %v", err)
	}
	defer client.cleanupBucket(ctx)

	testKey := "test-basic-operations.txt"
	testContent := []byte("S3 basic operations test content")

	// Test PUT
	_, err = client.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(client.bucket),
		Key:    aws.String(testKey),
		Body:   bytes.NewReader(testContent),
	})
	if err != nil {
		t.Fatalf("PutObject failed: %v", err)
	}

	// Test GET
	result, err := client.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(client.bucket),
		Key:    aws.String(testKey),
	})
	if err != nil {
		t.Fatalf("GetObject failed: %v", err)
	}
	defer result.Body.Close()

	downloaded, err := io.ReadAll(result.Body)
	if err != nil {
		t.Fatalf("Read object failed: %v", err)
	}

	if !bytes.Equal(testContent, downloaded) {
		t.Errorf("Content mismatch")
	}

	// Test HEAD
	head, err := client.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(client.bucket),
		Key:    aws.String(testKey),
	})
	if err != nil {
		t.Fatalf("HeadObject failed: %v", err)
	}

	if head.ContentLength == nil || *head.ContentLength != int64(len(testContent)) {
		cl := int64(0)
		if head.ContentLength != nil {
			cl = *head.ContentLength
		}
		t.Errorf("ContentLength mismatch: got %d, want %d", cl, len(testContent))
	}

	// Test DELETE
	_, err = client.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(client.bucket),
		Key:    aws.String(testKey),
	})
	if err != nil {
		t.Fatalf("DeleteObject failed: %v", err)
	}

	// Verify deleted
	_, err = client.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(client.bucket),
		Key:    aws.String(testKey),
	})
	if err == nil {
		t.Error("Expected error after deletion")
	}
}

func TestS3RangeRequests(t *testing.T) {
	ctx := context.Background()

	client, err := NewS3TestClient()
	if err != nil {
		t.Skipf("S3 server not available: %v", err)
	}

	if err := client.setupBucket(ctx); err != nil {
		t.Fatalf("Setup bucket failed: %v", err)
	}
	defer client.cleanupBucket(ctx)

	testKey := "test-range.txt"
	testContent := bytes.Repeat([]byte("0123456789"), 1000) // 10KB

	_, err = client.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(client.bucket),
		Key:    aws.String(testKey),
		Body:   bytes.NewReader(testContent),
	})
	if err != nil {
		t.Fatalf("PutObject failed: %v", err)
	}

	testCases := []struct {
		name      string
		start     int64
		end       int64
		wantBytes int64
	}{
		{"first_100", 0, 99, 100},
		{"middle", 5000, 5099, 100},
		{"last_100", int64(len(testContent)) - 100, int64(len(testContent)) - 1, 100},
		{"single_byte", 5000, 5000, 1},
		{"full_range", 0, int64(len(testContent)) - 1, int64(len(testContent))},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rangeStr := fmt.Sprintf("bytes=%d-%d", tc.start, tc.end)
			result, err := client.client.GetObject(ctx, &s3.GetObjectInput{
				Bucket: aws.String(client.bucket),
				Key:    aws.String(testKey),
				Range:  aws.String(rangeStr),
			})
			if err != nil {
				t.Fatalf("GetObject with range failed: %v", err)
			}
			defer result.Body.Close()

			downloaded, err := io.ReadAll(result.Body)
			if err != nil {
				t.Fatalf("Read failed: %v", err)
			}

			if int64(len(downloaded)) != tc.wantBytes {
				t.Errorf("Downloaded %d bytes, want %d", len(downloaded), tc.wantBytes)
			}

			expected := testContent[tc.start : tc.end+1]
			if !bytes.Equal(downloaded, expected) {
				t.Error("Range content mismatch")
			}
		})
	}
}

func TestS3MultipartUpload(t *testing.T) {
	ctx := context.Background()

	client, err := NewS3TestClient()
	if err != nil {
		t.Skipf("S3 server not available: %v", err)
	}

	if err := client.setupBucket(ctx); err != nil {
		t.Fatalf("Setup bucket failed: %v", err)
	}
	defer client.cleanupBucket(ctx)

	testKey := "test-multipart.txt"
	partSize := int64(5 * 1024 * 1024) // 5MB parts
	totalSize := int64(15 * 1024 * 1024) // 15MB total

	// 1. Create multipart upload
	createResp, err := client.client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{
		Bucket: aws.String(client.bucket),
		Key:    aws.String(testKey),
	})
	if err != nil {
		t.Fatalf("CreateMultipartUpload failed: %v", err)
	}
	uploadID := createResp.UploadId

	// 2. Upload parts
	var completedParts []types.CompletedPart
	for partNumber := int32(1); partNumber <= 3; partNumber++ {
		partData := bytes.Repeat([]byte{byte(partNumber)}, int(partSize))
		uploadResp, err := client.client.UploadPart(ctx, &s3.UploadPartInput{
			Bucket:     aws.String(client.bucket),
			Key:        aws.String(testKey),
			UploadId:   uploadID,
			PartNumber: aws.Int32(partNumber),
			Body:       bytes.NewReader(partData),
		})
		if err != nil {
			t.Fatalf("UploadPart %d failed: %v", partNumber, err)
		}

		completedParts = append(completedParts, types.CompletedPart{
			ETag:       uploadResp.ETag,
			PartNumber: aws.Int32(partNumber),
		})
	}

	// 3. Complete multipart upload
	_, err = client.client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(client.bucket),
		Key:      aws.String(testKey),
		UploadId: uploadID,
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: completedParts,
		},
	})
	if err != nil {
		t.Fatalf("CompleteMultipartUpload failed: %v", err)
	}

	// 4. Verify the uploaded file
	head, err := client.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(client.bucket),
		Key:    aws.String(testKey),
	})
	if err != nil {
		t.Fatalf("HeadObject failed: %v", err)
	}

	if head.ContentLength == nil || *head.ContentLength != totalSize {
		cl := int64(0)
		if head.ContentLength != nil {
			cl = *head.ContentLength
		}
		t.Errorf("File size mismatch: got %d, want %d", cl, totalSize)
	}
}

func TestS3ListOperations(t *testing.T) {
	ctx := context.Background()

	client, err := NewS3TestClient()
	if err != nil {
		t.Skipf("S3 server not available: %v", err)
	}

	if err := client.setupBucket(ctx); err != nil {
		t.Fatalf("Setup bucket failed: %v", err)
	}
	defer client.cleanupBucket(ctx)

	// Create test files
	testFiles := []string{
		"file1.txt",
		"file2.txt",
		"dir1/file3.txt",
		"dir1/file4.txt",
		"dir1/subdir/file5.txt",
	}

	for _, key := range testFiles {
		_, err := client.client.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String(client.bucket),
			Key:    aws.String(key),
			Body:   bytes.NewReader([]byte("test content")),
		})
		if err != nil {
			t.Fatalf("PutObject %s failed: %v", key, err)
		}
	}

	// Test list all objects
	listResp, err := client.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(client.bucket),
	})
	if err != nil {
		t.Fatalf("ListObjectsV2 failed: %v", err)
	}

	if len(listResp.Contents) != len(testFiles) {
		t.Errorf("Listed %d objects, want %d", len(listResp.Contents), len(testFiles))
	}

	// Test prefix filtering
	listResp, err = client.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket:    aws.String(client.bucket),
		Prefix:    aws.String("dir1/"),
		Delimiter: aws.String("/"),
	})
	if err != nil {
		t.Fatalf("ListObjectsV2 with prefix failed: %v", err)
	}

	// Should have dir1/file3.txt, dir1/file4.txt, and dir1/subdir/ prefix
	t.Logf("Found %d objects with prefix dir1/", len(listResp.Contents))
	t.Logf("Found %d prefixes", len(listResp.CommonPrefixes))
}

func TestS3DeleteOperations(t *testing.T) {
	ctx := context.Background()

	client, err := NewS3TestClient()
	if err != nil {
		t.Skipf("S3 server not available: %v", err)
	}

	if err := client.setupBucket(ctx); err != nil {
		t.Fatalf("Setup bucket failed: %v", err)
	}
	defer client.cleanupBucket(ctx)

	// Create test files
	testFiles := []string{"file1.txt", "file2.txt", "file3.txt"}
	for _, key := range testFiles {
		_, err := client.client.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String(client.bucket),
			Key:    aws.String(key),
			Body:   bytes.NewReader([]byte("test content")),
		})
		if err != nil {
			t.Fatalf("PutObject %s failed: %v", key, err)
		}
	}

	// Delete multiple objects
	var objects []types.ObjectIdentifier
	for _, key := range testFiles[:2] {
		objects = append(objects, types.ObjectIdentifier{
			Key: aws.String(key),
		})
	}

	_, err = client.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
		Bucket: aws.String(client.bucket),
		Delete: &types.Delete{
			Objects: objects,
		},
	})
	if err != nil {
		t.Fatalf("DeleteObjects failed: %v", err)
	}

	// Verify deletion
	listResp, err := client.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(client.bucket),
	})
	if err != nil {
		t.Fatalf("ListObjectsV2 failed: %v", err)
	}

	if len(listResp.Contents) != 1 {
		t.Errorf("Expected 1 remaining object, got %d", len(listResp.Contents))
	}
}

func TestS3Metadata(t *testing.T) {
	ctx := context.Background()

	client, err := NewS3TestClient()
	if err != nil {
		t.Skipf("S3 server not available: %v", err)
	}

	if err := client.setupBucket(ctx); err != nil {
		t.Fatalf("Setup bucket failed: %v", err)
	}
	defer client.cleanupBucket(ctx)

	testKey := "test-metadata.txt"
	testContent := []byte("test content")

	// Upload with metadata
	_, err = client.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(client.bucket),
		Key:         aws.String(testKey),
		Body:        bytes.NewReader(testContent),
		ContentType: aws.String("text/plain"),
		Metadata: map[string]string{
			"custom-key":   "custom-value",
			"author":       "test",
			"description":  "S3 metadata test",
		},
	})
	if err != nil {
		t.Fatalf("PutObject with metadata failed: %v", err)
	}

	// Verify metadata
	head, err := client.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(client.bucket),
		Key:    aws.String(testKey),
	})
	if err != nil {
		t.Fatalf("HeadObject failed: %v", err)
	}

	if head.ContentType == nil || *head.ContentType != "text/plain" {
		t.Error("ContentType not set correctly")
	}

	expectedMetadata := map[string]string{
		"custom-key":   "custom-value",
		"author":       "test",
		"description":  "S3 metadata test",
	}

	for key, expectedValue := range expectedMetadata {
		value, exists := head.Metadata[key]
		if !exists {
			t.Errorf("Metadata key %s not found", key)
		}
		if value != expectedValue {
			t.Errorf("Metadata value mismatch: got %s, want %s", value, expectedValue)
		}
	}
}

func TestS3LargeFile(t *testing.T) {
	ctx := context.Background()

	client, err := NewS3TestClient()
	if err != nil {
		t.Skipf("S3 server not available: %v", err)
	}

	if err := client.setupBucket(ctx); err != nil {
		t.Fatalf("Setup bucket failed: %v", err)
	}
	defer client.cleanupBucket(ctx)

	testKey := "test-large-file.bin"
	fileSize := int64(50 * 1024 * 1024) // 50MB

	// Generate test data with pattern for verification
	data := make([]byte, fileSize)
	for i := int64(0); i < fileSize; i++ {
		data[i] = byte(i % 256)
	}

	t.Log("Uploading 50MB file...")
	startTime := time.Now()

	_, err = client.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(client.bucket),
		Key:    aws.String(testKey),
		Body:   bytes.NewReader(data),
	})
	if err != nil {
		t.Fatalf("PutObject failed: %v", err)
	}

	uploadDuration := time.Since(startTime)
	t.Logf("Upload took: %v (%.2f MB/s)", uploadDuration, float64(fileSize)/uploadDuration.Seconds()/1024/1024)

	// Download and verify
	t.Log("Downloading 50MB file...")
	startTime = time.Now()

	result, err := client.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(client.bucket),
		Key:    aws.String(testKey),
	})
	if err != nil {
		t.Fatalf("GetObject failed: %v", err)
	}
	defer result.Body.Close()

	downloaded, err := io.ReadAll(result.Body)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	downloadDuration := time.Since(startTime)
	t.Logf("Download took: %v (%.2f MB/s)", downloadDuration, float64(fileSize)/downloadDuration.Seconds()/1024/1024)

	if !bytes.Equal(data, downloaded) {
		t.Error("Large file data mismatch")
	}

	// Verify checksums
	origHash := calculateHash(data)
	downHash := calculateHash(downloaded)
	if origHash != downHash {
		t.Errorf("Hash mismatch: orig=%s, down=%s", origHash, downHash)
	}
}

func TestS3ConcurrentOperations(t *testing.T) {
	ctx := context.Background()

	client, err := NewS3TestClient()
	if err != nil {
		t.Skipf("S3 server not available: %v", err)
	}

	if err := client.setupBucket(ctx); err != nil {
		t.Fatalf("Setup bucket failed: %v", err)
	}
	defer client.cleanupBucket(ctx)

	// Concurrent uploads
	uploadCount := 20
	done := make(chan bool, uploadCount)

	for i := 0; i < uploadCount; i++ {
		go func(index int) {
			key := fmt.Sprintf("concurrent-file-%d.txt", index)
			content := []byte(fmt.Sprintf("Concurrent upload %d", index))

			_, err := client.client.PutObject(ctx, &s3.PutObjectInput{
				Bucket: aws.String(client.bucket),
				Key:    aws.String(key),
				Body:   bytes.NewReader(content),
			})
			if err != nil {
				log.Printf("Upload %d failed: %v", index, err)
				done <- false
				return
			}

			done <- true
		}(i)
	}

	// Wait for all uploads
	success := 0
	for i := 0; i < uploadCount; i++ {
		if <-done {
			success++
		}
	}

	if success != uploadCount {
		t.Errorf("Only %d/%d uploads succeeded", success, uploadCount)
	}

	// Verify all files
	listResp, err := client.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(client.bucket),
	})
	if err != nil {
		t.Fatalf("ListObjectsV2 failed: %v", err)
	}

	if len(listResp.Contents) != uploadCount {
		t.Errorf("Expected %d objects, got %d", uploadCount, len(listResp.Contents))
	}
}

func TestS3ErrorHandling(t *testing.T) {
	ctx := context.Background()

	client, err := NewS3TestClient()
	if err != nil {
		t.Skipf("S3 server not available: %v", err)
	}

	t.Run("NonExistentBucket", func(t *testing.T) {
		_, err := client.client.HeadBucket(ctx, &s3.HeadBucketInput{
			Bucket: aws.String("nonexistent-bucket"),
		})
		if err == nil {
			t.Error("Expected error for non-existent bucket")
		}
	})

	if err := client.setupBucket(ctx); err != nil {
		t.Fatalf("Setup bucket failed: %v", err)
	}
	defer client.cleanupBucket(ctx)

	t.Run("NonExistentObject", func(t *testing.T) {
		_, err := client.client.HeadObject(ctx, &s3.HeadObjectInput{
			Bucket: aws.String(client.bucket),
			Key:    aws.String("nonexistent-object"),
		})
		if err == nil {
			t.Error("Expected error for non-existent object")
		}
	})

	t.Run("InvalidRange", func(t *testing.T) {
		testKey := "test-range-error.txt"
		_, err := client.client.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String(client.bucket),
			Key:    aws.String(testKey),
			Body:   bytes.NewReader([]byte("test")),
		})
		if err != nil {
			t.Fatalf("PutObject failed: %v", err)
		}

		// Request invalid range
		_, err = client.client.GetObject(ctx, &s3.GetObjectInput{
			Bucket: aws.String(client.bucket),
			Key:    aws.String(testKey),
			Range:  aws.String("bytes=1000-2000"),
		})
		if err == nil {
			t.Error("Expected error for invalid range")
		}
	})
}
