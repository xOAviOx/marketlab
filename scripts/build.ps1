$ErrorActionPreference = "Stop"

pnpm --dir web install --frozen-lockfile
pnpm --dir web build
if (Test-Path "cmd/marketlab/dist") {
    Remove-Item "cmd/marketlab/dist" -Recurse -Force
}
New-Item "cmd/marketlab/dist" -ItemType Directory -Force | Out-Null
Copy-Item "web/dist/*" "cmd/marketlab/dist" -Recurse -Force
New-Item "dist" -ItemType Directory -Force | Out-Null
go build -trimpath -o "dist/marketlab.exe" ./cmd/marketlab

