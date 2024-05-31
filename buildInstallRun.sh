#!/bin/bash
git pull && go build -o . ./cmd/... && go install ./cmd/... && $GOPATH/bin/tempsense-cli

