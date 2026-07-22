package svgcheck

import internal "github.com/justsml/pre-print-check/internal/svgcheck"

// CheckFile reads an SVG file from path and returns a preflight report.
func CheckFile(path string, rawTarget string) (Report, error) {
	report, err := internal.CheckFile(path, rawTarget)
	return publicReport(report), err
}

// Check inspects SVG bytes and returns a preflight report for the target.
func Check(input []byte, rawTarget string) (Report, error) {
	report, err := internal.Check(input, rawTarget)
	return publicReport(report), err
}

// GenerateOverlay returns an annotated SVG overlay for locatable findings.
func GenerateOverlay(input []byte, opts OverlayOptions) ([]byte, error) {
	return internal.GenerateOverlay(input, internal.OverlayOptions{Target: opts.Target})
}

// Fix applies conservative SVG repairs, plus unsafe repairs when requested.
func Fix(input []byte, opts FixOptions) (FixResult, error) {
	result, err := internal.Fix(input, internal.FixOptions{
		Target:     opts.Target,
		Unsafe:     opts.Unsafe,
		Categories: opts.Categories,
	})
	return FixResult{SVG: result.SVG, Changes: result.Changes, Skipped: result.Skipped}, err
}

// FixCategoryNames returns all supported fix category names.
func FixCategoryNames() []string {
	return internal.FixCategoryNames()
}

// ParseFixCategories parses a comma- or space-separated fix category list.
func ParseFixCategories(raw string) ([]string, error) {
	return internal.ParseFixCategories(raw)
}

// ParseTarget parses an output material, size, or resolution target.
func ParseTarget(raw string) (Target, error) {
	target, err := internal.ParseTarget(raw)
	return publicTarget(target), err
}

func publicReport(report internal.Report) Report {
	issues := make([]Issue, len(report.Issues))
	for i, issue := range report.Issues {
		issues[i] = Issue{
			Severity:       Severity(issue.Severity),
			Code:           issue.Code,
			Message:        issue.Message,
			Rank:           FindingRank(issue.Rank),
			FixCategory:    FixCategory(issue.FixCategory),
			UnsafeRequired: issue.UnsafeRequired,
			AutomaticFix:   issue.AutomaticFix,
		}
	}

	strokeSummaries := make([]StrokeWidthSummary, len(report.Meta.ThinStrokeSummaries))
	for i, summary := range report.Meta.ThinStrokeSummaries {
		strokeSummaries[i] = StrokeWidthSummary{Width: summary.Width, Count: summary.Count}
	}
	textOverlaps := make([]TextShapeOverlap, len(report.Meta.TextShapeOverlaps))
	for i, overlap := range report.Meta.TextShapeOverlaps {
		textOverlaps[i] = TextShapeOverlap{Text: overlap.Text, ShapeType: overlap.ShapeType, ShapeCount: overlap.ShapeCount}
	}

	meta := report.Meta
	return Report{
		Path:   report.Path,
		Target: publicTarget(report.Target),
		Meta: SVGMeta{
			FoundSVG:               meta.FoundSVG,
			Width:                  meta.Width,
			Height:                 meta.Height,
			WidthPixels:            meta.WidthPixels,
			HeightPixels:           meta.HeightPixels,
			ViewBox:                meta.ViewBox,
			HasXMLNS:               meta.HasXMLNS,
			Scripts:                meta.Scripts,
			EventAttrs:             meta.EventAttrs,
			ExternalRefs:           meta.ExternalRefs,
			RasterImages:           meta.RasterImages,
			InlineRasterImages:     meta.InlineRasterImages,
			TextElements:           meta.TextElements,
			Filters:                meta.Filters,
			FilterRefs:             meta.FilterRefs,
			Shadows:                meta.Shadows,
			Masks:                  meta.Masks,
			ClipPaths:              meta.ClipPaths,
			Opacity:                meta.Opacity,
			BlendModes:             meta.BlendModes,
			ThinStrokes:            meta.ThinStrokes,
			ThinStrokeSummaries:    strokeSummaries,
			NearDisconnected:       meta.NearDisconnected,
			TextShapeOverlaps:      textOverlaps,
			SmallShapesSub1MM:      meta.SmallShapesSub1MM,
			SmallShapesSub2MM:      meta.SmallShapesSub2MM,
			SubtleEffects:          meta.SubtleEffects,
			LargeShadows:           meta.LargeShadows,
			BackgroundTransparency: meta.BackgroundTransparency,
			MissingBleedShapes:     meta.MissingBleedShapes,
			SafeAreaRiskShapes:     meta.SafeAreaRiskShapes,
			ColorValues:            meta.ColorValues,
			UniqueColors:           meta.UniqueColors,
			CMYKColors:             meta.CMYKColors,
		},
		Issues: issues,
	}
}

func publicTarget(target internal.Target) Target {
	return Target{
		Raw:          target.Raw,
		Material:     MaterialTarget(target.Material),
		WidthInches:  target.WidthInches,
		HeightInches: target.HeightInches,
		PixelsWide:   target.PixelsWide,
		PixelsHigh:   target.PixelsHigh,
	}
}
