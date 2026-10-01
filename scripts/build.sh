#!/usr/bin/env sh
set -eu

pnpm --dir web install --frozen-lockfile
pnpm --dir web build
rm -rf cmd/marketlab/dist
mkdir -p cmd/marketlab/dist
cp -R web/dist/. cmd/marketlab/dist/
go build -trimpath -o dist/marketlab ./cmd/marketlab

