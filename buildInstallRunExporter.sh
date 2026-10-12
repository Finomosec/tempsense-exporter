#!/bin/bash
export PATH=/home/frederic/go1.23.2/bin/:$PATH
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd $SCRIPT_DIR
go build -o . ./cmd/... && go install ./cmd/... && $GOPATH/bin/tempsense-exporter

