#!/bin/bash

set -e

echo "🔨 Rebuilding Go Lambda functions using 'provided.al2'..."

for dir in components/AIChat components/Auth ; do
  echo "📁 Building: $dir"
  (cd "$dir" && GOOS=linux GOARCH=amd64 go build -o bootstrap main.go)
done

echo "✅ All bootstrap binaries successfully built."
