//go:build js && wasm

package main

import "github.com/justsml/pre-print-check/internal/wasmapi"

func main() {
	wasmapi.ServeFix("prePrintCheckFix")
}
