.PHONY: build build-linux test vet deploy

# The config to deploy: your own copy of deploy/config.sample.json, which
# carries placeholders deploy.sh refuses to push.
CONFIG ?= ./config.local.json

build:
	go build -o mmapi ./cmd/mmapi
	go build -o mmctl ./cmd/mmctl

build-linux:
	GOOS=linux GOARCH=amd64 go build -o mmapi ./cmd/mmapi
	GOOS=linux GOARCH=amd64 go build -o mmctl ./cmd/mmctl

test:
	go test ./...

vet:
	go vet ./...

deploy: build-linux
	@echo "Usage: make deploy HOST=<host> [CONFIG=<path>]"
	@test -n "$(HOST)" || (echo "HOST is required"; exit 1)
	./deploy/deploy.sh $(HOST) ./mmapi $(CONFIG)
