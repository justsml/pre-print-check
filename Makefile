.PHONY: build test vet

build:
	go build -o pre-print .

test:
	go test ./...

vet:
	go vet ./...
