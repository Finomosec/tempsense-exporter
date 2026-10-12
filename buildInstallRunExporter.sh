#!/bin/bash
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd $SCRIPT_DIR
go build -o . ./cmd/... && go install ./cmd/... && $GOPATH/bin/tempsense-exporter

