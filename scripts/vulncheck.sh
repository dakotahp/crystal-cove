#!/bin/sh
# Run govulncheck under the Go builder image the Dockerfile uses, so the
# standard library it checks is the one compiled into the shipped binary.
set -eu

builder=$(awk '/^FROM golang:/ { print $2; exit }' Dockerfile)
echo "==> govulncheck with $builder"
docker run --rm -v "$PWD:/src" -w /src "$builder" \
  go run golang.org/x/vuln/cmd/govulncheck@v1.7.0 ./...
