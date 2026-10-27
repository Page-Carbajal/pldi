package download

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type fakeS3 struct {
	body   string
	err    error
	calls  int
	failOn int
}

func (f *fakeS3) GetObject(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	if f.failOn > 0 && f.calls >= f.failOn {
		return &s3.GetObjectOutput{
			Body: io.NopCloser(&errReader{err: errors.New("read failed")}),
		}, nil
	}
	return &s3.GetObjectOutput{
		Body: io.NopCloser(strings.NewReader(f.body)),
	}, nil
}

type errReader struct {
	err error
}

func (e *errReader) Read([]byte) (int, error) { return 0, e.err }

func TestObjectSuccess(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "content.zip")
	err := Object(context.Background(), &fakeS3{body: "hello-zip"}, "b", "p/content.zip", dest)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte("hello-zip")) {
		t.Fatalf("%q", got)
	}
}

func TestObjectLeavesNoPartialOnFailure(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "db.sql")
	err := Object(context.Background(), &fakeS3{failOn: 1}, "b", "p/db.sql", dest)
	if err == nil {
		t.Fatal("expected error")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("partial file exists: %v", err)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}
}
