package svgcheck

import (
	"fmt"
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

func TestAnalyzeSVGCollectsReportAndOverlayEvidenceInOneResult(t *testing.T) {
	input := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100"><path d="M 10 10 L 30 10" stroke="black" stroke-width="0.2"/><path d="M 31 10 L 50 10" stroke="black" stroke-width="0.2"/></svg>`)
	target, err := ParseTarget("paper@10in")
	if err != nil {
		t.Fatalf("ParseTarget returned error: %v", err)
	}

	analysis, err := analyzeSVG(input, target)
	if err != nil {
		t.Fatalf("analyzeSVG returned error: %v", err)
	}
	if analysis.Meta.ThinStrokes != 2 {
		t.Fatalf("expected two thin strokes, got %#v", analysis.Meta.ThinStrokeSummaries)
	}
	if len(analysis.ThinShapes) != 2 {
		t.Fatalf("expected two locatable thin shapes, got %#v", analysis.ThinShapes)
	}
	if len(analysis.Endpoints) != 4 {
		t.Fatalf("expected four geometry endpoints, got %#v", analysis.Endpoints)
	}
}

func TestIssuesCarryCentralRemediationPolicy(t *testing.T) {
	report, err := Check([]byte(`<svg width="100" height="50"><script>alert(1)</script><image href="photo.png"/><path d="M 0 0 L 10 10" stroke="black" stroke-width="0.2"/></svg>`), "vinyl")
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}

	script := issueByCode(report, "script")
	if script == nil {
		t.Fatal("expected script issue")
	}
	if script.FixCategory != FixCategorySafety || !script.UnsafeRequired || !script.AutomaticFix {
		t.Fatalf("unexpected script remediation policy: %#v", script)
	}

	raster := issueByCode(report, "raster-not-cuttable")
	if raster == nil {
		t.Fatal("expected raster-not-cuttable issue")
	}
	if raster.FixCategory != FixCategoryRaster || !raster.UnsafeRequired || !raster.AutomaticFix {
		t.Fatalf("unexpected raster remediation policy: %#v", raster)
	}

	thinStroke := issueByCode(report, "thin-stroke")
	if thinStroke == nil {
		t.Fatal("expected thin-stroke issue")
	}
	if thinStroke.FixCategory != FixCategoryStrokes || thinStroke.UnsafeRequired || thinStroke.AutomaticFix {
		t.Fatalf("unexpected thin-stroke remediation policy: %#v", thinStroke)
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

func TestFixCategoriesScopeChanges(t *testing.T) {
	input := []byte(`<svg width="100" height="50"><script>alert(1)</script><rect onclick="x()" /></svg>`)

	metadataOnly, err := Fix(input, FixOptions{Categories: []string{"metadata"}})
	if err != nil {
		t.Fatalf("Fix metadata returned error: %v", err)
	}
	got := string(metadataOnly.SVG)
	if !strings.Contains(got, `xmlns="http://www.w3.org/2000/svg"`) || !strings.Contains(got, `viewBox="0 0 100 50"`) {
		t.Fatalf("expected metadata fixes, got %s", got)
	}
	if !strings.Contains(got, "<script") || !strings.Contains(got, "onclick") {
		t.Fatalf("metadata fix should not remove unsafe content, got %s", got)
	}

	safetyOnly, err := Fix(input, FixOptions{Unsafe: true, Categories: []string{"safety"}})
	if err != nil {
		t.Fatalf("Fix safety returned error: %v", err)
	}
	got = string(safetyOnly.SVG)
	if strings.Contains(got, "<script") || strings.Contains(got, "onclick") {
		t.Fatalf("expected safety fix to remove unsafe content, got %s", got)
	}
	if strings.Contains(got, "viewBox") {
		t.Fatalf("safety-only fix should not add metadata, got %s", got)
	}
}

func TestFixAllSkipsUnsafeChangesWithoutUnsafeFlag(t *testing.T) {
	result, err := Fix([]byte(`<svg width="100" height="50"><script>alert(1)</script><rect onclick="x()" /></svg>`), FixOptions{})
	if err != nil {
		t.Fatalf("Fix returned error: %v", err)
	}

	got := string(result.SVG)
	if strings.Contains(got, "<script") == false || strings.Contains(got, "onclick") == false {
		t.Fatalf("expected unsafe content to remain without --unsafe, got %s", got)
	}
	if len(result.Skipped) == 0 {
		t.Fatalf("expected skipped unsafe note, got %#v", result)
	}
}

func TestFixEffectsAndRasterRequireUnsafe(t *testing.T) {
	input := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="100" height="50" viewBox="0 0 100 50">
		<defs><filter id="blur"><feGaussianBlur stdDeviation="2"/></filter></defs>
		<image href="data:image/png;base64,AAA=" width="10" height="10"/>
		<rect filter="url(#blur)" opacity="0.5" width="20" height="20"/>
	</svg>`)

	safeResult, err := Fix(input, FixOptions{Target: "paper", Categories: []string{"effects,raster"}})
	if err != nil {
		t.Fatalf("Fix safe effects/raster returned error: %v", err)
	}
	if !strings.Contains(string(safeResult.SVG), "<filter") || !strings.Contains(string(safeResult.SVG), "<image") {
		t.Fatalf("expected destructive content to remain without unsafe, got %s", string(safeResult.SVG))
	}
	if len(safeResult.Skipped) < 2 {
		t.Fatalf("expected skipped notes for effects and raster, got %#v", safeResult.Skipped)
	}

	unsafeResult, err := Fix(input, FixOptions{Target: "paper", Unsafe: true, Categories: []string{"effects,raster"}})
	if err != nil {
		t.Fatalf("Fix unsafe effects/raster returned error: %v", err)
	}
	got := strings.ToLower(string(unsafeResult.SVG))
	for _, removed := range []string{"<filter", "<image", "filter=", "opacity="} {
		if strings.Contains(got, removed) {
			t.Fatalf("expected %q to be removed, got %s", removed, got)
		}
	}
}

func TestFixBleedExpandsBackgroundRect(t *testing.T) {
	input := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="4in" height="3in" viewBox="0 0 384 288">
		<rect width="384" height="288" fill="#f8fafc"/>
	</svg>`)

	result, err := Fix(input, FixOptions{Target: "paper", Categories: []string{"bleed"}})
	if err != nil {
		t.Fatalf("Fix bleed returned error: %v", err)
	}
	got := string(result.SVG)
	if !strings.Contains(got, `x="-15.118"`) || !strings.Contains(got, `width="414.236"`) {
		t.Fatalf("expected background rect to expand for bleed, got %s", got)
	}
}

func TestParseFixCategories(t *testing.T) {
	categories, err := ParseFixCategories("metadata, safety effects")
	if err != nil {
		t.Fatalf("ParseFixCategories returned error: %v", err)
	}
	want := []string{"metadata", "safety", "effects"}
	if !slices.Equal(categories, want) {
		t.Fatalf("categories = %#v, want %#v", categories, want)
	}

	for _, raw := range []string{"metadata,,safety", ",metadata", "metadata,", "not-real"} {
		t.Run(raw, func(t *testing.T) {
			if _, err := ParseFixCategories(raw); err == nil {
				t.Fatalf("expected ParseFixCategories(%q) to fail", raw)
			}
		})
	}
}

func TestParseTargets(t *testing.T) {
	tests := []struct {
		name         string
		raw          string
		wantInches   float64
		wantWidth    int
		wantMaterial MaterialTarget
	}{
		{name: "meters", raw: "1.2m", wantInches: 47.24409448824},
		{name: "8k", raw: "8k", wantWidth: 7680},
		{name: "material with size", raw: "fabric@14in", wantInches: 14, wantMaterial: MaterialFabric},
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
			if target.Material != tt.wantMaterial {
				t.Fatalf("Material = %q, want %q", target.Material, tt.wantMaterial)
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
		`id="pre-print-check-overlay"`,
		`id="pre-print-check-near-disconnected-highlights"`,
		`id="pre-print-check-thin-stroke-highlights"`,
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
	if strings.Contains(string(screenOverlay), `id="pre-print-check-near-disconnected-highlights"`) {
		t.Fatalf("did not expect near-disconnected highlights for screen overlay: %s", string(screenOverlay))
	}
}

func TestBleedAndSafeAreaRecommendations(t *testing.T) {
	input := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="4in" height="3in" viewBox="0 0 384 288">
		<rect width="384" height="288" fill="#f8fafc"/>
		<text x="4" y="26" font-size="18" fill="#111111">Edge</text>
		<circle cx="360" cy="144" r="8" fill="#ef4444"/>
	</svg>`)

	report, err := Check(input, "paper")
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}

	assertIssueMessageContains(t, report, "missing-bleed", "Bleed:", "trim edge", "0.125in/3mm")
	assertIssueMessageContains(t, report, "safe-area-risk", "Safe area:", "2 non-background elements", "printer template")
}

func TestBleedAllowsExplicitOverhang(t *testing.T) {
	input := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="4in" height="3in" viewBox="0 0 384 288">
		<rect x="-14" y="-14" width="412" height="316" fill="#f8fafc"/>
		<text x="96" y="150" font-size="18" fill="#111111">Centered</text>
	</svg>`)

	report, err := Check(input, "paper")
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if hasIssueCode(report, "missing-bleed") {
		t.Fatalf("did not expect missing-bleed when background extends past the trim edge: %#v", report.Issues)
	}
	if hasIssueCode(report, "safe-area-risk") {
		t.Fatalf("did not expect safe-area-risk for centered content: %#v", report.Issues)
	}
}

func TestDetailedProductionRecommendations(t *testing.T) {
	var smallShapes strings.Builder
	for i := 0; i < 80; i++ {
		x := 20 + i%20
		y := 220 + i/20
		fmt.Fprintf(&smallShapes, `<circle cx="%d" cy="%d" r="0.2" fill="#111111"/>`, x, y)
	}

	input := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="14in" height="10in" viewBox="0 0 1344 960">
		<rect width="1344" height="960" fill="#ffffff" fill-opacity="0.82"/>
		<defs>
			<filter id="soft"><feGaussianBlur stdDeviation="1.4"/></filter>
			<filter id="shadow"><feDropShadow dx="6" dy="6" stdDeviation="4"/></filter>
		</defs>
		<polygon points="90,70 210,70 210,130 90,130" fill="#111111"/>
		<polygon points="130,55 250,55 250,115 130,115" fill="#777777"/>
		<polygon points="170,80 290,80 290,140 170,140" fill="#eeeeee"/>
		<text x="120" y="112" font-size="38" fill="#ffffff">Acme</text>
		<g stroke="#111111" fill="none">
			<line x1="40" y1="180" x2="160" y2="180" stroke-width="1pt"/>
			<line x1="40" y1="188" x2="160" y2="188" stroke-width="1pt"/>
			<line x1="40" y1="196" x2="160" y2="196" stroke-width="1pt"/>
			<line x1="40" y1="204" x2="160" y2="204" stroke-width="1pt"/>
			<line x1="40" y1="212" x2="160" y2="212" stroke-width="1pt"/>
			<line x1="40" y1="220" x2="160" y2="220" stroke-width="1pt"/>
		</g>
		<rect x="320" y="70" width="160" height="80" fill="#777777" filter="url(#shadow)"/>
		<rect x="520" y="70" width="80" height="80" fill="#eeeeee" filter="url(#soft)"/>
		` + smallShapes.String() + `
	</svg>`)

	report, err := Check(input, "fabric@14in")
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}

	assertIssueMessageContains(t, report, "text-overlap-shapes", `"Acme"`, "overlaps 3 polygon")
	assertIssueMessageContains(t, report, "thin-stroke", "6 stroked", "1pt", "14.0in on fabric")
	assertIssueMessageContains(t, report, "small-detail-durability", "80 sub-1mm", "80 sub-2mm", "material choices")
	assertIssueMessageContains(t, report, "fabric-effects", "subtle effect", "large shadow")
	assertIssueMessageContains(t, report, "background-transparency", "1 background transparency issue")
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

func assertIssueMessageContains(t *testing.T, report Report, code string, parts ...string) {
	t.Helper()
	issue := issueByCode(report, code)
	if issue == nil {
		t.Fatalf("expected issue %q in %#v", code, report.Issues)
	}
	for _, part := range parts {
		if !strings.Contains(issue.Message, part) {
			t.Fatalf("expected issue %q message to contain %q, got %q", code, part, issue.Message)
		}
	}
}

func closeTo(got, want, tolerance float64) bool {
	diff := got - want
	if diff < 0 {
		diff = -diff
	}
	return diff <= tolerance
}
