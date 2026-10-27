package snapshot

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/punchlistdigital/pldi/internal/awsx"
	"github.com/punchlistdigital/pldi/internal/config"
	"github.com/punchlistdigital/pldi/internal/download"
	"github.com/punchlistdigital/pldi/internal/instances"
	"github.com/punchlistdigital/pldi/internal/ssmrun"
)

// RunOptions configures a wp-snapshot.
type RunOptions struct {
	IDOrName string
	Bucket   string
	Prefix   string // empty => auto
	WPRoot   string
	OutDir   string
	Exclude  []string
	Timeout  time.Duration
	Force    bool
	DryRun   bool
	Verbose  bool
	Quiet    bool
	Progress io.Writer
	Stdout   io.Writer
	Now      func() time.Time
}

// Run orchestrates resolve → remote script → download.
func Run(ctx context.Context, ec2 awsx.EC2API, ssmAPI awsx.SSMAPI, s3api awsx.S3API, opts RunOptions) error {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}
	if opts.Progress == nil {
		opts.Progress = io.Discard
	}
	if opts.OutDir == "" {
		opts.OutDir = "."
	}

	logf := func(format string, args ...any) {
		if opts.Quiet {
			return
		}
		fmt.Fprintf(opts.Progress, format+"\n", args...)
	}

	logf("pldi: resolving %s", opts.IDOrName)
	inst, err := instances.Resolve(ctx, ec2, opts.IDOrName)
	if err != nil {
		return err
	}

	contentPath := filepath.Join(opts.OutDir, "content.zip")
	dbPath := filepath.Join(opts.OutDir, "db.sql")
	if !opts.Force {
		for _, p := range []string{contentPath, dbPath} {
			if _, err := os.Stat(p); err == nil {
				return fmt.Errorf("%s already exists (use --force to overwrite)", p)
			} else if !os.IsNotExist(err) {
				return err
			}
		}
	}

	prefix := opts.Prefix
	if prefix == "" {
		name := inst.Name
		if name == "-" {
			name = ""
		}
		ts := opts.Now().UTC().Format("2006-01-02-150405")
		prefix = config.DefaultPrefix(name, inst.InstanceID, ts)
	}

	logf("pldi: instance %s (%s); prefix=%s; out=%s; bucket=%s", inst.Name, inst.InstanceID, prefix, opts.OutDir, opts.Bucket)
	if len(opts.Exclude) == 0 {
		logf("pldi: excludes: (none)")
	} else {
		logf("pldi: excludes: %s", strings.Join(opts.Exclude, ", "))
	}

	script, err := RenderRemoteScript(ScriptParams{
		Bucket:  opts.Bucket,
		Prefix:  prefix,
		WPRoot:  opts.WPRoot,
		Exclude: opts.Exclude,
		Verbose: opts.Verbose,
	})
	if err != nil {
		return err
	}

	contentURI := fmt.Sprintf("s3://%s/%s/content.zip", opts.Bucket, prefix)
	dbURI := fmt.Sprintf("s3://%s/%s/db.sql", opts.Bucket, prefix)

	if opts.DryRun {
		fmt.Fprintln(opts.Stdout, "dry-run: remote script")
		fmt.Fprintln(opts.Stdout, script)
		fmt.Fprintf(opts.Stdout, "dry-run: would upload/download %s\n", contentURI)
		fmt.Fprintf(opts.Stdout, "dry-run: would upload/download %s\n", dbURI)
		fmt.Fprintf(opts.Stdout, "dry-run: local %s\n", contentPath)
		fmt.Fprintf(opts.Stdout, "dry-run: local %s\n", dbPath)
		return nil
	}

	logf("pldi: sending SSM command")
	start := opts.Now()
	res, err := ssmrun.SendAndWait(ctx, ssmAPI, inst.InstanceID, script, ssmrun.Options{
		Timeout:   opts.Timeout,
		PollEvery: 2 * time.Second,
		Progress:  opts.Progress,
		Verbose:   opts.Verbose,
		Quiet:     opts.Quiet,
	})
	if err != nil {
		if res.Stderr != "" {
			fmt.Fprintln(opts.Progress, res.Stderr)
		}
		if opts.Verbose && res.Stdout != "" {
			fmt.Fprintln(opts.Progress, res.Stdout)
		}
		return err
	}

	logf("pldi: downloading content.zip")
	if err := download.Object(ctx, s3api, opts.Bucket, prefix+"/content.zip", contentPath); err != nil {
		return err
	}
	logf("pldi: downloading db.sql")
	if err := download.Object(ctx, s3api, opts.Bucket, prefix+"/db.sql", dbPath); err != nil {
		return err
	}

	contentInfo, err := os.Stat(contentPath)
	if err != nil {
		return err
	}
	dbInfo, err := os.Stat(dbPath)
	if err != nil {
		return err
	}

	elapsed := opts.Now().Sub(start)
	if elapsed < res.Duration {
		elapsed = res.Duration
	}
	fmt.Fprintf(opts.Stdout, "instance: %s (%s)\n", inst.Name, inst.InstanceID)
	fmt.Fprintf(opts.Stdout, "s3: %s\n", contentURI)
	fmt.Fprintf(opts.Stdout, "s3: %s\n", dbURI)
	fmt.Fprintf(opts.Stdout, "local: %s (%d bytes)\n", contentPath, contentInfo.Size())
	fmt.Fprintf(opts.Stdout, "local: %s (%d bytes)\n", dbPath, dbInfo.Size())
	fmt.Fprintf(opts.Stdout, "duration: %s\n", elapsed.Round(time.Second))
	return nil
}
