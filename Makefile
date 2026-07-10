.PHONY: build test vet go-package-check wasm npm-test serve-web

build:
	go build -o pre-print-check .

test:
	go test ./...

vet:
	go vet ./...

go-package-check:
	go list ./svgcheck
	go test ./svgcheck

wasm:
	mkdir -p dist
	GOOS=js GOARCH=wasm go build -ldflags="-s -w" -o dist/pre-print-check.wasm ./cmd/preprintcheck-wasm
	GOOS=js GOARCH=wasm go build -ldflags="-s -w" -o dist/pre-print-check-check.wasm ./cmd/preprintcheck-wasm-check
	GOOS=js GOARCH=wasm go build -ldflags="-s -w" -o dist/pre-print-check-fix.wasm ./cmd/preprintcheck-wasm-fix
	cp "$$(go env GOROOT)/lib/wasm/wasm_exec.js" dist/wasm_exec.js

npm-test:
	npm test

serve-web: wasm
	python3 -m http.server 8765 --bind 127.0.0.1
