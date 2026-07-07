package svgcheck

import (
	"os"
	"path/filepath"
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

func TestNamespaceDoesNotCountAsExternalReference(t *testing.T) {
	report, err := Check([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><rect /></svg>`), "")
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if hasIssueCode(report, "external-reference") {
		t.Fatalf("did not expect external-reference for xmlns in %#v", report.Issues)
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

func TestPaperTargetFlagsPrintColorAndEffects(t *testing.T) {
	report, err := Check([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="100" height="100" viewBox="0 0 100 100">
		<defs><filter id="shadow"><feDropShadow dx="1" dy="1" stdDeviation="2" /></filter></defs>
		<rect fill="#ff0000" stroke="rgb(0, 128, 255)" filter="url(#shadow)" width="50" height="50" />
	</svg>`), "paper")
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}

	if !hasIssueCode(report, "rgb-colors-for-print") {
		t.Fatalf("expected rgb-colors-for-print in %#v", report.Issues)
	}
	if !hasIssueCode(report, "print-effects-require-flattening") {
		t.Fatalf("expected print-effects-require-flattening in %#v", report.Issues)
	}
	if !hasIssueCode(report, "shadow-effect") {
		t.Fatalf("expected shadow-effect in %#v", report.Issues)
	}
	if !report.HasErrors() {
		t.Fatal("expected print effects to be an error for paper output")
	}
}

func TestInlineRasterAndColorCountAreRanked(t *testing.T) {
	report, err := Check([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="100" height="100" viewBox="0 0 100 100">
		<image href="data:image/png;base64,AAA=" />
		<rect fill="#111111" />
		<rect fill="#222222" />
	</svg>`), "")
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}

	inlineRaster := issueByCode(report, "inline-raster-image")
	if inlineRaster == nil {
		t.Fatalf("expected inline-raster-image in %#v", report.Issues)
	}
	if inlineRaster.Rank != RankLow {
		t.Fatalf("inline raster rank = %q, want %q", inlineRaster.Rank, RankLow)
	}

	colorCount := issueByCode(report, "color-count")
	if colorCount == nil {
		t.Fatalf("expected color-count in %#v", report.Issues)
	}
	if colorCount.Rank != RankLow {
		t.Fatalf("color-count rank = %q, want %q", colorCount.Rank, RankLow)
	}
}

func TestTargetProfilesApplyOnlyRelevantProductionChecks(t *testing.T) {
	input := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="100" height="100" viewBox="0 0 100 100">
		<defs><filter id="shadow"><feDropShadow dx="1" dy="1" stdDeviation="2" /></filter></defs>
		<image href="data:image/png;base64,AAA=" width="20" height="20" />
		<rect fill="#ff0000" stroke="#000000" stroke-width="0.2px" filter="url(#shadow)" width="50" height="50" />
	</svg>`)

	tests := []struct {
		name      string
		target    string
		wantCodes []string
		denyCodes []string
	}{
		{
			name:   "screen keeps print-only checks quiet",
			target: "screen",
			denyCodes: []string{
				"color-count",
				"raster-image",
				"inline-raster-image",
				"shadow-effect",
				"thin-stroke",
				"rgb-colors-for-print",
				"print-effects-require-flattening",
			},
		},
		{
			name:      "paper requires press color and flattened effects",
			target:    "paper",
			wantCodes: []string{"color-count", "inline-raster-image", "shadow-effect", "thin-stroke", "rgb-colors-for-print", "print-effects-require-flattening"},
		},
		{
			name:      "physical size behaves like production review without CMYK assumptions",
			target:    "20ft",
			wantCodes: []string{"color-count", "inline-raster-image", "shadow-effect", "thin-stroke", "low-effective-ppi"},
			denyCodes: []string{
				"rgb-colors-for-print",
				"print-effects-require-flattening",
			},
		},
		{
			name:      "cutter target prioritizes geometry errors",
			target:    "vinyl",
			wantCodes: []string{"raster-not-cuttable", "effects-may-not-output", "thin-stroke"},
			denyCodes: []string{
				"raster-image",
				"rgb-colors-for-print",
				"print-effects-require-flattening",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report, err := Check(input, tt.target)
			if err != nil {
				t.Fatalf("Check returned error: %v", err)
			}
			for _, code := range tt.wantCodes {
				if !hasIssueCode(report, code) {
					t.Fatalf("expected issue %q in %#v", code, report.Issues)
				}
			}
			for _, code := range tt.denyCodes {
				if hasIssueCode(report, code) {
					t.Fatalf("did not expect issue %q in %#v", code, report.Issues)
				}
			}
		})
	}
}

func TestNearDisconnectedLinesAreProductionOnly(t *testing.T) {
	input := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="100" height="100" viewBox="0 0 100 100">
		<g fill="none" stroke="#111111" stroke-width="4">
			<path d="M 10 10 L 50 10" />
			<path d="M 50.8 10 L 90 10" />
			<polyline points="20,30 40,30 60,30" />
			<line x1="60.7" y1="30" x2="80" y2="30" />
		</g>
	</svg>`)

	for _, target := range []string{"paper", "20ft", "vinyl"} {
		t.Run(target, func(t *testing.T) {
			report, err := Check(input, target)
			if err != nil {
				t.Fatalf("Check returned error: %v", err)
			}
			issue := issueByCode(report, "near-disconnected-lines")
			if issue == nil {
				t.Fatalf("expected near-disconnected-lines in %#v", report.Issues)
			}
			if issue.Rank != RankLow {
				t.Fatalf("near-disconnected-lines rank = %q, want %q", issue.Rank, RankLow)
			}
		})
	}

	screenReport, err := Check(input, "screen")
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if hasIssueCode(screenReport, "near-disconnected-lines") {
		t.Fatalf("did not expect near-disconnected-lines for screen output: %#v", screenReport.Issues)
	}
}

func TestNearDisconnectedLinesRequireVisualStrokeConnection(t *testing.T) {
	input := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="100" height="100" viewBox="0 0 100 100">
		<g fill="none" stroke="#111111" stroke-width="0.25">
			<path d="M 10 10 L 50 10" />
			<path d="M 50.8 10 L 90 10" />
		</g>
		<g fill="none" stroke="#111111" stroke-width="4">
			<path d="M 10 30 L 50 30" />
			<path d="M 50.8 30 L 90 30" />
		</g>
	</svg>`)

	report, err := Check(input, "paper")
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if hasIssueCode(report, "near-disconnected-lines") {
		t.Fatalf("did not expect thin separated strokes or a single thick near-join to flag as constructed shape geometry: %#v", report.Issues)
	}
}

