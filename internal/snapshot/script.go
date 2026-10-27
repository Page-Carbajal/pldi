package snapshot

import (
	"bytes"
	"embed"
	"fmt"
	"text/template"
)

//go:embed remote.sh.tmpl
var remoteFS embed.FS

// ScriptParams are values rendered into the remote bash template.
type ScriptParams struct {
	Bucket  string
	Prefix  string
	WPRoot  string
	Exclude []string
	Verbose bool
}

var remoteTmpl = template.Must(template.New("remote.sh.tmpl").ParseFS(remoteFS, "remote.sh.tmpl"))

// RenderRemoteScript returns the bash script for SSM Run Command.
func RenderRemoteScript(p ScriptParams) (string, error) {
	if p.Bucket == "" || p.Prefix == "" || p.WPRoot == "" {
		return "", fmt.Errorf("bucket, prefix, and wp_root are required")
	}
	var buf bytes.Buffer
	if err := remoteTmpl.ExecuteTemplate(&buf, "remote.sh.tmpl", p); err != nil {
		return "", err
	}
	return buf.String(), nil
}
