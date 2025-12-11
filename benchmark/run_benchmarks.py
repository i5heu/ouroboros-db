#!/usr/bin/env python3
import json
import os
import shutil
import subprocess
import sys
import time
from pathlib import Path
from statistics import median, mean, stdev
from typing import Dict, List, Optional
import math


# -------------------------------------------------------------------
# Verbose logging helpers
# -------------------------------------------------------------------
VERBOSE = os.environ.get("VERBOSE", "0") not in ("0", "", "false", "False", "no", "No")


def log(msg: str) -> None:
    """Always logged (and flushed)."""
    print(msg, flush=True)


def vlog(msg: str) -> None:
    """Verbose-only logging (and flushed)."""
    if VERBOSE:
        print(f"[verbose] {msg}", flush=True)


def run(cmd, cwd=None, capture_output=False, text=True):
    """Run a command and fail hard on error."""
    vlog(f"Running command: {' '.join(cmd)} (cwd={cwd})")
    start = time.time()
    result = subprocess.run(
        cmd, cwd=cwd, capture_output=capture_output, text=text, check=False
    )
    duration = time.time() - start
    vlog(f"Command finished in {duration:.3f}s, exit={result.returncode}")
    if capture_output:
        vlog(f"stdout:\n{result.stdout}")
        vlog(f"stderr:\n{result.stderr}")
    if result.returncode != 0:
        if capture_output:
            sys.stderr.write(result.stderr or "")
            sys.stderr.write(result.stdout or "")
        raise RuntimeError(
            f"Command failed with exit code {result.returncode}: {' '.join(cmd)}"
        )
    return result


def get_env(name, default=None, type_=str):
    val = os.environ.get(name, default)
    if type_ is int and val is not None:
        return int(val)
    return val


def copy_repo(src: Path, dst: Path) -> None:
    """Copy repo from src to dst (fresh), including .git."""
    if dst.exists():
        vlog(f"Removing existing work dir: {dst}")
        shutil.rmtree(dst)
    log(f"📁 Copying repo from {src} to {dst}…")
    shutil.copytree(src, dst, symlinks=True)
    log("✅ Repo copy finished.")


def collect_tags(repo_dir: Path, num_versions: int) -> List[str]:
    vlog("Collecting tags with: git tag --sort=-creatordate")
    result = run(
        ["git", "tag", "--sort=-creatordate"], cwd=repo_dir, capture_output=True
    )
    tags = [line.strip() for line in result.stdout.splitlines() if line.strip()]
    vlog(f"Found tags: {tags}")
    return tags[:num_versions]


# -------------------------------------------------------------------
# Metadata parsing (goos/goarch/pkg/cpu)
# -------------------------------------------------------------------
def extract_meta_line(line: str, meta: dict) -> None:
    line = line.strip()
    if line.startswith("goos: "):
        meta["goos"] = line[len("goos: ") :].strip()
    elif line.startswith("goarch: "):
        meta["goarch"] = line[len("goarch: ") :].strip()
    elif line.startswith("pkg: "):
        meta["pkg"] = line[len("pkg: ") :].strip()
    elif line.startswith("cpu: "):
        meta["cpu"] = line[len("cpu: ") :].strip()


# -------------------------------------------------------------------
# Parsing benchmark output
# -------------------------------------------------------------------
def parse_benchmark_output_line(line: str):
    """
    Parse a single 'Benchmark...' output line from go test.

    Example:
    BenchmarkFoo-8    12345 ns/op   6789 B/op   10 allocs/op
    """
    line = line.strip()
    if not line.startswith("Benchmark"):
        return None
    parts = line.split()
    if len(parts) < 3:
        return None

    name = parts[0]
    ns: Optional[float] = None
    bytes_op: Optional[float] = None
    allocs_op: Optional[float] = None

    def parse_time_with_unit(value: str, unit: str) -> float:
        x = float(value)
        if unit == "ns/op":
            return x
        if unit in ("µs/op", "us/op"):
            return x * 1_000.0
        if unit == "ms/op":
            return x * 1_000_000.0
        if unit == "s/op":
            return x * 1_000_000_000.0
        return x

    i = 1
    while i < len(parts):
        token = parts[i]
        if token in ("ns/op", "µs/op", "us/op", "ms/op", "s/op") and i >= 1:
            ns = parse_time_with_unit(parts[i - 1], token)
        if token == "B/op" and i >= 1:
            try:
                bytes_op = float(parts[i - 1])
            except ValueError:
                pass
        if token == "allocs/op" and i >= 1:
            try:
                allocs_op = float(parts[i - 1])
            except ValueError:
                pass
        i += 1

    if ns is None:
        return None

    return {
        "name": name,
        "ns_per_op": ns,
        "bytes_per_op": bytes_op,
        "allocs_per_op": allocs_op,
    }


