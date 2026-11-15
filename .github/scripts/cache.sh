#!/usr/bin/env bash
# Keeps the caches that runs on main save down to what the run used, and keeps
# one copy of each.
#
#   cache.sh mark DIR         after restoring DIR: mark every restored file unused
#   cache.sh trim DIR         before saving DIR: delete the files this run never used
#   cache.sh prune PREFIX KEY after saving KEY: delete main's other caches named PREFIX*
#
# Go and Bazel both set a cache entry's mtime when they use it (Go only when the
# mtime is over an hour old), so `mark` sets every restored file two hours back
# and `trim` deletes the files still that old. Two hours is past Go's one-hour
# interval and well short of the five days after which Go trims an entry itself.
#
# Without this both caches only grow: Bazel's own disk-cache GC runs only after
# the server has idled for minutes, which never happens in CI, and Go trims only
# entries unused for five days.
set -euo pipefail

cmd=$1
dir=${2:-}
if [ "$cmd" != prune ] && command -v cygpath >/dev/null; then
  dir=$(cygpath -u "$dir") # Windows: go env GOCACHE gives a Windows path
fi
stamp="${RUNNER_TEMP:-/tmp}/cache-mark-$(printf %s "$dir" | cksum | cut -d' ' -f1)"

size() { du -sh "$1" 2>/dev/null | cut -f1; }

case "$cmd" in
mark)
  mkdir -p "$dir"
  find "$dir" -type f -exec perl -e '$t = time - 7200; utime $t, $t, @ARGV' {} +
  touch "$stamp"
  echo "restored $(size "$dir") in $dir"
  ;;
trim)
  if [ ! -e "$stamp" ]; then echo "no mark for $dir; keeping it whole"; exit 0; fi
  before=$(size "$dir")
  find "$dir" -type f ! -newer "$stamp" -delete
  echo "trimmed $dir from $before to $(size "$dir")"
  ;;
prune)
  prefix=$2 key=$3
  gh cache list --ref refs/heads/main --key "$prefix" --limit 100 --json id,key \
    --jq ".[] | select(.key != \"$key\") | \"\(.id) \(.key)\"" |
    while read -r id old; do
      echo "deleting $old, superseded by $key"
      gh cache delete "$id"
    done
  ;;
*)
  echo "usage: cache.sh mark DIR | trim DIR | prune PREFIX KEY" >&2
  exit 2
  ;;
esac
