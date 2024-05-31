#!/bin/bash
go build -o . ./cmd/... && go install ./cmd/... && $GOPATH/bin/tempsense-cli

