#!/usr/bin/env bash
# Runs the checks against a real S3 gateway (versitygw), FTP server and restic repository in Docker.
set -euo pipefail
cd "$(dirname "$0")"

id=$$
image=check-backup-integration
network=check-backup-$id
s3host=check-backup-s3-$id
runner=check-backup-runner-$id
s3_image=versity/versitygw:v1.10.0

docker build -q -t "$image" . >/dev/null
docker network create "$network" >/dev/null
cleanup() {
  docker rm -f "$runner" "$s3host" >/dev/null 2>&1 || true
  docker network rm "$network" >/dev/null 2>&1 || true
}
trap cleanup EXIT

docker run -d --name "$s3host" --network "$network" --tmpfs /data "$s3_image" \
  --access sensu --secret sensu-secret --port :9000 posix /data >/dev/null
docker run -d --name "$runner" --network "$network" "$image" >/dev/null

run() { docker exec "$runner" bash -c "$1"; }

# Local archives: two files, plus a .md5, a hidden file and a subdirectory that must be ignored.
run 'mkdir -p /var/archives/sub && head -c 100 /dev/urandom > /var/archives/db.sql.gz &&
     head -c 200 /dev/urandom > /var/archives/files.tar.gz && echo x > /var/archives/db.sql.gz.md5 &&
     echo x > /var/archives/.lock && chmod -R a+rX /var/archives'

# FTP server on port 2121, archives in the "backups" directory.
run 'mkdir -p /srv/ftp/backups && chown -R nobody /srv/ftp &&
     (runuser -u nobody -- python3 -m pyftpdlib -p 2121 -d /srv/ftp -u sensu -P sensu-secret -w >/tmp/ftp.log 2>&1 &)'

# S3: curl signs the requests itself, no client needed.
s3() { run "curl -sf --aws-sigv4 aws:amz:us-east-1:s3 -u sensu:sensu-secret $1"; }
for _ in $(seq 30); do s3 "-X PUT http://$s3host:9000/backups" && break; sleep 1; done

# restic repository with one snapshot of the archives.
run 'mkdir -p /etc/restic && printf "export RESTIC_REPOSITORY=/srv/restic\nexport RESTIC_PASSWORD=sensu-secret\n" > /etc/restic/env &&
     export RESTIC_REPOSITORY=/srv/restic RESTIC_PASSWORD=sensu-secret RESTIC_CACHE_DIR=/tmp/restic-cache &&
     restic init -q && restic backup -q /var/archives && chmod -R a+rwX /srv/restic /etc/restic'

failures=0
expect() {
  local want=$1 desc=$2; shift 2
  local out rc=0
  # Run as an unprivileged user, like sensu-agent does.
  out=$(docker exec -e HOME=/tmp "$runner" runuser -u nobody -- "$@" 2>&1) || rc=$?
  out=$(tail -1 <<<"$out")
  if [[ $rc -eq $want ]]; then
    echo "ok   [$rc] $desc -> $out"
  else
    echo "FAIL [$rc, want $want] $desc -> $out"
    failures=$((failures + 1))
  fi
}

s3args=(check-backup-s3 -e "http://$s3host:9000" -r us-east-1 -a sensu -k sensu-secret -b backups -d /var/archives)
expect 3 "s3 without bucket" check-backup-s3
expect 3 "s3 without local directory" "${s3args[@]}" -d ""
expect 2 "s3 empty bucket" "${s3args[@]}"
s3 "-T /var/archives/db.sql.gz http://$s3host:9000/backups/db.sql.gz"
expect 2 "s3 one file missing" "${s3args[@]}"
run 'head -c 150 /dev/urandom > /tmp/short' && s3 "-T /tmp/short http://$s3host:9000/backups/files.tar.gz"
expect 2 "s3 size mismatch" "${s3args[@]}"
s3 "-T /var/archives/files.tar.gz http://$s3host:9000/backups/files.tar.gz"
expect 0 "s3 all present" "${s3args[@]}"
expect 2 "s3 wrong secret" "${s3args[@]}" -k wrong
expect 2 "s3 unknown bucket" "${s3args[@]}" -b nope

ftpargs=(check-backup-ftp -H 127.0.0.1:2121 -u sensu -p sensu-secret -D backups -d /var/archives)
expect 3 "ftp without host" check-backup-ftp
expect 3 "ftp without local directory" "${ftpargs[@]}" -d ""
expect 2 "ftp empty directory" "${ftpargs[@]}"
run 'cp /var/archives/db.sql.gz /srv/ftp/backups/ && head -c 10 /dev/urandom > /srv/ftp/backups/files.tar.gz'
expect 2 "ftp size mismatch" "${ftpargs[@]}"
run 'cp /var/archives/files.tar.gz /srv/ftp/backups/'
expect 0 "ftp all present" "${ftpargs[@]}"
expect 2 "ftp wrong password" "${ftpargs[@]}" -p wrong
expect 2 "ftp unknown directory" "${ftpargs[@]}" -D nope
expect 2 "ftp unreachable" check-backup-ftp -H 127.0.0.1:1 -d /var/archives

expect 2 "restic missing env file" check-backup-restic -e /nope
expect 3 "restic invalid subset" check-backup-restic --read-data-subset 10
expect 3 "restic exclusive options" check-backup-restic --read-data --read-data-subset 1/2
expect 0 "restic quick" check-backup-restic
expect 0 "restic subset" check-backup-restic --read-data-subset 50%
expect 0 "restic full" check-backup-restic --read-data
run 'f=$(find /srv/restic/data -type f | head -1) && chmod u+w "$f" && printf corrupt | dd of="$f" bs=1 seek=50 conv=notrunc status=none'
expect 2 "restic full on corrupted repo" check-backup-restic --read-data
run 'sed -i s/sensu-secret/wrong/ /etc/restic/env'
expect 2 "restic wrong password" check-backup-restic

exit $failures
