package svgcheck

import (
	"slices"
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

	for _, code := range []string{"missing-xmlns", "missing-viewbox", "script", "event-handler", "low-effective-ppi"} {
		if !hasIssueCode(report, code) {
			t.Fatalf("expected issue %q in %#v", code, report.Issues)
		}
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
	result, err := Fix([]byte(`<svg width="100" height="50"><script>alert(1)</script><rect onclick="x()" onload='y()' /></svg>`), FixOptions{Unsafe: true})
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
	if strings.Contains(got, "onload") {
		t.Fatalf("expected single-quoted event handler removal, got %s", got)
	}

	if !slices.Contains(result.Changes, "removed 1 script element(s)") {
		t.Fatalf("expected script removal change, got %#v", result.Changes)
	}
	if !slices.Contains(result.Changes, "removed 2 event handler attribute(s)") {
		t.Fatalf("expected event handler removal change, got %#v", result.Changes)
	}
}

func TestParseTargets(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		wantInches float64
		wantWidth  int
	}{
		{name: "meters", raw: "1.2m", wantInches: 47.24409448824},
		{name: "8k", raw: "8k", wantWidth: 7680},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, err := ParseTarget(tt.raw)
			if err != nil {
				t.Fatalf("ParseTarget(%q) returned error: %v", tt.raw, err)
			}
			if tt.wantInches > 0 && !closeTo(target.WidthInches, tt.wantInches, 0.001) {
				t.Fatalf("WidthInches = %f, want %f", target.WidthInches, tt.wantInches)
			}
			if target.PixelsWide != tt.wantWidth {
				t.Fatalf("PixelsWide = %d, want %d", target.PixelsWide, tt.wantWidth)
			}
		})
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
		t.Run(raw, func(t *testing.T) {
			target, err := ParseTarget(raw)
			if err != nil {
				t.Fatalf("ParseTarget(%q) returned error: %v", raw, err)
			}
			if target.Material != want {
				t.Fatalf("ParseTarget(%q) material = %q, want %q", raw, target.Material, want)
			}
		})
	}
}

func TestVinylTargetFlagsNonCuttableContent(t *testing.T) {
	report, err := Check([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><filter id="blur" /><image href="art.png" /><text>Hello</text></svg>`), "vinyl")
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}

	for _, code := range []string{"raster-not-cuttable", "text-not-outlined", "effects-may-not-output"} {
		if !hasIssueCode(report, code) {
			t.Fatalf("expected issue %q in %#v", code, report.Issues)
		}
	}
}

func hasIssueCode(report Report, code string) bool {
	for _, issue := range report.Issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func closeTo(got, want, tolerance float64) bool {
	diff := got - want
	if diff < 0 {
		diff = -diff
	}
	return diff <= tolerance
}
