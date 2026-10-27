# pldi

CLI to snapshot a live Bitnami WordPress site on EC2 via SSM: zip `wp-content`, dump the database, stage both in S3, and download `content.zip` + `db.sql` into your project directory.

## Install

```bash
go install github.com/punchlistdigital/pldi@latest
```

Or build from this repo:

```bash
make build
./pldi --version
```

Tagged releases are produced with GoReleaser (`make release` locally, or the `release` workflow on tag push).

## Quick start

```bash
pldi status
pldi ec2:instances list
pldi ec2:wp-snapshot i-05e6a35c06aaf677b
# or by Name tag:
pldi ec2:wp-snapshot trinityac
```

After a successful snapshot, the current directory contains `content.zip` and `db.sql`. Importing into DDEV (or similar) is left to you.

## Configuration

Precedence: **flags > process env (`PLDI_*` / `AWS_*`) > `.env` > YAML > defaults**.

YAML is loaded from the first of: `./pldi.yaml`, `./.pldi.yaml`, `$XDG_CONFIG_HOME/pldi/config.yaml` (or `~/.config/pldi/config.yaml`).

`.env` is loaded only from `./.env` if present.

See [`pldi.example.yaml`](pldi.example.yaml) and [`.env.example`](.env.example).

| Key | Env | Default | Notes |
|---|---|---|---|
| `profile` | `PLDI_PROFILE` / `AWS_PROFILE` | | AWS shared config profile |
| `region` | `PLDI_REGION` / `AWS_REGION` | | required for AWS calls |
| `bucket` | `PLDI_BUCKET` | `page-temp-backup` | S3 staging bucket |
| `wp_root` | `PLDI_WP_ROOT` | `/bitnami/wordpress` | |
| `out` | `PLDI_OUT` | `.` | local output directory |
| `exclude` | | | list of `wp-content` subpaths to skip |
| `timeout` | `PLDI_TIMEOUT` | `30m` | remote command timeout |

`--prefix` is **flag-only** (per-run). If omitted, pldi uses `<slugified-Name>/<YYYY-MM-DD-HHMMSS>` (or the instance ID when Name is missing).

AWS access keys are **not** accepted in pldi config; use the standard AWS credential chain.

## Commands

### `pldi status`

Shows effective config (with source per key) and probes STS `GetCallerIdentity`. Exit `0` when region, bucket, and identity are OK.

### `pldi ec2:instances list`

Table of `Name` and `Instance ID` (running only by default). `--all`, `--json`.

### `pldi ec2:wp-snapshot <instance-id|name>`

Flags: `--bucket`, `--prefix`, `--wp-root`, `--out`, `--exclude` (repeatable), `--timeout`, `--force`, `--dry-run`, `--verbose`/`-v`, `--quiet`/`-q`.

Progress (stderr by default):

- Phase banners (resolve, instance/prefix, SSM send, downloads)
- Remote step markers and a zip summary (top-level dirs + file count)
- `--verbose` / `-v` also streams remote stdout/stderr and lists zip contents
- `--quiet` / `-q` suppresses progress (errors + final summary only)

## IAM

### Caller (your laptop / CI user)

Typical permissions:

- `ec2:DescribeInstances`
- `ssm:DescribeInstanceInformation`
- `ssm:SendCommand` (document `AWS-RunShellScript`)
- `ssm:GetCommandInvocation`
- `s3:GetObject` on the staging bucket/prefix
- `sts:GetCallerIdentity`

### Instance role

The EC2 instance profile must allow:

- SSM managed-instance core (so the agent is online)
- `s3:PutObject` on the staging bucket/prefix (remote script uploads with the instance `aws` CLI)

## S3 lifecycle

Artifacts are left in the bucket after download. Attach a lifecycle rule on `page-temp-backup` (or your bucket) to expire objects after a few days.

## Development

```bash
make test
make vet
make lint
make build
```

Requires Go 1.23+, and for full script tests: `bash`, `php`, `shellcheck`.
