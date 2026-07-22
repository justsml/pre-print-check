package svgcheck_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/justsml/pre-print-check/svgcheck"
)

func TestPublicTypesAreOwnedByPublicPackage(t *testing.T) {
	for name, value := range map[string]any{
		"Report": svgcheck.Report{},
		"Issue":  svgcheck.Issue{},
		"Target": svgcheck.Target{},
	} {
		if got := reflect.TypeOf(value).PkgPath(); got != "github.com/justsml/pre-print-check/svgcheck" {
			t.Fatalf("%s is owned by %q, want public svgcheck package", name, got)
		}
	}
}

func TestPublicPackageChecksSVG(t *testing.T) {
	report, err := svgcheck.Check([]byte(`<svg width="100" height="50"><script>alert(1)</script></svg>`), "paper")
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if !report.HasErrors() {
		t.Fatal("expected script issue to make the report fail")
	}
	if report.Target.Material != svgcheck.MaterialPaper {
		t.Fatalf("Material = %q, want %q", report.Target.Material, svgcheck.MaterialPaper)
	}
}

func TestPublicPackageFixesAndOverlaysSVG(t *testing.T) {
	result, err := svgcheck.Fix([]byte(`<svg width="100" height="50"><rect /></svg>`), svgcheck.FixOptions{
		Categories: []string{string(svgcheck.FixCategoryMetadata)},
	})
	if err != nil {
		t.Fatalf("Fix returned error: %v", err)
	}
	if !strings.Contains(string(result.SVG), `viewBox="0 0 100 50"`) {
		t.Fatalf("expected viewBox fix, got %s", string(result.SVG))
	}

	overlay, err := svgcheck.GenerateOverlay(result.SVG, svgcheck.OverlayOptions{Target: "paper"})
	if err != nil {
		t.Fatalf("GenerateOverlay returned error: %v", err)
	}
	if !strings.Contains(string(overlay), "pre-print-check-overlay") {
		t.Fatalf("expected overlay marker, got %s", string(overlay))
	}
}
