#!/bin/bash
export CGO_ENABLED=0
export GOARCH=arm64
export GOOS=linux
go build \
    -tags lambda.norpc \
    -o ./cmd/lambda_consumer_demo/bootstrap \
    ./cmd/lambda_consumer_demo/main.go

zip -j ./cmd/lambda_consumer_demo/terraform/bootstrap.zip ./cmd/lambda_consumer_demo/bootstrap

rm ./cmd/lambda_consumer_demo/bootstrap
