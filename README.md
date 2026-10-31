# sensu-plugins-backup

Sensu Go checks verifying backups:

- `check-backup-s3`: every file of a local directory (default `/var/archives`) is present in an S3 bucket with the same size;
- `check-backup-ftp`: same thing on an FTP server;
- `check-backup-restic`: integrity of a restic repository (`restic check`).

Static Go binaries (no CGO, no Ruby runtime): one asset for every Linux distribution, `amd64` and `arm64`.

## Behavior

`check-backup-s3` / `check-backup-ftp` compare the files of the local directory, skipping subdirectories, hidden files and `.md5` files. Every problem is reported, not only the first one.

| Situation | State |
|---|---|
| All files present with the same size | OK |
| File missing on the remote, or size mismatch | CRITICAL |
| Remote error (connection, authentication, permission, remote directory…) | CRITICAL |
| Invalid argument, unreadable local directory | UNKNOWN |

`check-backup-restic`:

| Situation | State |
|---|---|
| `restic check` succeeds | OK (last line of the restic output) |
| `restic check` fails, env file missing or unreadable | CRITICAL |
| Invalid argument, restic binary not found | UNKNOWN |

## Usage

```
check-backup-s3 [flags]
  -b, --bucket-name string      S3 bucket holding the archives (required)
  -d, --directory-name string   Local directory holding the archives (default "/var/archives")
  -e, --s3-endpoint string      S3 endpoint URL, e.g. https://s3.fr-par.scw.cloud, https://s3.gra.io.cloud.ovh.net (required)
  -r, --s3-region string        S3 region, e.g. fr-par (Scaleway), gra (OVH) (required)
  -a, --s3-access-key string    S3 access key (default: AWS_ACCESS_KEY, then AWS_ACCESS_KEY_ID or ~/.aws/credentials)
  -k, --s3-secret-key string    S3 secret key (default: AWS_SECRET_KEY, then AWS_SECRET_ACCESS_KEY or ~/.aws/credentials)
      --timeout int             Timeout of the whole verification in seconds (default 60)

check-backup-ftp [flags]
  -H, --ftp-host string         FTP host, with an optional port (default 21) (required)
  -u, --ftp-user string         FTP user (default: anonymous)
  -p, --ftp-password string     FTP password
  -D, --ftp-directory string    Remote directory holding the archives (default: login directory)
  -d, --directory-name string   Local directory holding the archives (default "/var/archives")
      --timeout int             Timeout of the whole verification in seconds (default 60)

check-backup-restic [flags]
  -e, --env-file string           Environment file defining the restic repository and credentials (default "/etc/restic/env")
  -b, --restic-binary string      Path to the restic binary (default "restic")
      --read-data                 Read all data blobs (thorough but slow, use at low frequency)
      --read-data-subset string   Read a subset of data blobs: "n/t" (e.g. "1/7") or "x%" (e.g. "10%")
```

Every option can also be set with an environment variable, which keeps secrets out of the process list:

| Option | Variable |
|---|---|
| `--s3-access-key` / `--s3-secret-key` | `AWS_ACCESS_KEY` / `AWS_SECRET_KEY` |
| `--s3-region` / `--s3-endpoint` / `--bucket-name` | `CHECK_BACKUP_S3_REGION` / `CHECK_BACKUP_S3_ENDPOINT` / `CHECK_BACKUP_S3_BUCKET` |
| `--ftp-host` / `--ftp-user` / `--ftp-password` / `--ftp-directory` | `CHECK_BACKUP_FTP_HOST` / `CHECK_BACKUP_FTP_USER` / `CHECK_BACKUP_FTP_PASSWORD` / `CHECK_BACKUP_FTP_DIRECTORY` |
| `--directory-name` / `--timeout` | `CHECK_BACKUP_DIRECTORY` / `CHECK_BACKUP_TIMEOUT` |
| `--env-file` / `--restic-binary` | `CHECK_BACKUP_RESTIC_ENV_FILE` / `CHECK_BACKUP_RESTIC_BINARY` |
| `--read-data` / `--read-data-subset` | `CHECK_BACKUP_RESTIC_READ_DATA` / `CHECK_BACKUP_RESTIC_READ_DATA_SUBSET` |

Without S3 keys, `AWS_ACCESS_KEY_ID`/`AWS_SECRET_ACCESS_KEY` then `~/.aws/credentials` are used. Requests use path-style addressing (`https://<endpoint>/<bucket>/<key>`), supported by Scaleway and OVH Object Storage.

The FTP check uses passive mode (`PASV`) with the address announced by the server.

The restic env file is a dotenv file (`export` prefixes allowed); its variables override the agent environment for the `restic` process only.

Examples:

```sh
check-backup-s3 -b my-backups -e https://s3.fr-par.scw.cloud -r fr-par          # Scaleway
check-backup-s3 -b my-backups -e https://s3.gra.io.cloud.ovh.net -r gra          # OVH
check-backup-ftp -H ftp.example.com -u backup -D /backups   # password in CHECK_BACKUP_FTP_PASSWORD
check-backup-restic --read-data-subset 1/7
```

## Check definition

```yaml
---
type: CheckConfig
api_version: core/v2
metadata:
  name: check-backup-s3
spec:
  command: >-
    check-backup-s3
    {{ .annotations.check_backup_s3_args | default "" }}
  runtime_assets:
    - opsone/sensu-plugins-backup
  interval: 3600
  timeout: 120
  publish: true
  subscriptions:
    - backup-s3
```

Credentials are best passed through environment variables (`AWS_ACCESS_KEY`/`AWS_SECRET_KEY`, `CHECK_BACKUP_FTP_PASSWORD`), e.g. with the check `env_vars` or Sensu `secrets`, rather than on the command line.

Per-host arguments are set as an entity annotation, e.g. in the agent configuration:

```yaml
annotations:
  check_backup_s3_args: "-b my-backups -e https://s3.fr-par.scw.cloud -r fr-par"
```

`check-backup-restic --read-data` reads the whole repository: schedule it at a low frequency (daily or weekly) with a matching `timeout`.

## Migrating from the Ruby version (≤ 0.1.5)

- Commands lose their `.rb` suffix: `check-backup-s3.rb` → `check-backup-s3`, etc.
- `check-backup-ftp`: the host shorthand is now `-H` (`-h` is `--help`).
- `check-backup-s3`: `--s3-endpoint` and `--s3-region` are required (they defaulted to AWS `eu-west-3`).
- The checks report all failing files instead of stopping at the first one.
- The asset no longer depends on the Ruby runtime asset, nor on the Debian release.

## Build and release

```sh
make test          # go vet + unit tests
make integration   # runs the checks against a real S3 gateway, FTP server and restic repository in Docker
```

Release:

```sh
git tag 0.2.0 && git push origin 0.2.0
make release       # version taken from the tag, or forced: make release VERSION=0.2.0
sensuctl create -f dist/opsone-sensu-plugins-backup_0.2.0.yml
```

`make release` runs `make dist`, which runs the tests then produces in `dist/`:

- one archive per architecture (`linux/amd64`, `linux/arm64`) in Sensu asset layout (`bin/check-backup-*`);
- the sha512 checksums file;
- `opsone-sensu-plugins-backup_<version>.yml`, the asset definition pointing to the GitHub release of the tag.

It then creates the GitHub release of the tag with `gh`, uploads the archives and checksums, and uses the asset definition as release description.

`check-backup-s3 version` prints the version, commit and build date.
