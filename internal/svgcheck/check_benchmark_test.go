package svgcheck

import (
	"os"
	"path/filepath"
	"testing"
)

var (
	benchmarkSimpleSVG = []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="100" height="50" viewBox="0 0 100 50"><rect width="100" height="50" fill="#fff"/><path d="M10 10 L90 10" stroke="#111" stroke-width="0.25"/></svg>`)
	benchmarkLargeSVG  = mustReadBenchmarkFile("formatting-currency-infographic.svg")
)

func BenchmarkCheckSimplePaper(b *testing.B) {
	benchmarkCheck(b, benchmarkSimpleSVG, "paper")
}

func BenchmarkCheckLargePaper(b *testing.B) {
	benchmarkCheck(b, benchmarkLargeSVG, "paper")
}

func BenchmarkFixSimpleMetadata(b *testing.B) {
	input := []byte(`<svg width="100" height="50"><rect width="100" height="50" /></svg>`)
	for b.Loop() {
		if _, err := Fix(input, FixOptions{Categories: []string{"metadata"}}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGenerateOverlayLargePaper(b *testing.B) {
	for b.Loop() {
		if _, err := GenerateOverlay(benchmarkLargeSVG, OverlayOptions{Target: "paper"}); err != nil {
			b.Fatal(err)
		}
	}
}

func benchmarkCheck(b *testing.B, input []byte, target string) {
	for b.Loop() {
		if _, err := Check(input, target); err != nil {
			b.Fatal(err)
		}
	}
}

func mustReadBenchmarkFile(name string) []byte {
	input, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		panic(err)
	}
	return input
}
