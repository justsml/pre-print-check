.PHONY: build test vet wasm npm-test serve-web

build:
	go build -o pre-print .

test:
	go test ./...

vet:
	go vet ./...

wasm:
	mkdir -p dist
	GOOS=js GOARCH=wasm go build -o dist/pre-print.wasm ./cmd/preprint-wasm
	cp "$$(go env GOROOT)/lib/wasm/wasm_exec.js" dist/wasm_exec.js

npm-test:
	npm test

serve-web: wasm
	python3 -m http.server 8765 --bind 127.0.0.1
