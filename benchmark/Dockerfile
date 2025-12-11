# benchmark/Dockerfile
FROM golang:1.25

# Install git and Python (for the benchmark runner)
RUN apt-get update && apt-get install -y --no-install-recommends \
    git \
    python3 \
    && git config --global --add safe.directory '*' \
    && rm -rf /var/lib/apt/lists/*

# Install benchstat (we might still use it later if we want; harmless to have)
RUN go install golang.org/x/perf/cmd/benchstat@latest

# Ensure Go bin and unbuffered Python
ENV PATH="/go/bin:${PATH}"
ENV PYTHONUNBUFFERED=1

# Default locations (overridable via env)
# REPO_SRC: where host repo is mounted (read-only)
# REPO_DIR: where we copy it inside the container
ENV REPO_SRC=/repo-src
ENV REPO_DIR=/repo-work

# HTML output on host (mount a volume here)
ENV HTML_OUTPUT=/results-html/benchmarks.html

# Benchmark config defaults
ENV NUM_VERSIONS=5
ENV BENCH_PKG_PATTERN=./...
ENV BENCH_FILTER=.
ENV BENCH_COUNT=8
ENV BENCH_TIME=1s
ENV VERBOSE=1

# We'll mount /benchmark from the host; script lives there
WORKDIR /benchmark

ENTRYPOINT ["python3", "/benchmark/run_benchmarks.py"]
