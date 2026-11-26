package snapshot

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/punchlistdigital/pldi/internal/awsx"
	"github.com/punchlistdigital/pldi/internal/config"
	"github.com/punchlistdigital/pldi/internal/download"
	"github.com/punchlistdigital/pldi/internal/instances"
	"github.com/punchlistdigital/pldi/internal/ssmrun"
)

// DBRunOptions configures a db-snapshot (database dump only).
type DBRunOptions struct {
	IDOrName string
	Bucket   string
	Prefix   string // S3 folder; empty => instance slug
	WPRoot   string
	OutDir   string
	Timeout  time.Duration
	Force    bool
	DryRun   bool
	Verbose  bool
	Quiet    bool
	Progress io.Writer
	Stdout   io.Writer
	Now      func() time.Time
}

// RunDB orchestrates resolve → remote dump → download of a single db.sql.
// The dump is staged at s3://<bucket>/<prefix>/db-<timestamp>.sql and saved
// locally as <out>/db.sql.
func RunDB(ctx context.Context, ec2 awsx.EC2API, ssmAPI awsx.SSMAPI, s3api awsx.S3API, opts DBRunOptions) error {
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

	logf := newLogf(opts.Progress, opts.Quiet)

	logf("pldi: resolving %s", opts.IDOrName)
	inst, err := instances.Resolve(ctx, ec2, opts.IDOrName)
	if err != nil {
		return err
	}

	dbPath := filepath.Join(opts.OutDir, "db.sql")
	if err := refuseExisting([]string{dbPath}, opts.Force); err != nil {
		return err
	}

	folder := opts.Prefix
	if folder == "" {
		folder = config.InstanceSlug(inst.Name, inst.InstanceID)
	}
	dbFile := "db-" + timestamp(opts.Now) + ".sql"
	key := folder + "/" + dbFile
	dbURI := fmt.Sprintf("s3://%s/%s", opts.Bucket, key)

	logf("pldi: instance %s (%s); key=%s; out=%s; bucket=%s", inst.Name, inst.InstanceID, key, opts.OutDir, opts.Bucket)

	script, err := RenderRemoteScript(ScriptParams{
		Bucket: opts.Bucket,
		Prefix: folder,
		WPRoot: opts.WPRoot,
		DBOnly: true,
		DBFile: dbFile,
	})
	if err != nil {
		return err
	}

	if opts.DryRun {
		fmt.Fprintln(opts.Stdout, "dry-run: remote script")
		fmt.Fprintln(opts.Stdout, script)
		fmt.Fprintf(opts.Stdout, "dry-run: would upload/download %s\n", dbURI)
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
		reportSSMFailure(opts.Progress, opts.Verbose, res)
		return err
	}

	logf("pldi: downloading %s", dbFile)
	if err := download.Object(ctx, s3api, opts.Bucket, key, dbPath); err != nil {
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
	fmt.Fprintf(opts.Stdout, "s3: %s\n", dbURI)
	fmt.Fprintf(opts.Stdout, "local: %s (%d bytes)\n", dbPath, dbInfo.Size())
	fmt.Fprintf(opts.Stdout, "duration: %s\n", elapsed.Round(time.Second))
	return nil
}