def go_bench_collect(
    repo_dir: Path,
    version_label: str,
    bench_pkg_pattern: str,
    bench_filter: str,
    bench_count: int,
    bench_time: str,
    samples: Dict[str, Dict[str, Dict[str, List[float]]]],
    meta: dict,
) -> None:
    """
    Run go test -bench with -json and collect samples into 'samples'.

    samples structure:
    {
      benchmark_name: {
        version_label: {
          "ns": [ ... ],
          "bytes": [ ... ],
          "allocs": [ ... ],
        },
        ...
      },
      ...
    }
    """
    cmd = [
        "go",
        "test",
        bench_pkg_pattern,
        "-run=^$",
        f"-bench={bench_filter}",
        f"-benchtime={bench_time}",
        f"-count={bench_count}",
        "-json",
    ]

    log(f"⏱  go test (json) for {version_label} started…")
    start = time.time()
    proc = subprocess.run(
        cmd, cwd=repo_dir, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True
    )
    duration = time.time() - start
    log(
        f"✅ go test (json) for {version_label} finished in {duration:.2f}s (exit={proc.returncode})"
    )

    if proc.returncode != 0:
        sys.stderr.write(proc.stdout or "")
        raise RuntimeError(f"go test benchmark failed for {version_label}")

    for line in proc.stdout.splitlines():
        extract_meta_line(line, meta)  # metadata may appear outside JSON
        line = line.strip()
        if not line:
            continue

        try:
            rec = json.loads(line)
        except json.JSONDecodeError:
            continue

        if rec.get("Action") != "output":
            continue

        out = rec.get("Output", "")
        for sub in out.splitlines():
            extract_meta_line(sub, meta)
            if not sub.startswith("Benchmark"):
                continue
            parsed = parse_benchmark_output_line(sub)
            if not parsed:
                continue

            bname = parsed["name"]
            ns = parsed["ns_per_op"]
            b = parsed["bytes_per_op"]
            a = parsed["allocs_per_op"]

            bench_entry = samples.setdefault(bname, {})
            ver_entry = bench_entry.setdefault(
                version_label, {"ns": [], "bytes": [], "allocs": []}
            )
            ver_entry["ns"].append(ns)
            if b is not None:
                ver_entry["bytes"].append(b)
            if a is not None:
                ver_entry["allocs"].append(a)


# -------------------------------------------------------------------
# Stats helpers
# -------------------------------------------------------------------
def ns_to_human(ns: float) -> str:
    if ns >= 1e9:
        return f"{ns / 1e9:.3f}s"
    if ns >= 1e6:
        return f"{ns / 1e6:.3f}ms"
    if ns >= 1e3:
        return f"{ns / 1e3:.3f}µs"
    return f"{ns:.1f}ns"


def bytes_to_human(b: Optional[float]) -> str:
    if b is None:
        return "n/a"
    if b >= 1024**2:
        return f"{b / 1024**2:.2f}MiB"
    if b >= 1024:
        return f"{b / 1024:.2f}KiB"
    return f"{b:.0f}B"


def allocs_to_human(a: Optional[float]) -> str:
    if a is None:
        return "n/a"
    if a >= 1000:
        return f"{a/1000:.3f}k"
    return f"{a:.0f}"


def slugify(s: str) -> str:
    return (
        s.replace("/", "_")
        .replace(" ", "_")
        .replace("\t", "_")
        .replace(":", "_")
        .replace(".", "_")
    )


def format_delta_pct(delta: Optional[float]) -> str:
    if delta is None:
        return "n/a"
    if abs(delta) < 0.005:
        return "0.0%"
    sign = "+" if delta >= 0 else ""
    return f"{sign}{delta:.1f}%"


def format_pct(value: Optional[float]) -> str:
    """Format percentage without sign, for noise etc."""
    if value is None:
        return "n/a"
    return f"{value:.1f}%"


def format_ci_pct(value: Optional[float]) -> str:
    """Format CI width as ±X%."""
    if value is None:
        return "n/a"
    return f"±{value:.1f}%"


