.PHONY: build build-linux test deploy

build:
	go build -o mmapi ./cmd/mmapi
	go build -o mmctl ./cmd/mmctl

build-linux:
	GOOS=linux GOARCH=amd64 go build -o mmapi ./cmd/mmapi
	GOOS=linux GOARCH=amd64 go build -o mmctl ./cmd/mmctl

test:
	go test ./...

deploy: build-linux
	@echo "Usage: make deploy HOST=<host>"
	@test -n "$(HOST)" || (echo "HOST is required"; exit 1)
	./deploy/deploy.sh $(HOST) ./mmapi ./deploy/config.json
