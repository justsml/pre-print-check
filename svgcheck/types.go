package svgcheck

import "fmt"

type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)

type FindingRank string

const (
	RankHigh     FindingRank = "high"
	RankModerate FindingRank = "moderate"
	RankLow      FindingRank = "low"
)

type Issue struct {
	Severity       Severity
	Code           string
	Message        string
	Rank           FindingRank
	FixCategory    FixCategory
	UnsafeRequired bool
	AutomaticFix   bool
}

type Report struct {
	Path   string
	Target Target
	Meta   SVGMeta
	Issues []Issue
}

func (r Report) HasErrors() bool {
	for _, issue := range r.Issues {
		if issue.Severity == SeverityError {
			return true
		}
	}
	return false
}

func (r Report) Summary() string {
	if r.Meta.Width != "" || r.Meta.Height != "" || r.Meta.ViewBox != "" {
		return fmt.Sprintf("SVG: width=%q height=%q viewBox=%q", r.Meta.Width, r.Meta.Height, r.Meta.ViewBox)
	}
	return "SVG metadata unavailable"
}

func (r Report) FriendlySummary() string {
	errors, warnings, info := r.IssueCounts()
	switch {
	case errors > 0:
		return fmt.Sprintf("Needs attention before production: %d error(s), %d warning(s), %d note(s).", errors, warnings, info)
	case warnings > 0:
		return fmt.Sprintf("Looks close, with %d warning(s) and %d note(s) worth reviewing.", warnings, info)
	case info > 0:
		return fmt.Sprintf("Looks usable, with %d informational note(s).", info)
	default:
		return "Looks print-ready for the checks this tool can run."
	}
}

func (r Report) IssueCounts() (errors, warnings, info int) {
	for _, issue := range r.Issues {
		switch issue.Severity {
		case SeverityError:
			errors++
		case SeverityWarning:
			warnings++
		case SeverityInfo:
			info++
		}
	}
	return errors, warnings, info
}

type SVGMeta struct {
	FoundSVG               bool
	Width                  string
	Height                 string
	WidthPixels            float64
	HeightPixels           float64
	ViewBox                string
	HasXMLNS               bool
	Scripts                int
	EventAttrs             int
	ExternalRefs           int
	RasterImages           int
	InlineRasterImages     int
	TextElements           int
	Filters                int
	FilterRefs             int
	Shadows                int
	Masks                  int
	ClipPaths              int
	Opacity                int
	BlendModes             int
	ThinStrokes            int
	ThinStrokeSummaries    []StrokeWidthSummary
	NearDisconnected       int
	TextShapeOverlaps      []TextShapeOverlap
	SmallShapesSub1MM      int
	SmallShapesSub2MM      int
	SubtleEffects          int
	LargeShadows           int
	BackgroundTransparency int
	MissingBleedShapes     int
	SafeAreaRiskShapes     int
	ColorValues            int
	UniqueColors           int
	CMYKColors             int
}

type StrokeWidthSummary struct {
	Width string
	Count int
}

type TextShapeOverlap struct {
	Text       string
	ShapeType  string
	ShapeCount int
}

type Target struct {
	Raw          string
	Material     MaterialTarget
	WidthInches  float64
	HeightInches float64
	PixelsWide   int
	PixelsHigh   int
}

func (t Target) Description() string {
	if t.Material != "" {
		if t.WidthInches > 0 {
			return fmt.Sprintf("%s (%s, %.2f in wide)", t.Raw, t.Material.Description(), t.WidthInches)
		}
		return fmt.Sprintf("%s (%s, assuming S/M/L outputs)", t.Raw, t.Material.Description())
	}
	if t.PixelsWide > 0 {
		return fmt.Sprintf("%s (%dx%d px)", t.Raw, t.PixelsWide, t.PixelsHigh)
	}
	if t.WidthInches > 0 {
		return fmt.Sprintf("%s (%.2f in wide)", t.Raw, t.WidthInches)
	}
	return t.Raw
}

func (t Target) PhysicalWidthsInches() []float64 {
	if t.WidthInches > 0 {
		return []float64{t.WidthInches}
	}
	if t.Material.NeedsPhysicalSize() {
		return []float64{3, 8, 14}
	}
	return nil
}

type MaterialTarget string

const (
	MaterialScreen      MaterialTarget = "screen"
	MaterialPaper       MaterialTarget = "paper"
	MaterialFabric      MaterialTarget = "fabric"
	MaterialVinyl       MaterialTarget = "vinyl"
	MaterialBanner      MaterialTarget = "banner"
	MaterialSignage     MaterialTarget = "signage"
	MaterialVehicleWrap MaterialTarget = "vehicle-wrap"
	MaterialPackaging   MaterialTarget = "packaging"
	MaterialLaser       MaterialTarget = "laser"
	MaterialCNC         MaterialTarget = "cnc"
	MaterialPlotter     MaterialTarget = "plotter"
)

func (m MaterialTarget) Description() string {
	switch m {
	case MaterialScreen:
		return "screen/web output"
	case MaterialPaper:
		return "paper print output"
	case MaterialFabric:
		return "fabric/textile output"
	case MaterialVinyl:
		return "vinyl/decal output"
	case MaterialBanner:
		return "large-format banner output"
	case MaterialSignage:
		return "rigid signage output"
	case MaterialVehicleWrap:
		return "vehicle wrap output"
	case MaterialPackaging:
		return "packaging/label output"
	case MaterialLaser:
		return "laser cut or engraving output"
	case MaterialCNC:
		return "CNC/router output"
	case MaterialPlotter:
		return "plotter/cutter output"
	default:
		return string(m)
	}
}

func (m MaterialTarget) NeedsPhysicalSize() bool {
	switch m {
	case MaterialPaper, MaterialFabric, MaterialVinyl, MaterialBanner, MaterialSignage, MaterialVehicleWrap, MaterialPackaging:
		return true
	default:
		return false
	}
}

func (m MaterialTarget) NeedsPureVectorGeometry() bool {
	switch m {
	case MaterialVinyl, MaterialLaser, MaterialCNC, MaterialPlotter:
		return true
	default:
		return false
	}
}

type FixOptions struct {
	Target     string
	Unsafe     bool
	Categories []string
}

type FixResult struct {
	SVG     []byte
	Changes []string
	Skipped []string
}

type FixCategory string

const (
	FixCategoryAll        FixCategory = "all"
	FixCategoryMetadata   FixCategory = "metadata"
	FixCategorySafety     FixCategory = "safety"
	FixCategoryBleed      FixCategory = "bleed"
	FixCategoryEffects    FixCategory = "effects"
	FixCategoryRaster     FixCategory = "raster"
	FixCategoryReferences FixCategory = "references"
	FixCategoryColors     FixCategory = "colors"
	FixCategoryStrokes    FixCategory = "strokes"
	FixCategoryGeometry   FixCategory = "geometry"
	FixCategoryTypography FixCategory = "typography"
	FixCategoryDetail     FixCategory = "detail"
	FixCategorySizing     FixCategory = "sizing"
	FixCategoryCutter     FixCategory = "cutter"
)

type OverlayOptions struct {
	Target string
}
