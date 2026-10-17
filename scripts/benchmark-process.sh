#!/usr/bin/env bash
# Read-only Linux /proc benchmark for a running Docktop binary.
# CPU percentages measure one logical core as 100%, not whole-host utilization.
set -euo pipefail

usage() { echo "Usage: $0 DOCKTOP_PID [DURATION_SECONDS=30] [INTERVAL_SECONDS=2]" >&2; exit 2; }
[[ $# -ge 1 && $# -le 3 ]] || usage
pid="$1"; duration="${2:-30}"; interval="${3:-2}"
for number in "$pid" "$duration" "$interval"; do
  [[ "$number" =~ ^[0-9]+$ ]] || usage
done
(( pid > 1 && duration >= 2 && duration <= 300 && interval >= 1 && interval <= 10 && interval <= duration )) || usage
[[ -r "/proc/$pid/status" && -r "/proc/$pid/stat" ]] || { echo "Cannot access PID $pid in /proc" >&2; exit 1; }
process_name="$(sed -n 's/^Name:[[:space:]]*//p' "/proc/$pid/status")"
[[ "$process_name" == "docktop" ]] || { echo "Refusing to measure unrelated process: $process_name" >&2; exit 1; }
hz="$(getconf CLK_TCK)"
[[ "$hz" =~ ^[0-9]+$ ]] && (( hz > 0 )) || { echo "Invalid clock tick frequency" >&2; exit 1; }

# Parse /proc/[pid]/stat after the final ') ' so process comm cannot shift fields.
read_sample() {
  local stat tail
  [[ -r "/proc/$pid/stat" && -r "/proc/$pid/status" ]] || return 1
  IFS= read -r stat < "/proc/$pid/stat" || return 1
  [[ "$stat" == *') '* ]] || return 1
  tail="${stat##*) }"
  local -a fields
  read -r -a fields <<< "$tail"
  (( ${#fields[@]} >= 20 )) || return 1
  # fields[11]/[12] are utime/stime, fields[19] is process starttime.
  [[ "${fields[11]}" =~ ^[0-9]+$ && "${fields[12]}" =~ ^[0-9]+$ && "${fields[19]}" =~ ^[0-9]+$ ]] || return 1
  ticks=$((fields[11] + fields[12]))
  start_time="${fields[19]}"
  rss_kib="$(awk '$1 == "VmRSS:" {print $2; exit}' "/proc/$pid/status")"
  [[ "$rss_kib" =~ ^[0-9]+$ ]] || return 1
  time_ns="$(date +%s%N)"
  [[ "$time_ns" =~ ^[0-9]+$ ]] || return 1
}

read_sample || { echo "Failed to sample Docktop process" >&2; exit 1; }
initial_start="$start_time"
previous_ticks="$ticks"; previous_time_ns="$time_ns"
min_rss="$rss_kib"; max_rss="$rss_kib"; sum_rss=0; sum_cpu_tenths=0; samples=0
elapsed=0
printf 'Docktop PID %s | duration %ss | interval %ss | CPU: one core = 100%%\n' "$pid" "$duration" "$interval"
printf '%-9s %12s %12s\n' 'elapsed' 'RSS(MiB)' 'CPU(%)'
while (( elapsed < duration )); do
  pause="$interval"
  if (( duration - elapsed < pause )); then pause=$((duration - elapsed)); fi
  sleep "$pause"
  elapsed=$((elapsed + pause))
  read_sample || { echo "Docktop exited or /proc became inaccessible" >&2; exit 1; }
  [[ "$start_time" == "$initial_start" ]] || { echo "PID was reused; aborting" >&2; exit 1; }
  delta_ns=$((time_ns - previous_time_ns))
  delta_ticks=$((ticks - previous_ticks))
  (( delta_ns > 0 && delta_ticks >= 0 )) || { echo "Nonmonotonic counters; aborting" >&2; exit 1; }
  # CPU percentage times ten, using 64-bit integer arithmetic.
  cpu_tenths=$((delta_ticks * 1000000000000 / (hz * delta_ns)))
  printf '%-9s %8d.%d    %8d.%d\n' "${elapsed}s" "$((rss_kib/1024))" "$(((rss_kib%1024)*10/1024))" "$((cpu_tenths/10))" "$((cpu_tenths%10))"
  (( rss_kib < min_rss )) && min_rss=$rss_kib
  (( rss_kib > max_rss )) && max_rss=$rss_kib
  sum_rss=$((sum_rss + rss_kib))
  sum_cpu_tenths=$((sum_cpu_tenths + cpu_tenths))
  samples=$((samples + 1))
  previous_ticks="$ticks"; previous_time_ns="$time_ns"
done
(( samples > 0 )) || { echo "No samples collected" >&2; exit 1; }
avg_rss=$((sum_rss/samples)); avg_cpu_tenths=$((sum_cpu_tenths/samples))
printf '\nSummary (%d intervals; includes Docktop only, not Docker workloads or child processes)\n' "$samples"
printf 'RSS MiB: min %d.%d | avg %d.%d | peak %d.%d\n' "$((min_rss/1024))" "$(((min_rss%1024)*10/1024))" "$((avg_rss/1024))" "$(((avg_rss%1024)*10/1024))" "$((max_rss/1024))" "$(((max_rss%1024)*10/1024))"
printf 'CPU: mean %d.%d%% of one logical core across intervals\n' "$((avg_cpu_tenths/10))" "$((avg_cpu_tenths%10))"