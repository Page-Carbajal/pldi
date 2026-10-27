package download

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/punchlistdigital/pldi/internal/awsx"
)

// Object downloads s3://bucket/key to destPath using a temp file then rename.
func Object(ctx context.Context, api awsx.S3API, bucket, key, destPath string) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(destPath), "."+filepath.Base(destPath)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}()

	out, err := api.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("s3 get s3://%s/%s: %w", bucket, key, err)
	}
	defer out.Body.Close()

	if _, err := io.Copy(tmp, out.Body); err != nil {
		return fmt.Errorf("download s3://%s/%s: %w", bucket, key, err)
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, destPath); err != nil {
		return err
	}
	return nil
}
