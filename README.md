# sensu-plugins-backup

Sensu Go checks verifying backups:

- `check-backup-s3`: every file of a local directory (e.g. `/var/archives`) is present in an S3 bucket with the same size;
- `check-backup-ftp`: same thing on an FTP server;
- `check-backup-restic`: integrity of a restic repository (`restic check`).

Each one also checks that the backups are recent, to catch backups that silently stop (see [Backup age](#backup-age)).

Static Go binaries (no CGO, no Ruby runtime): one asset for every Linux distribution, `amd64` only.

## Behavior

`check-backup-s3` / `check-backup-ftp` compare the files of the local directory, skipping subdirectories, hidden files and `.md5` files. Every problem is reported, not only the first one.

| Situation | State |
|---|---|
| All files present with the same size | OK |
| File missing on the remote, or size mismatch | CRITICAL |
| Remote error (connection, authentication, permission, remote directory…) | CRITICAL |
| A set of archives older than `--max-age`, or no archive at all | CRITICAL |
| Invalid argument, unreadable local directory | UNKNOWN |

### Backup age

A remote copy or a sound repository says nothing about whether backups still run: backup-manager leaves its old archives in place until their TTL expires, restic keeps its old snapshots. Each check therefore requires the newest backup of every set to be younger than `--max-age` hours (default 26, for daily backups; `0` disables the age check):

- `check-backup-s3` / `check-backup-ftp`: archives of the local directory, grouped by name without their backup-manager date (`host-etc.20261108.tar.gz` and `host-etc.20261107.master.tar.gz` belong to `host-etc.*.tar.gz`), using the modification time of the newest file;
- `check-backup-restic`: newest snapshot of each host and paths group (`restic snapshots --latest 1 --group-by host,paths`), only checked when `restic check` succeeds.

Grouping catches a single failing target, e.g. a MySQL dump no longer produced while the tarballs are. The age result is appended to the check output:

```
check-backup-s3 CRITICAL: 3 file(s) present in S3 bucket backups; 1 archive set(s) older than 26h in /var/archives: h-mysql-db.*.sql.gz (last h-mysql-db.20261106.sql.gz, 54h ago)
check-backup-restic CRITICAL: [QUICK] no errors were found; 1 snapshot group(s) older than 26h in repository: web1:/etc,/var/www (last snapshot 6232c79b, 30h ago)
```

After removing a backup target, its set stays CRITICAL until its old archives are deleted (or purged by backup-manager), or its snapshots forgotten (`restic forget`).

`check-backup-restic`:

| Situation | State |
|---|---|
| `restic check` succeeds | OK (last line of the restic output) |
| `restic check` fails, env file missing or unreadable | CRITICAL |
| A snapshot group older than `--max-age`, no snapshot at all, `restic snapshots` fails | CRITICAL |
| Invalid argument, restic binary not found | UNKNOWN |

## Usage

```
check-backup-s3 [flags]
  -b, --bucket-name string      S3 bucket holding the archives (required)
  -d, --directory-name string   Local directory holding the archives, e.g. /var/archives (required)
  -e, --s3-endpoint string      S3 endpoint URL, e.g. https://s3.fr-par.scw.cloud, https://s3.gra.io.cloud.ovh.net (required)
  -r, --s3-region string        S3 region, e.g. fr-par (Scaleway), gra (OVH) (required)
  -a, --s3-access-key string    S3 access key (fallback: AWS_ACCESS_KEY_ID, AWS_ACCESS_KEY, ~/.aws/credentials)
  -k, --s3-secret-key string    S3 secret key (fallback: AWS_SECRET_ACCESS_KEY, AWS_SECRET_KEY, ~/.aws/credentials)
      --timeout int             Timeout of the whole verification in seconds (default 60)
      --max-age int             Maximum age in hours of the newest backup of each set, 0 disables the age check (default 26)

check-backup-ftp [flags]
  -H, --ftp-host string         FTP host, with an optional port (default 21) (required)
  -u, --ftp-user string         FTP user (default: anonymous)
  -p, --ftp-password string     FTP password
  -D, --ftp-directory string    Remote directory holding the archives (default: login directory)
  -d, --directory-name string   Local directory holding the archives, e.g. /var/archives (required)
      --timeout int             Timeout of the whole verification in seconds (default 60)
      --max-age int             Maximum age in hours of the newest backup of each set, 0 disables the age check (default 26)

check-backup-restic [flags]
  -e, --env-file string           Environment file defining the restic repository and credentials (required)
  -b, --restic-binary string      Path to the restic binary (default "restic")
      --read-data                 Read all data blobs (thorough but slow, use at low frequency)
      --read-data-subset string   Read a subset of data blobs: "n/t" (e.g. "1/7") or "x%" (e.g. "10%")
      --max-age int               Maximum age in hours of the newest backup of each set, 0 disables the age check (default 26)
```

Every option can also be set with an environment variable, which keeps secrets out of the process list:

| Option | Variable |
|---|---|
| `--s3-access-key` / `--s3-secret-key` | `CHECK_BACKUP_S3_ACCESS_KEY` / `CHECK_BACKUP_S3_SECRET_KEY` |
| `--s3-region` / `--s3-endpoint` / `--bucket-name` | `CHECK_BACKUP_S3_REGION` / `CHECK_BACKUP_S3_ENDPOINT` / `CHECK_BACKUP_S3_BUCKET` |
| `--ftp-host` / `--ftp-user` / `--ftp-password` / `--ftp-directory` | `CHECK_BACKUP_FTP_HOST` / `CHECK_BACKUP_FTP_USER` / `CHECK_BACKUP_FTP_PASSWORD` / `CHECK_BACKUP_FTP_DIRECTORY` |
| `--directory-name` / `--timeout` / `--max-age` | `CHECK_BACKUP_DIRECTORY` / `CHECK_BACKUP_TIMEOUT` / `CHECK_BACKUP_MAX_AGE` |
| `--env-file` / `--restic-binary` | `CHECK_BACKUP_RESTIC_ENV_FILE` / `CHECK_BACKUP_RESTIC_BINARY` |
| `--read-data` / `--read-data-subset` | `CHECK_BACKUP_RESTIC_READ_DATA` / `CHECK_BACKUP_RESTIC_READ_DATA_SUBSET` |

Without S3 keys, the standard AWS sources are used: `AWS_ACCESS_KEY_ID`/`AWS_SECRET_ACCESS_KEY` (or `AWS_ACCESS_KEY`/`AWS_SECRET_KEY`), then `~/.aws/credentials`. Requests use path-style addressing (`https://<endpoint>/<bucket>/<key>`), supported by Scaleway and OVH Object Storage.

The FTP check uses passive mode (`PASV`) with the address announced by the server.

The restic env file is a dotenv file (`export` prefixes allowed); its variables override the agent environment for the `restic` process only.

Examples:

```sh
check-backup-s3 -d /var/archives -b my-backups -e https://s3.fr-par.scw.cloud -r fr-par    # Scaleway
check-backup-s3 -d /var/archives -b my-backups -e https://s3.gra.io.cloud.ovh.net -r gra    # OVH
check-backup-ftp -d /var/archives -H ftp.example.com -u backup -D /backups   # password in CHECK_BACKUP_FTP_PASSWORD
check-backup-restic -e /etc/restic/env --read-data-subset 1/7
```

## Check definitions

Every option is passed through `env_vars`, filled per host from entity annotations with token substitution (applied by the agent to the whole check definition, `env_vars` included). This keeps credentials out of the process list, and one annotation maps to one option.

Naming rule: an annotation is named after its environment variable, lowercased, without the `CHECK_` prefix (`CHECK_BACKUP_FTP_HOST` → `backup_ftp_host`).

- Optional options get an explicit, non-empty `default`, matching the plugin default. Never use `| default ""`: Sensu rejects an empty environment variable (`FOO=`), and the agent then silently drops the check, without any event.
- Required options have no `default`: on a host missing the annotation, the agent does not run the check and reports an `unmatched token` error event instead.

Annotations, e.g. in the agent configuration (`/etc/sensu/agent.yml`):

```yaml
annotations:
  # S3
  backup_s3_bucket: my-backups
  backup_s3_endpoint: https://s3.fr-par.scw.cloud     # OVH: https://s3.gra.io.cloud.ovh.net
  backup_s3_region: fr-par                            # OVH: gra
  backup_s3_access_key: SCWXXXXXXXXXXXXXXXXX
  backup_s3_secret_key: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
  # FTP
  backup_ftp_host: ftp.example.com                    # optional port: ftp.example.com:2121
  backup_ftp_user: backup
  backup_ftp_password: secret
  backup_ftp_directory: /backups                      # optional, default "." (login directory)
  # S3 and FTP, optional (check definition default: /var/archives)
  backup_directory: /var/archives
  # every check, optional
  backup_max_age: 170                                 # hours, e.g. weekly backups; default 26, 0 disables
  # restic, optional
  backup_restic_env_file: /etc/restic/env             # check definition default: /etc/restic/env
  backup_restic_read_data_subset: 1/7                 # weekly check, default 1/7
```

### S3

```yaml
---
type: CheckConfig
api_version: core/v2
metadata:
  name: check-backup-s3
spec:
  command: check-backup-s3
  env_vars:
    - CHECK_BACKUP_S3_BUCKET={{ .annotations.backup_s3_bucket }}
    - CHECK_BACKUP_S3_ENDPOINT={{ .annotations.backup_s3_endpoint }}
    - CHECK_BACKUP_S3_REGION={{ .annotations.backup_s3_region }}
    - CHECK_BACKUP_S3_ACCESS_KEY={{ .annotations.backup_s3_access_key }}
    - CHECK_BACKUP_S3_SECRET_KEY={{ .annotations.backup_s3_secret_key }}
    - CHECK_BACKUP_DIRECTORY={{ .annotations.backup_directory | default "/var/archives" }}
    - CHECK_BACKUP_MAX_AGE={{ .annotations.backup_max_age | default "26" }}
  runtime_assets:
    - opsone/sensu-plugins-backup
  interval: 3600
  timeout: 120
  publish: true
  subscriptions:
    - backup-s3
```

### FTP

```yaml
---
type: CheckConfig
api_version: core/v2
metadata:
  name: check-backup-ftp
spec:
  command: check-backup-ftp
  env_vars:
    - CHECK_BACKUP_FTP_HOST={{ .annotations.backup_ftp_host }}
    - CHECK_BACKUP_FTP_USER={{ .annotations.backup_ftp_user }}
    - CHECK_BACKUP_FTP_PASSWORD={{ .annotations.backup_ftp_password }}
    - CHECK_BACKUP_FTP_DIRECTORY={{ .annotations.backup_ftp_directory | default "." }}
    - CHECK_BACKUP_DIRECTORY={{ .annotations.backup_directory | default "/var/archives" }}
    - CHECK_BACKUP_MAX_AGE={{ .annotations.backup_max_age | default "26" }}
  runtime_assets:
    - opsone/sensu-plugins-backup
  interval: 3600
  timeout: 120
  publish: true
  subscriptions:
    - backup-ftp
```

### restic

Two checks: a quick one (repository structure, a few seconds, plus the snapshot age) and a weekly one reading a subset of the data, with the age check disabled to avoid a duplicate alert. The age is only checked once a day with the quick check: lower its `interval` for a faster alert. `--read-data` (`CHECK_BACKUP_RESTIC_READ_DATA=true`) reads the whole repository: keep it for small repositories, with a matching `timeout`.

```yaml
---
type: CheckConfig
api_version: core/v2
metadata:
  name: check-backup-restic
spec:
  command: check-backup-restic
  env_vars:
    - CHECK_BACKUP_RESTIC_ENV_FILE={{ .annotations.backup_restic_env_file | default "/etc/restic/env" }}
    - CHECK_BACKUP_MAX_AGE={{ .annotations.backup_max_age | default "26" }}
  runtime_assets:
    - opsone/sensu-plugins-backup
  interval: 86400
  timeout: 600
  publish: true
  subscriptions:
    - backup-restic
---
type: CheckConfig
api_version: core/v2
metadata:
  name: check-backup-restic-data
spec:
  command: check-backup-restic
  env_vars:
    - CHECK_BACKUP_RESTIC_ENV_FILE={{ .annotations.backup_restic_env_file | default "/etc/restic/env" }}
    - CHECK_BACKUP_RESTIC_READ_DATA_SUBSET={{ .annotations.backup_restic_read_data_subset | default "1/7" }}
    - CHECK_BACKUP_MAX_AGE=0
  runtime_assets:
    - opsone/sensu-plugins-backup
  cron: "0 4 * * 0"
  timeout: 7200
  publish: true
  subscriptions:
    - backup-restic
```

The `restic` binary must be installed on the host (or given with `CHECK_BACKUP_RESTIC_BINARY`), and the env file readable by the `sensu` user.

Entity annotations are visible in the Sensu dashboard and API: to keep credentials out of them, use Sensu [secrets](https://docs.sensu.io/sensu-go/latest/operations/manage-secrets/secrets/) instead of `env_vars` for the keys and passwords.

## Migrating from the Ruby version (≤ 0.1.5)

- Commands lose their `.rb` suffix: `check-backup-s3.rb` → `check-backup-s3`, etc.
- `check-backup-ftp`: the host shorthand is now `-H` (`-h` is `--help`).
- `check-backup-s3`: `--s3-endpoint` and `--s3-region` are required (they defaulted to AWS `eu-west-3`).
- `check-backup-s3` / `check-backup-ftp`: `--directory-name` is required (it defaulted to `/var/archives`); the default now lives in the check definition.
- `check-backup-restic`: `--env-file` is required (it defaulted to `/etc/restic/env`); the default now lives in the check definition.
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

- one archive per architecture listed in `ARCHS` (`linux/amd64`) in Sensu asset layout (`bin/check-backup-*`);
- the sha512 checksums file;
- `opsone-sensu-plugins-backup_<version>.yml`, the asset definition pointing to the GitHub release of the tag.

It then creates the GitHub release of the tag with `gh`, uploads the archives and checksums, and uses the asset definition as release description.

`check-backup-s3 version` prints the version, commit and build date.
