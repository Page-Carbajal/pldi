package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/knadh/koanf/parsers/dotenv"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/env/v2"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

// Source identifies where a setting value came from.
type Source string

const (
	SourceDefault Source = "default"
	SourceYAML    Source = "yaml"
	SourceDotenv  Source = "dotenv"
	SourceEnv     Source = "env"
	SourceFlag    Source = "flag"
)

// Keys that may be set via YAML / .env / process env / flags.
var ConfigurableKeys = []string{
	"profile", "region", "bucket", "wp_root", "out", "exclude", "timeout",
}

// Settings holds resolved configuration (prefix is flag-only elsewhere).
type Settings struct {
	Profile string
	Region  string
	Bucket  string
	WPRoot  string
	Out     string
	Exclude []string
	Timeout time.Duration
}

// Loaded is settings plus provenance metadata.
type Loaded struct {
	Settings     Settings
	Sources      map[string]Source
	YAMLPath     string
	DotenvLoaded bool
}

const (
	defaultBucket  = "page-temp-backup"
	defaultWPRoot  = "/bitnami/wordpress"
	defaultOut     = "."
	defaultTimeout = 30 * time.Minute
)

// Load reads defaults, then YAML, .env, and PLDI_* process env.
// Flag overrides are applied later via ApplyFlags.
func Load() (*Loaded, error) {
	k := koanf.New(".")
	sources := map[string]Source{}

	defaults := map[string]any{
		"bucket":  defaultBucket,
		"wp_root": defaultWPRoot,
		"out":     defaultOut,
		"timeout": defaultTimeout.String(),
	}
	if err := k.Load(confmap.Provider(defaults, "."), nil); err != nil {
		return nil, err
	}
	for _, key := range []string{"bucket", "wp_root", "out", "timeout"} {
		sources[key] = SourceDefault
	}

	yamlPath, err := findYAML()
	if err != nil {
		return nil, err
	}
	if yamlPath != "" {
		before := snapshot(k)
		if err := k.Load(file.Provider(yamlPath), yaml.Parser()); err != nil {
			return nil, fmt.Errorf("load yaml %s: %w", yamlPath, err)
		}
		markChanged(k, before, sources, SourceYAML)
	}

	dotenvLoaded := false
	if _, err := os.Stat(".env"); err == nil {
		before := snapshot(k)
		parser := dotenv.ParserEnv("PLDI_", ".", func(s string) string {
			return strings.ToLower(strings.TrimPrefix(s, "PLDI_"))
		})
		if err := k.Load(file.Provider(".env"), parser); err != nil {
			return nil, fmt.Errorf("load .env: %w", err)
		}
		dotenvLoaded = true
		markChanged(k, before, sources, SourceDotenv)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	before := snapshot(k)
	if err := k.Load(env.Provider(".", env.Opt{
		Prefix: "PLDI_",
		TransformFunc: func(key, val string) (string, any) {
			key = strings.ToLower(strings.TrimPrefix(key, "PLDI_"))
			if val == "" {
				return "", nil
			}
			return key, val
		},
	}), nil); err != nil {
		return nil, fmt.Errorf("load env: %w", err)
	}
	markChanged(k, before, sources, SourceEnv)

	// AWS_* are not PLDI_* but inform profile/region defaults when unset.
	if k.String("profile") == "" {
		if p := os.Getenv("AWS_PROFILE"); p != "" {
			_ = k.Set("profile", p)
			sources["profile"] = SourceEnv
		}
	}
	if k.String("region") == "" {
		if r := os.Getenv("AWS_REGION"); r != "" {
			_ = k.Set("region", r)
			sources["region"] = SourceEnv
		} else if r := os.Getenv("AWS_DEFAULT_REGION"); r != "" {
			_ = k.Set("region", r)
			sources["region"] = SourceEnv
		}
	}

	settings, err := settingsFrom(k)
	if err != nil {
		return nil, err
	}

	return &Loaded{
		Settings:     settings,
		Sources:      sources,
		YAMLPath:     yamlPath,
		DotenvLoaded: dotenvLoaded,
	}, nil
}

// ApplyFlags overlays CLI flag values and marks their sources as flag.
func (l *Loaded) ApplyFlags(flags map[string]string, exclude []string, excludeSet bool) error {
	k := koanf.New(".")
	base := map[string]any{
		"profile": l.Settings.Profile,
		"region":  l.Settings.Region,
		"bucket":  l.Settings.Bucket,
		"wp_root": l.Settings.WPRoot,
		"out":     l.Settings.Out,
		"timeout": l.Settings.Timeout.String(),
	}
	if len(l.Settings.Exclude) > 0 {
		base["exclude"] = l.Settings.Exclude
	}
	if err := k.Load(confmap.Provider(base, "."), nil); err != nil {
		return err
	}
	for key, val := range flags {
		if val == "" {
			continue
		}
		if err := k.Set(key, val); err != nil {
			return err
		}
		l.Sources[key] = SourceFlag
	}
	if excludeSet {
		if err := k.Set("exclude", exclude); err != nil {
			return err
		}
		l.Sources["exclude"] = SourceFlag
	}
	settings, err := settingsFrom(k)
	if err != nil {
		return err
	}
	l.Settings = settings
	return nil
}

func settingsFrom(k *koanf.Koanf) (Settings, error) {
	timeout := defaultTimeout
	if s := k.String("timeout"); s != "" {
		d, err := time.ParseDuration(s)
		if err != nil {
			return Settings{}, fmt.Errorf("parse timeout %q: %w", s, err)
		}
		timeout = d
	}
	var exclude []string
	if k.Exists("exclude") {
		exclude = k.Strings("exclude")
	}
	return Settings{
		Profile: k.String("profile"),
		Region:  k.String("region"),
		Bucket:  k.String("bucket"),
		WPRoot:  k.String("wp_root"),
		Out:     k.String("out"),
		Exclude: exclude,
		Timeout: timeout,
	}, nil
}

func findYAML() (string, error) {
	candidates := []string{"pldi.yaml", ".pldi.yaml"}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		candidates = append(candidates, filepath.Join(xdg, "pldi", "config.yaml"))
	} else if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, ".config", "pldi", "config.yaml"))
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
	}
	return "", nil
}

func snapshot(k *koanf.Koanf) map[string]string {
	out := map[string]string{}
	for _, key := range ConfigurableKeys {
		if key == "exclude" {
			out[key] = strings.Join(k.Strings(key), ",")
			continue
		}
		out[key] = k.String(key)
	}
	return out
}

func markChanged(k *koanf.Koanf, before map[string]string, sources map[string]Source, src Source) {
	after := snapshot(k)
	for key, val := range after {
		if before[key] != val {
			sources[key] = src
		}
	}
}
