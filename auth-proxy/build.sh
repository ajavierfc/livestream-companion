#!/bin/bash
set -e
CGO_ENABLED=0 go build -o auth-proxy -ldflags="-s -w" main.go
mv auth-proxy ../auth-proxy-service