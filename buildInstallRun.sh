#!/bin/bash
export PATH=/home/frederic/go1.23.2/bin/:$PATH
cd /home/frederic/go/src/tempsense-exporter/
go build -o . ./cmd/... && go install ./cmd/... && $GOPATH/bin/tempsense-cli

