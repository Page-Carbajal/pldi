package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func chdir(t *testing.T, dir string) {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
}

func clearAWSEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"AWS_PROFILE", "AWS_REGION", "AWS_DEFAULT_REGION", "XDG_CONFIG_HOME",
		"PLDI_PROFILE", "PLDI_REGION", "PLDI_BUCKET", "PLDI_WP_ROOT", "PLDI_OUT", "PLDI_TIMEOUT", "PLDI_EXCLUDE",
	} {
		t.Setenv(key, "")
		_ = os.Unsetenv(key)
	}
}

func TestLoadDefaults(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	clearAWSEnv(t)

	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Settings.Bucket != "page-temp-backup" {
		t.Fatalf("bucket=%q", loaded.Settings.Bucket)
	}
	if loaded.Sources["bucket"] != SourceDefault {
		t.Fatalf("source=%q", loaded.Sources["bucket"])
	}
	if loaded.Settings.Timeout != 30*time.Minute {
		t.Fatalf("timeout=%v", loaded.Settings.Timeout)
	}
}

func TestLoadYAMLAndEnvPrecedence(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	clearAWSEnv(t)

	yamlBody := "bucket: from-yaml\nwp_root: /yaml/wp\n"
	if err := os.WriteFile("pldi.yaml", []byte(yamlBody), 0o644); err != nil {
		t.Fatal(err)
	}
	envBody := "PLDI_BUCKET=from-dotenv\n"
	if err := os.WriteFile(".env", []byte(envBody), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PLDI_BUCKET", "from-env")
	t.Setenv("PLDI_OUT", "/tmp/out")

	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Settings.Bucket != "from-env" {
		t.Fatalf("bucket=%q want from-env", loaded.Settings.Bucket)
	}
	if loaded.Sources["bucket"] != SourceEnv {
		t.Fatalf("bucket source=%q", loaded.Sources["bucket"])
	}
	if loaded.Settings.WPRoot != "/yaml/wp" {
		t.Fatalf("wp_root=%q", loaded.Settings.WPRoot)
	}
	if loaded.Sources["wp_root"] != SourceYAML {
		t.Fatalf("wp_root source=%q", loaded.Sources["wp_root"])
	}
	if loaded.Settings.Out != "/tmp/out" {
		t.Fatalf("out=%q", loaded.Settings.Out)
	}
	if loaded.YAMLPath != "pldi.yaml" {
		t.Fatalf("yaml path=%q", loaded.YAMLPath)
	}
	if !loaded.DotenvLoaded {
		t.Fatal("expected dotenv loaded")
	}
}

func TestApplyFlags(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	clearAWSEnv(t)

	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := loaded.ApplyFlags(map[string]string{
		"bucket": "flag-bucket",
		"region": "us-west-2",
	}, nil, false); err != nil {
		t.Fatal(err)
	}
	if loaded.Settings.Bucket != "flag-bucket" {
		t.Fatalf("bucket=%q", loaded.Settings.Bucket)
	}
	if loaded.Sources["bucket"] != SourceFlag {
		t.Fatalf("source=%q", loaded.Sources["bucket"])
	}
}

func TestLoadDotenvOnly(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	clearAWSEnv(t)
	if err := os.WriteFile(".env", []byte("PLDI_REGION=eu-west-1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Settings.Region != "eu-west-1" {
		t.Fatalf("region=%q", loaded.Settings.Region)
	}
	if loaded.Sources["region"] != SourceDotenv {
		t.Fatalf("source=%q", loaded.Sources["region"])
	}
}

func TestFindXDGConfig(t *testing.T) {
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, "pldi")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(cfgDir, "config.yaml")
	if err := os.WriteFile(path, []byte("bucket: xdg-bucket\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	chdir(t, work)
	clearAWSEnv(t)
	t.Setenv("XDG_CONFIG_HOME", dir)

	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Settings.Bucket != "xdg-bucket" {
		t.Fatalf("bucket=%q", loaded.Settings.Bucket)
	}
}
