# Changelog

## [0.2.1] - 2026-10-31

### Changed
- `check-backup-s3`: `--s3-access-key` / `--s3-secret-key` are read from `CHECK_BACKUP_S3_ACCESS_KEY` / `CHECK_BACKUP_S3_SECRET_KEY`, following the `CHECK_BACKUP_*` convention. `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY`, `AWS_ACCESS_KEY` / `AWS_SECRET_KEY` and `~/.aws/credentials` still work as fallback.

## [0.2.0] - 2026-10-31

### Changed
- Rewritten in Go with the Sensu plugin SDK: static binaries for `linux/amd64` and `linux/arm64`, no Ruby runtime asset.
- Commands renamed without the `.rb` suffix (`check-backup-s3`, `check-backup-ftp`, `check-backup-restic`).
- `check-backup-ftp`: the `--ftp-host` shorthand is `-H` (`-h` is `--help`).
- `check-backup-s3`: `--s3-endpoint` and `--s3-region` are required, no more AWS `eu-west-3` default.
- `check-backup-s3` / `check-backup-ftp` report every missing or mismatching file instead of the first one.

### Added
- Every option can be set through an environment variable.
- `--timeout` on `check-backup-s3` and `check-backup-ftp`.
- `check-backup-ftp`: optional port in `--ftp-host`, anonymous login when no user is given.
- `check-backup-s3`: falls back to `AWS_ACCESS_KEY_ID`/`AWS_SECRET_ACCESS_KEY` or `~/.aws/credentials` when no key is given.

## [0.1.5]

### Added
- `check-backup-restic`.
