BINARY  := bin/gopher-email
CMD     := ./cmd/gopher-email
MODULE  := github.com/dougpark/gopher-email

.PHONY: build test lint install clean

build:
	go build -o $(BINARY) $(CMD)

test:
	go test ./...

lint:
	golangci-lint run ./...

install: build
	cp $(BINARY) ~/bin/gopher-email

clean:
	rm -rf bin/

# Run once interactively to complete the OAuth flow.
auth:
	$(BINARY) run --config ./config.yaml --verbose

# Example: run the ingestion pipeline.
run: build
	$(BINARY) run --config ./config.yaml --verbose

# Re-index storage directory into DB.
sync: build
	$(BINARY) sync --config ./config.yaml --path ./storage --verbose
