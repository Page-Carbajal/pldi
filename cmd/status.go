package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/spf13/cobra"

	"github.com/punchlistdigital/pldi/internal/config"
)

func newStatusCmd(app *App) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show effective config and verify AWS identity",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(cmd, app, asJSON)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	return cmd
}

type statusIdentity struct {
	Account string `json:"account,omitempty"`
	ARN     string `json:"arn,omitempty"`
	UserID  string `json:"userId,omitempty"`
}

type statusJSON struct {
	OK       bool              `json:"ok"`
	Config   map[string]any    `json:"config"`
	Sources  map[string]string `json:"sources"`
	YAMLPath string            `json:"yamlPath,omitempty"`
	Dotenv   bool              `json:"dotenvLoaded"`
	Identity *statusIdentity   `json:"identity,omitempty"`
	Error    string            `json:"error,omitempty"`
}

func runStatus(cmd *cobra.Command, app *App, asJSON bool) error {
	cfg := app.Cfg
	settings := cfg.Settings

	sources := map[string]string{}
	for _, key := range config.ConfigurableKeys {
		if src, ok := cfg.Sources[key]; ok {
			sources[key] = string(src)
		} else {
			sources[key] = string(config.SourceDefault)
		}
	}

	configMap := map[string]any{
		"profile": settings.Profile,
		"region":  settings.Region,
		"bucket":  settings.Bucket,
		"wp_root": settings.WPRoot,
		"out":     settings.Out,
		"exclude": settings.Exclude,
		"timeout": settings.Timeout.String(),
	}

	var (
		ident    *statusIdentity
		probeErr error
	)

	problems := []string{}
	if settings.Region == "" {
		problems = append(problems, "region is empty")
	}
	if settings.Bucket == "" {
		problems = append(problems, "bucket is empty")
	}

	if app.STS != nil && settings.Region != "" {
		out, err := app.STS.GetCallerIdentity(cmd.Context(), &sts.GetCallerIdentityInput{})
		if err != nil {
			probeErr = err
			problems = append(problems, fmt.Sprintf("aws identity: %v", err))
		} else {
			ident = &statusIdentity{
				Account: deref(out.Account),
				ARN:     deref(out.Arn),
				UserID:  deref(out.UserId),
			}
		}
	} else if settings.Region == "" {
		// already recorded
	} else if app.STS == nil {
		problems = append(problems, "aws sts client not configured")
	}

	ok := len(problems) == 0
	errMsg := strings.Join(problems, "; ")

	if asJSON {
		payload := statusJSON{
			OK:       ok,
			Config:   configMap,
			Sources:  sources,
			YAMLPath: cfg.YAMLPath,
			Dotenv:   cfg.DotenvLoaded,
			Identity: ident,
			Error:    errMsg,
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		if err := enc.Encode(payload); err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("%s", errMsg)
		}
		return nil
	}

	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "pldi status\n")
	if cfg.YAMLPath != "" {
		fmt.Fprintf(w, "yaml: %s\n", cfg.YAMLPath)
	} else {
		fmt.Fprintf(w, "yaml: (none)\n")
	}
	fmt.Fprintf(w, "dotenv: %v\n", cfg.DotenvLoaded)
	printSetting(w, "profile", settings.Profile, sources["profile"])
	printSetting(w, "region", settings.Region, sources["region"])
	printSetting(w, "bucket", settings.Bucket, sources["bucket"])
	printSetting(w, "wp_root", settings.WPRoot, sources["wp_root"])
	printSetting(w, "out", settings.Out, sources["out"])
	printSetting(w, "timeout", settings.Timeout.Round(time.Second).String(), sources["timeout"])
	ex := strings.Join(settings.Exclude, ",")
	if ex == "" {
		ex = "(none)"
	}
	printSetting(w, "exclude", ex, sources["exclude"])

	if ident != nil {
		fmt.Fprintf(w, "identity:\n  account: %s\n  arn: %s\n  userId: %s\n", ident.Account, ident.ARN, ident.UserID)
	}
	if probeErr != nil {
		fmt.Fprintf(w, "identity error: %v\n", probeErr)
	}

	if !ok {
		fmt.Fprintf(w, "status: FAIL (%s)\n", errMsg)
		return fmt.Errorf("%s", errMsg)
	}
	fmt.Fprintf(w, "status: OK\n")
	return nil
}

func printSetting(w io.Writer, key, value, source string) {
	if value == "" {
		value = "(empty)"
	}
	fmt.Fprintf(w, "%s: %s  [%s]\n", key, value, source)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
