"""Read-only Prometheus resource summary for a UTC time window."""

import argparse
import datetime as dt
import json
from pathlib import Path
import urllib.parse
import urllib.request

BASE = "http://127.0.0.1:19090/api/v1/query_range"
parser = argparse.ArgumentParser()
parser.add_argument("--start", default="2026-11-13T21:32:00+00:00")
parser.add_argument("--end", default="2026-11-13T21:45:00+00:00")
parser.add_argument("--output", help="Optional JSON file for the sampled series")
args = parser.parse_args()
START = dt.datetime.fromisoformat(args.start).timestamp()
END = dt.datetime.fromisoformat(args.end).timestamp()
JOB = 'job="ai-buddy-load-backend"'
QUERIES = {
    "rss_mib": f"process_resident_memory_bytes{{{JOB}}} / 1048576",
    "heap_sys_mib": f"go_memstats_heap_sys_bytes{{{JOB}}} / 1048576",
    "heap_released_mib": f"go_memstats_heap_released_bytes{{{JOB}}} / 1048576",
    "heap_alloc_mib": f"go_memstats_heap_alloc_bytes{{{JOB}}} / 1048576",
    "cpu_cores": f"rate(process_cpu_seconds_total{{{JOB}}}[15s])",
    "goroutines": f"go_goroutines{{{JOB}}}",
    "up": f"up{{{JOB}}}",
    "login_rps": f'rate(http_requests_total{{{JOB},route="POST /api/auth/login"}}[15s])',
}

snapshot = {"start_utc": args.start, "end_utc": args.end, "step_seconds": 5, "series": {}}
for name, query in QUERIES.items():
    url = BASE + "?" + urllib.parse.urlencode({"query": query, "start": START, "end": END, "step": "5s"})
    with urllib.request.urlopen(url, timeout=10) as response:
        rows = json.load(response)["data"]["result"]
    values = [(dt.datetime.fromtimestamp(float(t), dt.timezone.utc).isoformat(), float(v))
              for row in rows for t, v in row["values"]]
    snapshot["series"][name] = {"query": query, "values": values}
    if not values:
        print(name, "no data")
        continue
    peak = max(values, key=lambda pair: pair[1])
    print(name, "points", len(values), "min", round(min(v for _, v in values), 3),
          "max", round(peak[1], 3), "at", peak[0], "first", values[0], "last", values[-1])
if args.output:
    path = Path(args.output)
    if path.exists():
        raise SystemExit(f"refusing to overwrite {path}")
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(snapshot, indent=2), encoding="utf-8")
    print("saved", path)
