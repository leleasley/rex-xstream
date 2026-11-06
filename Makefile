BINARY := rex-xstream
GO ?= go

.PHONY: build test vet fmt run docker docker-run clean

build:
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags="-s -w" -o $(BINARY) ./cmd/rex-xstream

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

fmt:
	$(GO) fmt ./...

run:
	$(GO) run ./cmd/rex-xstream

docker:
	docker build -t rex-xstream:local .

docker-run:
	docker compose up -d --build

clean:
	rm -f $(BINARY)
