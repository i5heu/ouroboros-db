#!/usr/bin/env bash

COVER_DIR=$(mktemp -d)
trap 'rm -rf "$COVER_DIR"' EXIT

time go test ./... -coverprofile="$COVER_DIR/cover.out" -covermode=atomic -coverpkg=./...
go tool cover -func="$COVER_DIR/cover.out"
