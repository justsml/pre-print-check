package svgcheck

import (
	"strings"
	"testing"
)

func TestCheckFlagsUnsafeSVG(t *testing.T) {
	report, err := Check([]byte(`<svg width="100" height="50"><script>alert(1)</script><rect onclick="x()" /></svg>`), "90in")
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}

	if !report.HasErrors() {
		t.Fatal("expected unsafe SVG to have errors")
	}
	if len(report.Issues) < 4 {
		t.Fatalf("expected several issues, got %#v", report.Issues)
	}
}

func TestFixAddsSafeRootAttributes(t *testing.T) {
	result, err := Fix([]byte(`<svg width="100" height="50"><rect /></svg>`), FixOptions{})
	if err != nil {
		t.Fatalf("Fix returned error: %v", err)
	}

	got := string(result.SVG)
	if !strings.Contains(got, `xmlns="http://www.w3.org/2000/svg"`) {
		t.Fatalf("expected xmlns, got %s", got)
	}
	if !strings.Contains(got, `viewBox="0 0 100 50"`) {
		t.Fatalf("expected viewBox, got %s", got)
	}
}

func TestUnsafeFixRemovesScriptAndEventHandlers(t *testing.T) {
	result, err := Fix([]byte(`<svg width="100" height="50"><script>alert(1)</script><rect onclick="x()" /></svg>`), FixOptions{Unsafe: true})
	if err != nil {
		t.Fatalf("Fix returned error: %v", err)
	}

	got := string(result.SVG)
	if strings.Contains(got, "<script") {
		t.Fatalf("expected script removal, got %s", got)
	}
	if strings.Contains(got, "onclick") {
		t.Fatalf("expected event handler removal, got %s", got)
	}
}

func TestParseTargets(t *testing.T) {
	target, err := ParseTarget("1.2m")
	if err != nil {
		t.Fatalf("ParseTarget returned error: %v", err)
	}
	if target.WidthInches < 47.2 || target.WidthInches > 47.3 {
		t.Fatalf("unexpected inches: %f", target.WidthInches)
	}

	target, err = ParseTarget("8k")
	if err != nil {
		t.Fatalf("ParseTarget returned error: %v", err)
	}
	if target.PixelsWide != 7680 {
		t.Fatalf("unexpected 8k width: %d", target.PixelsWide)
	}
}
