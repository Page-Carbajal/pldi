package snapshot

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var fixedNow = func() time.Time { return time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC) }

func TestRunDBDryRunNoAWSSideEffects(t *testing.T) {
	dir := t.TempDir()
	ssmAPI := &snapSSM{}
	s3api := &snapS3{}
	out := &bytes.Buffer{}
	progress := &bytes.Buffer{}
	err := RunDB(context.Background(), snapEC2{}, ssmAPI, s3api, DBRunOptions{
		IDOrName: "i-123",
		Bucket:   "b",
		WPRoot:   "/bitnami/wordpress",
		OutDir:   dir,
		DryRun:   true,
		Stdout:   out,
		Progress: progress,
		Now:      fixedNow,
	})
	if err != nil {
		t.Fatal(err)
	}
	if ssmAPI.sent || s3api.gets != 0 {
		t.Fatal("dry-run called SSM or S3")
	}
	if !strings.Contains(out.String(), "s3://b/my-site/db-2026-01-02-150405.sql") {
		t.Fatalf("key missing: %s", out.String())
	}
	if strings.Contains(out.String(), "content.zip") || strings.Contains(out.String(), "pldi:step zip") {
		t.Fatalf("db-only dry-run mentions wp-content zip: %s", out.String())
	}
	if !strings.Contains(progress.String(), "pldi: resolving") {
		t.Fatalf("expected phase banner: %q", progress.String())
	}
}

func TestRunDBPrefixOverridesFolder(t *testing.T) {
	out := &bytes.Buffer{}
	err := RunDB(context.Background(), snapEC2{}, &snapSSM{}, &snapS3{}, DBRunOptions{
		IDOrName: "i-123",
		Bucket:   "b",
		Prefix:   "custom",
		WPRoot:   "/bitnami/wordpress",
		OutDir:   t.TempDir(),
		DryRun:   true,
		Stdout:   out,
		Now:      fixedNow,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "s3://b/custom/db-2026-01-02-150405.sql") {
		t.Fatalf("prefix not applied: %s", out.String())
	}
}

func TestRunDBRefusesExistingFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "db.sql"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := RunDB(context.Background(), snapEC2{}, &snapSSM{}, &snapS3{}, DBRunOptions{
		IDOrName: "i-123",
		Bucket:   "b",
		WPRoot:   "/bitnami/wordpress",
		OutDir:   dir,
	})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("err=%v", err)
	}
}

func TestRunDBForceAndDownload(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "db.sql"), []byte("old"), 0o644)
	ssmAPI := &snapSSM{}
	s3api := &snapS3{}
	out := &bytes.Buffer{}
	err := RunDB(context.Background(), snapEC2{}, ssmAPI, s3api, DBRunOptions{
		IDOrName: "i-123",
		Bucket:   "b",
		WPRoot:   "/bitnami/wordpress",
		OutDir:   dir,
		Force:    true,
		Timeout:  time.Minute,
		Stdout:   out,
		Progress: io.Discard,
		Now:      fixedNow,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ssmAPI.sent || s3api.gets != 1 {
		t.Fatalf("sent=%v gets=%d", ssmAPI.sent, s3api.gets)
	}
	got, err := os.ReadFile(filepath.Join(dir, "db.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "data" {
		t.Fatalf("db.sql not overwritten: %q", got)
	}
	for _, want := range []string{"instance: My Site (i-123)", "s3://b/my-site/db-2026-01-02-150405.sql", "local: ", "duration:"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("summary missing %q: %s", want, out.String())
		}
	}
}

func TestRunDBQuietSuppressesPhases(t *testing.T) {
	progress := &bytes.Buffer{}
	out := &bytes.Buffer{}
	err := RunDB(context.Background(), snapEC2{}, &snapSSM{}, &snapS3{}, DBRunOptions{
		IDOrName: "i-123",
		Bucket:   "b",
		WPRoot:   "/bitnami/wordpress",
		OutDir:   t.TempDir(),
		Quiet:    true,
		Timeout:  time.Minute,
		Stdout:   out,
		Progress: progress,
	})
	if err != nil {
		t.Fatal(err)
	}
	if progress.Len() != 0 {
		t.Fatalf("quiet progress=%q", progress.String())
	}
	if !strings.Contains(out.String(), "instance:") {
		t.Fatalf("summary missing: %q", out.String())
	}
}

func TestRunDBSSMFailure(t *testing.T) {
	err := RunDB(context.Background(), snapEC2{}, &failSSM{}, &snapS3{}, DBRunOptions{
		IDOrName: "i-123",
		Bucket:   "b",
		WPRoot:   "/bitnami/wordpress",
		OutDir:   t.TempDir(),
		Timeout:  time.Minute,
		Progress: io.Discard,
	})
	if err == nil {
		t.Fatal("expected error")
	}
}