func TestClosedShapesDoNotCountAsNearDisconnectedLines(t *testing.T) {
	input := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="100" height="100" viewBox="0 0 100 100">
		<g fill="none" stroke="#111111" stroke-width="4">
			<path d="M 10 10 L 50 10 L 50 50 Z" />
			<polygon points="10,70 50,70 50,90 10,90" />
			<polyline points="70,70 90,70 70,70" />
		</g>
	</svg>`)

	report, err := Check(input, "paper")
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if hasIssueCode(report, "near-disconnected-lines") {
		t.Fatalf("did not expect closed geometry to flag near-disconnected-lines: %#v", report.Issues)
	}
}

func TestGenerateOverlayHighlightsLocatableIssues(t *testing.T) {
	input := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="100" height="100" viewBox="0 0 100 100">
		<g fill="none" stroke="#111111" stroke-width="4">
			<path d="M 10 10 L 50 10" />
			<path d="M 50.8 10 L 90 10" />
			<path d="M 90.8 10 L 10.8 10" />
		</g>
		<line x1="20" y1="50" x2="80" y2="50" stroke="#111111" stroke-width="0.2px" />
	</svg>`)

	overlay, err := GenerateOverlay(input, OverlayOptions{Target: "paper"})
	if err != nil {
		t.Fatalf("GenerateOverlay returned error: %v", err)
	}
	got := string(overlay)
	for _, want := range []string{
		`<svg xmlns="http://www.w3.org/2000/svg"`,
		`id="pre-print-overlay"`,
		`id="pre-print-near-disconnected-highlights"`,
		`id="pre-print-thin-stroke-highlights"`,
		`near-disconnected-lines`,
		`thin-stroke`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected overlay to contain %q, got %s", want, got)
		}
	}

	screenOverlay, err := GenerateOverlay(input, OverlayOptions{Target: "screen"})
	if err != nil {
		t.Fatalf("GenerateOverlay returned error: %v", err)
	}
	if strings.Contains(string(screenOverlay), `id="pre-print-near-disconnected-highlights"`) {
		t.Fatalf("did not expect near-disconnected highlights for screen overlay: %s", string(screenOverlay))
	}
}

func TestDownloadedFixtureCoverage(t *testing.T) {
	tests := []struct {
		name      string
		target    string
		wantCodes []string
	}{
		{
			name:      "code-example.svg",
			target:    "paper",
			wantCodes: []string{"missing-viewbox", "color-count", "shadow-effect", "rgb-colors-for-print", "print-effects-require-flattening"},
		},
		{
			name:      "url-comparison-chart.svg",
			target:    "paper",
			wantCodes: []string{"color-count", "rgb-colors-for-print"},
		},
		{
			name:      "formatting-currency-infographic.svg",
			target:    "paper",
			wantCodes: []string{"color-count", "rgb-colors-for-print"},
		},
		{
			name:      "site-mockup-lowres.svg",
			target:    "paper",
			wantCodes: []string{"color-count", "rgb-colors-for-print"},
		},
		{
			name:      "ed-tech-logo.svg",
			target:    "paper",
			wantCodes: []string{"color-count", "rgb-colors-for-print"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input, err := os.ReadFile(filepath.Join("testdata", tt.name))
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}

			report, err := Check(input, tt.target)
			if err != nil {
				t.Fatalf("Check returned error: %v", err)
			}

			for _, code := range tt.wantCodes {
				if !hasIssueCode(report, code) {
					t.Fatalf("expected issue %q in %#v", code, report.Issues)
				}
			}
		})
	}
}

