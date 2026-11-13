"""Plot completed load runs from local k6 and Prometheus report files."""

import json
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
REPORTS = ROOT / "load-results"
COLORS = {"public": "#2563eb", "read": "#ea580c", "mixed": "#16a34a"}


def load_runs():
    runs = []
    for path in sorted(REPORTS.glob("*/result.json")):
        manifest_path = path.with_name("manifest.json")
        series_path = path.with_name("server-series.json")
        if not manifest_path.exists() or not series_path.exists():
            continue
        manifest = json.loads(manifest_path.read_text(encoding="utf-8-sig"))
        if manifest.get("status") != "complete" or manifest.get("duration") != "3m":
            continue
        result = json.loads(path.read_text(encoding="utf-8-sig"))
        series = json.loads(series_path.read_text(encoding="utf-8-sig"))

        def mean_series(name):
            values = [float(value) for row in series[name]["data"] for _, value in row["values"]]
            return sum(values) / len(values) if values else None

        runs.append({
            "id": manifest["run_id"],
            "scenario": manifest["scenario"],
            "vus": manifest["vus"],
            "rps": result["client"]["rps"],
            "p95_ms": result["client"]["latency_p95_ms"],
            "cpu_cores": mean_series("backend_cpu_cores"),
            "ram_mib": mean_series("backend_ram_mib"),
        })
    return runs


def main():
    runs = load_runs()
    width, height = 980, 940
    panels = [
        ("Client RPS (k6, phase=workload)", "rps", "req/s"),
        ("Client latency p95 (k6, phase=workload)", "p95_ms", "ms"),
        ("Backend process CPU (Prometheus, mean)", "cpu_cores", "cores"),
        ("Backend resident RAM (Prometheus, mean)", "ram_mib", "MiB"),
    ]
    out = [f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {width} {height}">',
           '<rect width="100%" height="100%" fill="white"/>',
           '<style>text{font-family:Arial,sans-serif;fill:#172033}.small{font-size:13px}.title{font-size:17px;font-weight:bold}</style>',
           '<text x="48" y="32" class="title">AI-Buddy load series — completed runs only</text>',
           '<text x="48" y="52" class="small">Read 50 VU failed during login; read 100 and all mixed runs were not started.</text>']
    for index, (title, key, unit) in enumerate(panels):
        top = 76 + index * 211
        left, right, bottom = 92, 862, top + 150
        available = [run[key] for run in runs if run[key] is not None]
        maximum = max(available + [1]) * 1.1
        out += [f'<text x="48" y="{top + 7}" class="title">{title}</text>',
                f'<path d="M {left} {top + 25} V {bottom} H {right}" fill="none" stroke="#64748b"/>',
                f'<text x="48" y="{top + 33}" class="small">{maximum:.1f}</text>',
                f'<text x="60" y="{bottom + 4}" class="small">0</text>',
                f'<text x="{right + 10}" y="{bottom + 4}" class="small">VU</text>',
                f'<text x="{right + 10}" y="{top + 33}" class="small">{unit}</text>']
        for vu in (10, 50, 100):
            x = left + (vu / 100) * (right - left)
            out.append(f'<text x="{x - 12:.1f}" y="{bottom + 19}" class="small">{vu}</text>')
        for scenario, color in COLORS.items():
            points = sorted((run for run in runs if run["scenario"] == scenario and run[key] is not None), key=lambda run: run["vus"])
            coords = [(left + run["vus"] / 100 * (right - left), bottom - run[key] / maximum * (bottom - top - 25), run[key]) for run in points]
            if len(coords) > 1:
                out.append('<polyline points="' + ' '.join(f'{x:.1f},{y:.1f}' for x, y, _ in coords) + f'" fill="none" stroke="{color}" stroke-width="2"/>')
            for x, y, value in coords:
                out += [f'<circle cx="{x:.1f}" cy="{y:.1f}" r="5" fill="{color}"/>',
                        f'<text x="{x + 7:.1f}" y="{y - 7:.1f}" class="small">{value:.2f}</text>']
    out += ['<text x="48" y="932" class="small">Blue: public  |  Orange: read  |  Green: mixed (no completed runs)</text>', '</svg>']
    destination = REPORTS / "series-partial.svg"
    destination.write_text("\n".join(out), encoding="utf-8")
    print(f"Wrote {destination.relative_to(ROOT)} from {len(runs)} completed 3m runs")


if __name__ == "__main__":
    main()
