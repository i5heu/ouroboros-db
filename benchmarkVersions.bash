#!/bin/bash

docker build -f benchmark/Dockerfile -t my-go-bench .

docker run --rm \
  -v "$PWD":/repo-src:ro \
  -v "$PWD/benchmark":/benchmark \
  -v "$PWD/.bench-html":/results-html \
  -e NUM_VERSIONS=3 \
  -e BENCH_PKG_PATTERN=./... \
  -e BENCH_COUNT=30 \
  -e BENCH_TIME=2s \
  -e VERBOSE=1 \
  my-go-bench

  #-e BENCH_FILTER='BenchmarkBlob' \
