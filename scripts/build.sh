#!/usr/bin/env sh
set -eu

npm --prefix web ci
npm --prefix web run build
rm -rf cmd/marketlab/dist
mkdir -p cmd/marketlab/dist
cp -R web/dist/. cmd/marketlab/dist/
go build -trimpath -o dist/marketlab ./cmd/marketlab
