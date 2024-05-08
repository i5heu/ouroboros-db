#!/bin/bash

# Declare an array of version tags
declare -a versions=("v0.0.5" "v0.0.8")  # Add your arbitrary versions here

# Create a directory to hold the benchmark results
mkdir -p benchmarks

# Run benchmarks for each version
for version in "${versions[@]}"; do
    echo "Checking out version: $version"
    git checkout $version

    echo "Running benchmarks for version: $version"
    go test -run='^$' -bench=. -count=6 > "benchmarks/${version}"
done

# Checkout the main branch again
git checkout main

# Generate the benchstat comparison command with all the .txt files
benchstat_cmd="benchstat "
for version in "${versions[@]}"; do
    benchstat_cmd+=" benchmarks/${version}"
done

# Execute the comparison command and save it as a CSV
echo "Comparing all versions with benchstat..."
eval "$benchstat_cmd" > "benchmarks/combined_benchmarks_comparison"
