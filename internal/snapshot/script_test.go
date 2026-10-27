package snapshot

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderRemoteScriptBasics(t *testing.T) {
	script, err := RenderRemoteScript(ScriptParams{
		Bucket:  "page-temp-backup",
		Prefix:  "my-site/2026-01-01-000000",
		WPRoot:  "/bitnami/wordpress",
		Exclude: []string{"cache", "updraft"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"set -euo pipefail",
		"pldi:step dump",
		"pldi:step zip",
		"pldi:step upload",
		"--defaults-extra-file",
		"page-temp-backup",
		"cache",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("script missing %q", want)
		}
	}
}

func TestBashN(t *testing.T) {
	script, err := RenderRemoteScript(ScriptParams{
		Bucket: "b",
		Prefix: "p",
		WPRoot: "/bitnami/wordpress",
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "remote.sh")
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", "-n", path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bash -n: %v\n%s", err, out)
	}
}

func TestShellcheck(t *testing.T) {
	if _, err := exec.LookPath("shellcheck"); err != nil {
		t.Skip("shellcheck not installed")
	}
	script, err := RenderRemoteScript(ScriptParams{
		Bucket:  "b",
		Prefix:  "p/q",
		WPRoot:  "/bitnami/wordpress",
		Exclude: []string{"cache"},
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "remote.sh")
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("shellcheck", "-x", path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("shellcheck: %v\n%s", err, out)
	}
}

func TestWPConfigParseFixtures(t *testing.T) {
	if _, err := exec.LookPath("php"); err != nil {
		t.Skip("php not installed")
	}
	script, err := RenderRemoteScript(ScriptParams{
		Bucket: "b",
		Prefix: "p",
		WPRoot: "/bitnami/wordpress",
	})
	if err != nil {
		t.Fatal(err)
	}

	fixtures := []struct {
		name     string
		contents string
		wantPass string
		wantHost string
		wantPort string
	}{
		{
			name: "single_quotes",
			contents: `<?php
define('DB_NAME', 'wpdb');
define('DB_USER', 'wpuser');
define('DB_PASSWORD', 'secret');
define('DB_HOST', '127.0.0.1:3306');
`,
			wantPass: "secret",
			wantHost: "127.0.0.1",
			wantPort: "3306",
		},
		{
			name: "double_quotes",
			contents: `<?php
define("DB_NAME", "wpdb");
define("DB_USER", "wpuser");
define("DB_PASSWORD", "sec\\\"ret");
define("DB_HOST", "localhost");
`,
			wantPass: `sec\"ret`,
			wantHost: "localhost",
			wantPort: "3306",
		},
		{
			name: "password_special",
			contents: `<?php
define('DB_NAME', 'wpdb');
define('DB_USER', 'wpuser');
define('DB_PASSWORD', 'p@ss"w$rd');
define('DB_HOST', '127.0.0.1:3307');
`,
			wantPass: `p@ss"w$rd`,
			wantHost: "127.0.0.1",
			wantPort: "3307",
		},
	}

	for _, fx := range fixtures {
		t.Run(fx.name, func(t *testing.T) {
			dir := t.TempDir()
			cfgPath := filepath.Join(dir, "wp-config.php")
			if err := os.WriteFile(cfgPath, []byte(fx.contents), 0o644); err != nil {
				t.Fatal(err)
			}
			harness := filepath.Join(dir, "harness.sh")
			body := "#!/usr/bin/env bash\nset -euo pipefail\nWP_CONFIG=" + shellQuote(cfgPath) + "\n"
			// Extract function from rendered script.
			start := strings.Index(script, "pldi_read_db_const() {")
			end := strings.Index(script, "echo \"pldi:step parse-config\"")
			if start < 0 || end < 0 {
				t.Fatal("could not locate parser in script")
			}
			body += script[start:end]
			body += `
DB_PASSWORD="$(pldi_read_db_const DB_PASSWORD)"
DB_HOST_RAW="$(pldi_read_db_const DB_HOST)"
DB_HOST="$DB_HOST_RAW"
DB_PORT="3306"
if [[ "$DB_HOST_RAW" == *:* ]]; then
  DB_HOST="${DB_HOST_RAW%%:*}"
  DB_PORT="${DB_HOST_RAW##*:}"
fi
printf 'PASS=%s\n' "$DB_PASSWORD"
printf 'HOST=%s\n' "$DB_HOST"
printf 'PORT=%s\n' "$DB_PORT"
`
			if err := os.WriteFile(harness, []byte(body), 0o700); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("bash", harness)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("%v\n%s", err, out)
			}
			got := string(out)
			if strings.Contains(got, "PASS="+fx.wantPass) == false {
				// password may need careful compare line-by-line
				lines := strings.Split(strings.TrimSpace(got), "\n")
				var passLine string
				for _, ln := range lines {
					if strings.HasPrefix(ln, "PASS=") {
						passLine = strings.TrimPrefix(ln, "PASS=")
					}
				}
				if passLine != fx.wantPass {
					t.Fatalf("password got %q want %q\nout=%q", passLine, fx.wantPass, got)
				}
			}
			if !strings.Contains(got, "HOST="+fx.wantHost) {
				t.Fatalf("host missing in %q", got)
			}
			if !strings.Contains(got, "PORT="+fx.wantPort) {
				t.Fatalf("port missing in %q", got)
			}
		})
	}
}

func TestPasswordNeverInRenderedScriptLiterals(t *testing.T) {
	// Rendered template must not embed a concrete password; it only reads from wp-config at runtime.
	script, err := RenderRemoteScript(ScriptParams{
		Bucket: "b",
		Prefix: "p",
		WPRoot: "/bitnami/wordpress",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"DB_PASSWORD=", "password=supersecret"} {
		if strings.Contains(script, bad) && bad == "password=supersecret" {
			t.Fatalf("unexpected secret literal")
		}
	}
	if strings.Contains(script, "echo \"$DB_PASSWORD\"") || strings.Contains(script, "echo $DB_PASSWORD") {
		t.Fatal("script echoes DB_PASSWORD")
	}
}

func shellQuote(s string) string {
	return `'` + strings.ReplaceAll(s, `'`, `'\''`) + `'`
}
