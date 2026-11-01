# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

Sensu Go checks verifying backups, written in Go with `github.com/sensu/sensu-plugin-sdk`. Published as a Sensu asset (static binaries, `linux/amd64` only, see `ARCHS` in the Makefile). Same layout and tooling as `opsone/sensu-plugins-systemd`.

## Common Commands

```bash
make test          # go vet + unit tests
make build         # local binaries at the root
make integration   # Docker: versitygw (S3) + pyftpdlib (FTP) + real restic repo
make dist          # tests, then tar.gz per arch + sha512 + asset YAML in dist/
make release       # dist + GitHub release of the current tag (gh)
```

Version comes from `git describe --tags` (override with `VERSION=`), injected via ldflags into the SDK `version` package.

## Architecture

```
cmd/check-backup-s3/      # S3 presence/size check (minio-go)
cmd/check-backup-ftp/     # FTP presence/size check (jlaffaye/ftp, PASV trusting server IP)
cmd/check-backup-restic/  # runs `restic check`, env loaded from a dotenv file (godotenv)
internal/backup/          # LocalFiles (skips dirs, hidden, .md5), Verify against a Remote, output helpers
integration/              # Dockerfile + run.sh
scripts/asset-yml.sh      # builds the Sensu asset definition from the checksums file
```

S3/FTP checks implement `backup.Remote` (`Size(ctx, name)` returning `backup.ErrNotFound`) and delegate to `backup.Verify`, which reports every problem. Missing/mismatch and remote errors are CRITICAL; invalid args and local errors are UNKNOWN.

Options are declared with `sensu.PluginConfigOption`, each with an env var. `-h` is reserved by cobra for `--help` (redefining it panics).
