package svgcheck

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type Target struct {
	Raw          string
	Material     MaterialTarget
	WidthInches  float64
	HeightInches float64
	PixelsWide   int
	PixelsHigh   int
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

var physicalTargetPattern = regexp.MustCompile(`(?i)^\s*([0-9]+(?:\.[0-9]+)?)\s*(in|inch|inches|ft|foot|feet|m|meter|meters|cm|centimeter|centimeters|mm|millimeter|millimeters)\s*$`)

func ParseTarget(raw string) (Target, error) {
	t := Target{Raw: strings.TrimSpace(raw)}
	if t.Raw == "" {
		return t, nil
	}

	lower := strings.ToLower(t.Raw)
	switch lower {
	case "4k":
		t.PixelsWide = 3840
		t.PixelsHigh = 2160
		return t, nil
	case "8k":
		t.PixelsWide = 7680
		t.PixelsHigh = 4320
		return t, nil
	}

	if material, ok := parseMaterialTarget(lower); ok {
		t.Material = material
		return t, nil
	}

	matches := physicalTargetPattern.FindStringSubmatch(t.Raw)
	if matches == nil {
		return t, fmt.Errorf("unsupported target %q", raw)
	}

	value, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return t, fmt.Errorf("invalid target %q", raw)
	}

	inches := value
	switch strings.ToLower(matches[2]) {
	case "ft", "foot", "feet":
		inches = value * 12
	case "m", "meter", "meters":
		inches = value * 39.3700787402
	case "cm", "centimeter", "centimeters":
		inches = value * 0.3937007874
	case "mm", "millimeter", "millimeters":
		inches = value * 0.03937007874
	}

	t.WidthInches = inches
	return t, nil
}

func (t Target) Description() string {
	if t.Material != "" {
		return fmt.Sprintf("%s (%s)", t.Raw, t.Material.Description())
	}
	if t.PixelsWide > 0 {
		return fmt.Sprintf("%s (%dx%d px)", t.Raw, t.PixelsWide, t.PixelsHigh)
	}
	if t.WidthInches > 0 {
		return fmt.Sprintf("%s (%.2f in wide)", t.Raw, t.WidthInches)
	}
	return t.Raw
}

func parseMaterialTarget(raw string) (MaterialTarget, bool) {
	switch strings.TrimSpace(raw) {
	case "screen", "web", "digital", "display":
		return MaterialScreen, true
	case "paper", "print", "poster", "flyer", "card", "business-card":
		return MaterialPaper, true
	case "fabric", "textile", "apparel", "shirt", "t-shirt", "tshirt", "dtg", "screenprint", "screen-print":
		return MaterialFabric, true
	case "vinyl", "sticker", "stickers", "decal", "decals", "cut-vinyl", "adhesive":
		return MaterialVinyl, true
	case "banner", "banners", "mesh-banner":
		return MaterialBanner, true
	case "sign", "signs", "signage":
		return MaterialSignage, true
	case "wrap", "vehicle", "vehicle-wrap", "car-wrap", "truck-wrap":
		return MaterialVehicleWrap, true
	case "packaging", "package", "label", "labels":
		return MaterialPackaging, true
	case "laser", "laser-cut", "laser-engrave", "engrave", "engraving":
		return MaterialLaser, true
	case "cnc", "router", "routing", "mill", "milling":
		return MaterialCNC, true
	case "plotter", "cut-plotter", "vinyl-cutter":
		return MaterialPlotter, true
	default:
		return "", false
	}
}

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
