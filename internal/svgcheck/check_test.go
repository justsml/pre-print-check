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

func TestCheckAllowsBareSVGRoot(t *testing.T) {
	report, err := Check([]byte(`<svg><rect /></svg>`), "")
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if len(report.Issues) == 0 {
		t.Fatal("expected missing metadata issues")
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

func TestParseMaterialTargets(t *testing.T) {
	tests := map[string]MaterialTarget{
		"screen":       MaterialScreen,
		"web":          MaterialScreen,
		"paper":        MaterialPaper,
		"fabric":       MaterialFabric,
		"vinyl":        MaterialVinyl,
		"sticker":      MaterialVinyl,
		"banner":       MaterialBanner,
		"signage":      MaterialSignage,
		"vehicle-wrap": MaterialVehicleWrap,
		"packaging":    MaterialPackaging,
		"laser":        MaterialLaser,
		"cnc":          MaterialCNC,
		"plotter":      MaterialPlotter,
	}

	for raw, want := range tests {
		target, err := ParseTarget(raw)
		if err != nil {
			t.Fatalf("ParseTarget(%q) returned error: %v", raw, err)
		}
		if target.Material != want {
			t.Fatalf("ParseTarget(%q) material = %q, want %q", raw, target.Material, want)
		}
	}
}

func TestVinylTargetFlagsNonCuttableContent(t *testing.T) {
	report, err := Check([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><filter id="blur" /><image href="art.png" /><text>Hello</text></svg>`), "vinyl")
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}

	codes := map[string]bool{}
	for _, issue := range report.Issues {
		codes[issue.Code] = true
	}
	for _, code := range []string{"raster-not-cuttable", "text-not-outlined", "effects-may-not-output"} {
		if !codes[code] {
			t.Fatalf("expected issue %q in %#v", code, report.Issues)
		}
	}
}
