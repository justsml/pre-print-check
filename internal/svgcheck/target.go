package svgcheck

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type Target struct {
	Raw          string
	WidthInches  float64
	HeightInches float64
	PixelsWide   int
	PixelsHigh   int
}

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
	if t.PixelsWide > 0 {
		return fmt.Sprintf("%s (%dx%d px)", t.Raw, t.PixelsWide, t.PixelsHigh)
	}
	if t.WidthInches > 0 {
		return fmt.Sprintf("%s (%.2f in wide)", t.Raw, t.WidthInches)
	}
	return t.Raw
}
