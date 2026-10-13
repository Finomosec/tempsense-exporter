#!/bin/bash
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd $SCRIPT_DIR
go build -o bin/ ./cmd/... && bin/tempsense-exporter "$@"