func TestGeneratedPrintEdgeCaseFixtures(t *testing.T) {
	tests := []struct {
		name      string
		target    string
		wantCodes []string
	}{
		{
			name:      "print-edge-thin-lines.svg",
			target:    "paper",
			wantCodes: []string{"thin-stroke", "color-count", "rgb-colors-for-print"},
		},
		{
			name:      "print-edge-cmyk-colors.svg",
			target:    "paper",
			wantCodes: []string{"cmyk-in-svg", "color-count"},
		},
		{
			name:      "print-edge-effects.svg",
			target:    "paper",
			wantCodes: []string{"shadow-effect", "rgb-colors-for-print", "print-effects-require-flattening"},
		},
		{
			name:      "print-edge-many-colors.svg",
			target:    "fabric",
			wantCodes: []string{"color-count", "many-fabric-colors"},
		},
		{
			name:      "print-edge-near-disconnected-lines.svg",
			target:    "paper",
			wantCodes: []string{"near-disconnected-lines", "color-count", "rgb-colors-for-print"},
		},
		{
			name:      "print-edge-stylized-fonts.svg",
			target:    "vinyl",
			wantCodes: []string{"external-reference", "text-not-outlined"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := checkFixture(t, tt.name, tt.target)
			for _, code := range tt.wantCodes {
				if !hasIssueCode(report, code) {
					t.Fatalf("expected issue %q in %#v", code, report.Issues)
				}
			}
		})
	}

	cmykReport := checkFixture(t, "print-edge-cmyk-colors.svg", "paper")
	if hasIssueCode(cmykReport, "rgb-colors-for-print") {
		t.Fatalf("did not expect rgb-colors-for-print when CMYK-like color values are present: %#v", cmykReport.Issues)
	}

	manyColorReport := checkFixture(t, "print-edge-many-colors.svg", "")
	colorCount := issueByCode(manyColorReport, "color-count")
	if colorCount == nil {
		t.Fatalf("expected color-count in %#v", manyColorReport.Issues)
	}
	if colorCount.Rank != RankHigh {
		t.Fatalf("color-count rank = %q, want %q", colorCount.Rank, RankHigh)
	}
}

func TestSVGTestAssetsAreValidAndCovered(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("testdata", "*.svg"))
	if err != nil {
		t.Fatalf("glob fixtures: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("expected SVG fixtures in testdata")
	}

	covered := coveredFixtureNames()
	for _, path := range paths {
		name := filepath.Base(path)
		t.Run(name, func(t *testing.T) {
			if _, ok := covered[name]; !ok {
				t.Fatalf("fixture is not covered by an expectation test")
			}

			input, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			if len(input) == 0 {
				t.Fatal("fixture is empty")
			}

			report, err := Check(input, "")
			if err != nil {
				t.Fatalf("Check returned error: %v", err)
			}
			if !report.Meta.FoundSVG {
				t.Fatal("fixture did not expose an SVG root")
			}
			if len(report.Issues) == 0 {
				t.Fatal("fixture does not currently exercise any checker signal")
			}
		})
	}
}

func coveredFixtureNames() map[string]struct{} {
	names := []string{
		"code-example.svg",
		"ed-tech-logo.svg",
		"formatting-currency-infographic.svg",
		"site-mockup-lowres.svg",
		"url-comparison-chart.svg",
		"print-edge-cmyk-colors.svg",
		"print-edge-effects.svg",
		"print-edge-many-colors.svg",
		"print-edge-near-disconnected-lines.svg",
		"print-edge-stylized-fonts.svg",
		"print-edge-thin-lines.svg",
	}

	covered := make(map[string]struct{}, len(names))
	for _, name := range names {
		covered[name] = struct{}{}
	}
	return covered
}

func checkFixture(t *testing.T, name, target string) Report {
	t.Helper()
	input, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	report, err := Check(input, target)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	return report
}

func hasIssueCode(report Report, code string) bool {
	for _, issue := range report.Issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func issueByCode(report Report, code string) *Issue {
	for i := range report.Issues {
		if report.Issues[i].Code == code {
			return &report.Issues[i]
		}
	}
	return nil
}

func closeTo(got, want, tolerance float64) bool {
	diff := got - want
	if diff < 0 {
		diff = -diff
	}
	return diff <= tolerance
}