# -------------------------------------------------------------------
# HTML generation (using external template)
# -------------------------------------------------------------------
def generate_html(html_path: Path, samples: dict, template_path: Path, meta: dict) -> None:
    """
    Generate HTML report using an external template.

    Template must contain placeholders:
      __GENERATED_AT__
      __META_BLOCK__
      __BENCH_SECTIONS_HTML__
      __BENCH_DATA_JSON__
    """
    if not template_path.is_file():
        raise SystemExit(
            f"HTML template not found at {template_path}. "
            f"Make sure benchmark/bench_template.html exists and is mounted."
        )

    html_path.parent.mkdir(parents=True, exist_ok=True)

    bench_data = {}
    sections_html_parts = []

    import html as html_mod

    for bench_name in sorted(samples.keys()):
        bench_slug = slugify(bench_name)
        original_versions = samples[bench_name]

        # Enforce version order: current | newest → oldest
        all_keys = list(original_versions.keys())
        other_versions = [v for v in all_keys if v != "current"]
        other_versions_sorted = sorted(other_versions, reverse=True)

        ordered_keys: List[str] = []
        if "current" in original_versions:
            ordered_keys.append("current")
        ordered_keys.extend(other_versions_sorted)

        versions = {k: original_versions[k] for k in ordered_keys}

        # For JS plot (raw samples passed through)
        bench_data[bench_name] = {
            "id": bench_slug,
            "versions": versions,
        }

        # Medians and current baseline
        medians_ns: Dict[str, Optional[float]] = {}
        for v in ordered_keys:
            ns_list = versions[v].get("ns", [])
            medians_ns[v] = median(ns_list) if ns_list else None
        med_current = medians_ns.get("current")

        section = []
        section.append(f'<section id="sec-{bench_slug}">')
        subtitle = (
            'Boxplot per version. Lower is faster. Baseline for Δ is <code>current</code>.'
        )
        section.append(f'  <h2>{html_mod.escape(bench_name)}</h2>')
        section.append(f'  <div class="subtitle">{subtitle}</div>')

        section.append('  <table class="summary-table">')
        section.append(
            '    <thead><tr>'
            '<th>Version</th>'
            '<th>Median ns/op (±95% CI)</th>'
            '<th>Δ vs current</th>'
            '<th>Noise σ/μ</th>'
            '<th>95% CI width</th>'
            '<th>Samples</th>'
            '<th>Bytes/op</th>'
            '<th>Allocs/op</th>'
            '</tr></thead>'
        )
        section.append('    <tbody>')

        for version in ordered_keys:
            metrics = versions[version]
            ns_list = metrics.get("ns", [])
            b_list = metrics.get("bytes", [])
            a_list = metrics.get("allocs", [])

            n = len(ns_list)
            med_ns = medians_ns.get(version)

            mean_ns = None
            std_ns = None
            noise_pct = None
            ci_ns = None
            ci_pct = None

            if n >= 2:
                mean_ns = mean(ns_list)
                std_ns = stdev(ns_list)
                if mean_ns > 0:
                    noise_pct = (std_ns / mean_ns) * 100.0
                    se = std_ns / math.sqrt(n)
                    ci_ns = 1.96 * se
                    ci_pct = (ci_ns / mean_ns) * 100.0

            if med_ns is not None:
                ns_med_str = ns_to_human(med_ns)
                if ci_ns is not None:
                    ci_ns_str = ns_to_human(ci_ns)
                    med_display = f"{ns_med_str} ± {ci_ns_str}"
                else:
                    med_display = ns_med_str
                if med_current is not None and med_current > 0:
                    delta_curr = (med_ns / med_current - 1.0) * 100.0
                    delta_curr_str = format_delta_pct(delta_curr)
                else:
                    delta_curr_str = "n/a"
            else:
                med_display = "n/a"
                delta_curr_str = "n/a"

            noise_str = format_pct(noise_pct)
            ci_pct_str = format_ci_pct(ci_pct)

            b_med_raw = median(b_list) if b_list else None
            a_med_raw = median(a_list) if a_list else None
            b_med = bytes_to_human(b_med_raw)
            a_med = allocs_to_human(a_med_raw)

            section.append(
                "      <tr>"
                f"<td class=\"version\">{html_mod.escape(version)}</td>"
                f"<td>{html_mod.escape(med_display)}</td>"
                f"<td>{html_mod.escape(delta_curr_str)}</td>"
                f"<td>{html_mod.escape(noise_str)}</td>"
                f"<td>{html_mod.escape(ci_pct_str)}</td>"
                f"<td>{n}</td>"
                f"<td>{html_mod.escape(b_med)}</td>"
                f"<td>{html_mod.escape(a_med)}</td>"
                "</tr>"
            )

        section.append("    </tbody>")
        section.append("  </table>")
        section.append(f'  <div id="plot-{bench_slug}" class="plot"></div>')
        section.append("</section>")

        sections_html_parts.append("\n".join(section))

    sections_html = "\n\n".join(sections_html_parts)
    data_json = json.dumps(bench_data)
    generated_at = time.strftime("%Y-%m-%d %H:%M:%S", time.localtime())

    # Meta block
    meta_lines = []
    if meta.get("goos"):
        meta_lines.append(f"goos: {meta['goos']}")
    if meta.get("goarch"):
        meta_lines.append(f"goarch: {meta['goarch']}")
    if meta.get("pkg"):
        meta_lines.append(f"pkg: {meta['pkg']}")
    if meta.get("cpu"):
        meta_lines.append(f"cpu: {meta['cpu']}")
    meta_block = "<br>\n    ".join(meta_lines) + "<br>" if meta_lines else ""

    template = template_path.read_text(encoding="utf-8")
    doc = (
        template.replace("__GENERATED_AT__", generated_at)
        .replace("__META_BLOCK__", meta_block)
        .replace("__BENCH_SECTIONS_HTML__", sections_html)
        .replace("__BENCH_DATA_JSON__", data_json)
    )
    html_path.write_text(doc, encoding="utf-8")
    log(f"💾 HTML report written to: {html_path}")


