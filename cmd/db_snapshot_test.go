package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDBSnapshotMissingArgAndDryRun(t *testing.T) {
	app := testApp(t)
	ssmAPI := &snapTestSSM{}
	app.EC2 = snapTestEC2{}
	app.SSM = ssmAPI
	app.S3 = snapTestS3{}

	root := NewRootCmd(app)
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"ec2:db-snapshot"})
	if err := root.Execute(); err == nil {
		t.Fatal("expected missing arg error")
	}

	dir := t.TempDir()
	root = NewRootCmd(app)
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"ec2:db-snapshot", "i-abc", "--dry-run", "--out", dir, "--prefix", "custom"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if ssmAPI.sent {
		t.Fatal("dry-run sent SSM")
	}
	if !strings.Contains(buf.String(), "dry-run") || !strings.Contains(buf.String(), "/custom/db-") {
		t.Fatalf("%s", buf.String())
	}
	if strings.Contains(buf.String(), "content.zip") {
		t.Fatalf("db-snapshot mentions content.zip: %s", buf.String())
	}
}

func TestDBSnapshotVerboseQuietConflict(t *testing.T) {
	app := testApp(t)
	app.EC2 = snapTestEC2{}
	root := NewRootCmd(app)
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"ec2:db-snapshot", "i-abc", "--verbose", "--quiet", "--dry-run"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "cannot use --verbose and --quiet") {
		t.Fatalf("err=%v", err)
	}
}

func TestDBSnapshotForceOverwrite(t *testing.T) {
	app := testApp(t)
	ssmAPI := &snapTestSSM{}
	app.EC2 = snapTestEC2{}
	app.SSM = ssmAPI
	app.S3 = snapTestS3{}

	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "db.sql"), []byte("old"), 0o644)

	root := NewRootCmd(app)
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"ec2:db-snapshot", "i-abc", "--out", dir})
	if err := root.Execute(); err == nil {
		t.Fatal("expected exists error")
	}

	root = NewRootCmd(app)
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"ec2:db-snapshot", "i-abc", "--out", dir, "--force"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !ssmAPI.sent {
		t.Fatal("expected SSM send")
	}
}
