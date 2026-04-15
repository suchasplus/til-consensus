GO ?= go

.PHONY: fmt test vet lint

fmt:
	$(GO) fmt ./...

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

lint:
	golangci-lint run
