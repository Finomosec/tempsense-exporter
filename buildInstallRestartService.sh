#!/bin/bash
cd /home/frederic/go/src/tempsense-exporter/
go build -o . ./cmd/... && go install ./cmd/... && sudo systemctl restart tempsense-exporter.service

