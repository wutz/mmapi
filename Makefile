.PHONY: build build-linux test deploy

build:
	go build -o mmapi ./cmd/mmapi

build-linux:
	GOOS=linux GOARCH=amd64 go build -o mmapi ./cmd/mmapi

test:
	go test ./...

deploy: build-linux
	@echo "Usage: make deploy HOST=<host>"
	@test -n "$(HOST)" || (echo "HOST is required"; exit 1)
	./deploy/deploy.sh $(HOST) ./mmapi ./deploy/config.json
