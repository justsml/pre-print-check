package svgcheck

import internal "github.com/justsml/pre-print-check/internal/svgcheck"

type Severity = internal.Severity

const (
	SeverityError   = internal.SeverityError
	SeverityWarning = internal.SeverityWarning
	SeverityInfo    = internal.SeverityInfo
)

type FindingRank = internal.FindingRank

const (
	RankHigh     = internal.RankHigh
	RankModerate = internal.RankModerate
	RankLow      = internal.RankLow
)

type Issue = internal.Issue
type Report = internal.Report
type SVGMeta = internal.SVGMeta
type StrokeWidthSummary = internal.StrokeWidthSummary
type TextShapeOverlap = internal.TextShapeOverlap

type Target = internal.Target
type MaterialTarget = internal.MaterialTarget

const (
	MaterialScreen      = internal.MaterialScreen
	MaterialPaper       = internal.MaterialPaper
	MaterialFabric      = internal.MaterialFabric
	MaterialVinyl       = internal.MaterialVinyl
	MaterialBanner      = internal.MaterialBanner
	MaterialSignage     = internal.MaterialSignage
	MaterialVehicleWrap = internal.MaterialVehicleWrap
	MaterialPackaging   = internal.MaterialPackaging
	MaterialLaser       = internal.MaterialLaser
	MaterialCNC         = internal.MaterialCNC
	MaterialPlotter     = internal.MaterialPlotter
)

type FixOptions = internal.FixOptions
type FixResult = internal.FixResult
type FixCategory = internal.FixCategory

const (
	FixCategoryAll        = internal.FixCategoryAll
	FixCategoryMetadata   = internal.FixCategoryMetadata
	FixCategorySafety     = internal.FixCategorySafety
	FixCategoryBleed      = internal.FixCategoryBleed
	FixCategoryEffects    = internal.FixCategoryEffects
	FixCategoryRaster     = internal.FixCategoryRaster
	FixCategoryReferences = internal.FixCategoryReferences
	FixCategoryColors     = internal.FixCategoryColors
	FixCategoryStrokes    = internal.FixCategoryStrokes
	FixCategoryGeometry   = internal.FixCategoryGeometry
	FixCategoryTypography = internal.FixCategoryTypography
	FixCategoryDetail     = internal.FixCategoryDetail
	FixCategorySizing     = internal.FixCategorySizing
	FixCategoryCutter     = internal.FixCategoryCutter
)

type OverlayOptions = internal.OverlayOptions

// CheckFile reads an SVG file from path and returns a preflight report.
func CheckFile(path string, rawTarget string) (Report, error) {
	return internal.CheckFile(path, rawTarget)
}

// Check inspects SVG bytes and returns a preflight report for the target.
func Check(input []byte, rawTarget string) (Report, error) {
	return internal.Check(input, rawTarget)
}

// GenerateOverlay returns an annotated SVG overlay for locatable findings.
func GenerateOverlay(input []byte, opts OverlayOptions) ([]byte, error) {
	return internal.GenerateOverlay(input, opts)
}

// Fix applies conservative SVG repairs, plus unsafe repairs when requested.
func Fix(input []byte, opts FixOptions) (FixResult, error) {
	return internal.Fix(input, opts)
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
	return internal.ParseTarget(raw)
}
