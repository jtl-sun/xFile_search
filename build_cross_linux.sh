#!/usr/bin/env bash
set -euo pipefail
go test ./...
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags='-H=windowsgui' -o xFile_search.exe .
cp xFile_search.exe xFile_indexer.exe
echo "Built xFile_search.exe + xFile_indexer.exe"
