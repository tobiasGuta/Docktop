#!/usr/bin/env bash
# Read-only memory/CPU snapshot of an ALREADY running Docktop process.
set -euo pipefail
if [[ $# -ne 1 || ! "$1" =~ ^[0-9]+$ ]]; then
  echo "Usage: $0 DOCKTOP_PID" >&2
  exit 2
fi
pid="$1"
[[ -r "/proc/$pid/status" ]] || { echo "PID not accessible" >&2; exit 1; }
name="$(sed -n 's/^Name:[[:space:]]*//p' "/proc/$pid/status")"
if [[ "$name" != "docktop" ]]; then
  echo "Refusing to benchmark unrelated process '$name'" >&2
  exit 1
fi
printf 'PID    RSS(KiB)   CPU(%%)  ELAPSED\n'
for i in 1 2 3; do
  [[ -r "/proc/$pid/status" ]] || break
  rss="$(awk '/^VmRSS:/ {print $2}' "/proc/$pid/status")"
  ps -p "$pid" -o pid=,pcpu=,etime= | awk -v rss="${rss:-unknown}" '{printf "%s  %-10s %-6s %s\n",$1,rss,$2,$3}'
  if (( i < 3 )); then sleep 2; fi
done