# -------------------------------------------------------------------
# Main logic
# -------------------------------------------------------------------
def main() -> None:
    repo_src = Path(get_env("REPO_SRC", "/repo-src"))
    repo_dir = Path(get_env("REPO_DIR", "/repo-work"))
    html_output = Path(get_env("HTML_OUTPUT", "/results-html/benchmarks.html"))
    html_template = Path(get_env("HTML_TEMPLATE", "/benchmark/bench_template.html"))

    num_versions = get_env("NUM_VERSIONS", 5, int)
    bench_pkg_pattern = get_env("BENCH_PKG_PATTERN", "./...")
    bench_filter = get_env("BENCH_FILTER", ".")
    bench_count = get_env("BENCH_COUNT", 8, int)
    bench_time = get_env("BENCH_TIME", "1s")

    log("🚀 Starting OuroborosDB benchmark runner")
    log("----------------------------------------")
    log("📦 Source repo (host):  " + str(repo_src))
    log("📦 Work repo (copy):    " + str(repo_dir))
    log("📄 HTML output:         " + str(html_output))
    log("📄 HTML template:       " + str(html_template))
    log("🔢 Versions to test:    " + str(num_versions))
    log("🏁 go test pkg pattern: " + str(bench_pkg_pattern))
    log("🔍 Benchmark filter:    " + str(bench_filter))
    log("🔁 Bench count:         " + str(bench_count))
    log("⏱  Bench time:          " + str(bench_time))
    log("🔊 Verbose mode:        " + ("ON" if VERBOSE else "OFF"))
    log("")

    if not repo_src.is_dir():
        raise SystemExit(
            f"Source repo directory {repo_src} does not exist in container.\n"
            f"Did you mount your repo to {repo_src}?"
        )

    copy_repo(repo_src, repo_dir)

    # Current commit
    log("📍 Detecting current commit in work copy…")
    current_ref_res = run(["git", "rev-parse", "HEAD"], cwd=repo_dir, capture_output=True)
    current_ref = current_ref_res.stdout.strip()
    log(f"   → {current_ref}\n")

    # Collect tags
    log(f"🧷 Collecting last {num_versions} tags…")
    tags = collect_tags(repo_dir, num_versions)
    if tags:
        for t in tags:
            log(f"   • {t}")
    else:
        log("⚠️  No tags found.")
    log("")

    samples: Dict[str, Dict[str, Dict[str, List[float]]]] = {}
    meta: dict = {}

    # Benchmark historical tags
    for tag in tags:
        log(f"=== Benchmarking tag {tag} ===")
        run(["git", "checkout", "--quiet", tag], cwd=repo_dir)
        go_bench_collect(
            repo_dir, tag, bench_pkg_pattern, bench_filter, bench_count, bench_time, samples, meta
        )
        log("")

    # Benchmark current commit
    log(f"=== Benchmarking current commit {current_ref} ===")
    run(["git", "checkout", "--quiet", current_ref], cwd=repo_dir)
    go_bench_collect(
        repo_dir, "current", bench_pkg_pattern, bench_filter, bench_count, bench_time, samples, meta
    )
    log("")

    # CLI meta summary
    if meta:
        log("📋 Environment:")
        if "goos" in meta:
            log(f"  goos:   {meta['goos']}")
        if "goarch" in meta:
            log(f"  goarch: {meta['goarch']}")
        if "pkg" in meta:
            log(f"  pkg:    {meta['pkg']}")
        if "cpu" in meta:
            log(f"  cpu:    {meta['cpu']}")
        log("")

    # Text summary with noise + CI info
    log("📊 Text summary (median, Δ vs current, noise, CI)")
    for bench_name in sorted(samples.keys()):
        log(f"  • {bench_name}")
        versions = samples[bench_name]

        # Order: current | newest → oldest
        keys = list(versions.keys())
        others = [v for v in keys if v != "current"]
        ordered_keys = (["current"] if "current" in versions else []) + sorted(
            others, reverse=True
        )

        medians_ns = {
            v: (median(versions[v]["ns"]) if versions[v]["ns"] else None)
            for v in ordered_keys
        }
        med_current = medians_ns.get("current")

        if med_current is not None:
            log(f"      baseline (current): median={ns_to_human(med_current)}")

        for v in ordered_keys:
            ns_list = versions[v]["ns"]
            b_list = versions[v]["bytes"]
            a_list = versions[v]["allocs"]

            n = len(ns_list)
            if n >= 2:
                med_ns = medians_ns[v]
                mean_ns = mean(ns_list)
                std_ns = stdev(ns_list)
                ns_med_str = ns_to_human(med_ns) if med_ns is not None else "n/a"
                ns_mean_str = ns_to_human(mean_ns)
                ns_std_str = ns_to_human(std_ns)

                if mean_ns > 0:
                    noise_pct = (std_ns / mean_ns) * 100.0
                    se = std_ns / math.sqrt(n)
                    ci_ns = 1.96 * se
                    ci_pct = (ci_ns / mean_ns) * 100.0
                else:
                    noise_pct = None
                    ci_ns = None
                    ci_pct = None

                ci_ns_str = ns_to_human(ci_ns) if ci_ns is not None else "n/a"
                noise_str = format_pct(noise_pct)
                ci_pct_str = format_ci_pct(ci_pct)

                if med_current is not None and med_current > 0:
                    delta_curr = (med_ns / med_current - 1.0) * 100.0
                    delta_curr_str = format_delta_pct(delta_curr)
                else:
                    delta_curr_str = "n/a"
            elif n == 1:
                med_ns = medians_ns[v]
                ns_med_str = ns_to_human(med_ns) if med_ns is not None else "n/a"
                ns_mean_str = ns_med_str
                ns_std_str = "n/a"
                ci_ns_str = "n/a"
                noise_str = "n/a"
                ci_pct_str = "n/a"
                if med_current is not None and med_current > 0:
                    delta_curr = (med_ns / med_current - 1.0) * 100.0
                    delta_curr_str = format_delta_pct(delta_curr)
                else:
                    delta_curr_str = "n/a"
            else:
                ns_med_str = ns_mean_str = ns_std_str = "n/a"
                ci_ns_str = noise_str = ci_pct_str = "n/a"
                delta_curr_str = "n/a"

            b_med_raw = median(b_list) if b_list else None
            a_med_raw = median(a_list) if a_list else None
            b_med = bytes_to_human(b_med_raw)
            a_med = allocs_to_human(a_med_raw)

            log(
                f"      {v:>12}: "
                f"med={ns_med_str:<10} mean={ns_mean_str:<10} std={ns_std_str:<8} "
                f"Δcurr={delta_curr_str:<7} noise={noise_str:<7} CI95={ci_pct_str:<8} "
                f"n={n:<2} bytes={b_med:<10} allocs={a_med}"
            )
    log("")

    # HTML report
    generate_html(html_output, samples, html_template, meta)

    # Cleanup work repo copy
    vlog(f"Removing work repo dir: {repo_dir}")
    try:
        shutil.rmtree(repo_dir)
    except FileNotFoundError:
        pass

    log("✅ Done.")


if __name__ == "__main__":
    main()
