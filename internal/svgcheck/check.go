package svgcheck

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
)

type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)

type Issue struct {
	Severity Severity
	Code     string
	Message  string
}

type Report struct {
	Path   string
	Target Target
	Meta   SVGMeta
	Issues []Issue
}

type SVGMeta struct {
	FoundSVG     bool
	Width        string
	Height       string
	WidthPixels  float64
	HeightPixels float64
	ViewBox      string
	HasXMLNS     bool
	Scripts      int
	EventAttrs   int
	ExternalRefs int
	RasterImages int
	TextElements int
	Filters      int
	Masks        int
	ClipPaths    int
}

func CheckFile(path string, rawTarget string) (Report, error) {
	input, err := os.ReadFile(path)
	if err != nil {
		return Report{}, fmt.Errorf("read %s: %w", path, err)
	}
	report, err := Check(input, rawTarget)
	report.Path = path
	return report, err
}

func Check(input []byte, rawTarget string) (Report, error) {
	target, err := ParseTarget(rawTarget)
	if err != nil {
		return Report{}, err
	}

	meta, err := inspect(input)
	if err != nil {
		return Report{}, err
	}

	report := Report{Target: target, Meta: meta}
	report.addCoreIssues()
	report.addTargetIssues()
	return report, nil
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

func (r *Report) addCoreIssues() {
	if !r.Meta.HasXMLNS {
		r.addIssue(SeverityWarning, "missing-xmlns", "root <svg> is missing xmlns=\"http://www.w3.org/2000/svg\"")
	}
	if r.Meta.ViewBox == "" {
		r.addIssue(SeverityWarning, "missing-viewbox", "root <svg> is missing a viewBox; scaling may be inconsistent")
	}
	if r.Meta.Width == "" || r.Meta.Height == "" {
		r.addIssue(SeverityInfo, "missing-size", "root <svg> does not declare both width and height")
	}
	if r.Meta.Scripts > 0 {
		r.addIssue(SeverityError, "script", "SVG contains script elements; unsafe for many print/web pipelines")
	}
	if r.Meta.EventAttrs > 0 {
		r.addIssue(SeverityError, "event-handler", "SVG contains inline event handler attributes")
	}
	if r.Meta.ExternalRefs > 0 {
		r.addIssue(SeverityWarning, "external-reference", "SVG references external resources that may not render offline or in print")
	}
	if r.Meta.RasterImages > 0 {
		r.addIssue(SeverityWarning, "raster-image", "SVG embeds or references raster images; verify effective resolution for the final size")
	}
}

func (r *Report) addTargetIssues() {
	if r.Target.Raw == "" {
		return
	}

	if r.Target.Material != "" {
		r.addMaterialIssues()
	}

	if r.Meta.WidthPixels == 0 {
		return
	}
	if r.Target.WidthInches > 0 {
		ppi := r.Meta.WidthPixels / r.Target.WidthInches
		switch {
		case ppi < 72:
			r.addIssue(SeverityWarning, "low-effective-ppi", fmt.Sprintf("width implies about %.1f px/in at target size", ppi))
		case ppi < 150:
			r.addIssue(SeverityInfo, "modest-effective-ppi", fmt.Sprintf("width implies about %.1f px/in at target size", ppi))
		}
	}

	if r.Target.PixelsWide > 0 && r.Meta.WidthPixels > float64(r.Target.PixelsWide)*1.5 {
		r.addIssue(SeverityInfo, "oversized-for-target", "SVG width is much larger than the target raster width")
	}
}

func (r *Report) addMaterialIssues() {
	material := r.Target.Material
	if material.NeedsPhysicalSize() && r.Target.WidthInches == 0 {
		r.addIssue(SeverityInfo, "target-size-recommended", "provide a physical size target as well when checking effective raster resolution")
	}

	if material.NeedsPureVectorGeometry() {
		if r.Meta.RasterImages > 0 {
			r.addIssue(SeverityWarning, "raster-not-cuttable", "cutting and engraving targets usually need paths, not raster image elements")
		}
		if r.Meta.TextElements > 0 {
			r.addIssue(SeverityWarning, "text-not-outlined", "convert text to outlines/paths before sending to cutters or engravers")
		}
		if r.Meta.Filters > 0 || r.Meta.Masks > 0 || r.Meta.ClipPaths > 0 {
			r.addIssue(SeverityWarning, "effects-may-not-output", "filters, masks, and clipping paths may not survive cutter/engraver workflows")
		}
	}

	switch material {
	case MaterialFabric:
		if r.Meta.Filters > 0 {
			r.addIssue(SeverityInfo, "fabric-effects", "soft effects such as filters may rasterize or separate poorly for fabric production")
		}
	case MaterialBanner, MaterialSignage, MaterialVehicleWrap:
		if r.Meta.RasterImages > 0 {
			r.addIssue(SeverityInfo, "large-format-raster", "verify embedded raster images at final viewing distance and production scale")
		}
	case MaterialPackaging:
		if r.Meta.ExternalRefs > 0 {
			r.addIssue(SeverityWarning, "packaging-external-reference", "package artwork should be self-contained for handoff and archiving")
		}
	}
}

func (r *Report) addIssue(severity Severity, code, message string) {
	r.Issues = append(r.Issues, Issue{
		Severity: severity,
		Code:     code,
		Message:  message,
	})
}

func inspect(input []byte) (SVGMeta, error) {
	decoder := xml.NewDecoder(bytes.NewReader(input))
	meta := SVGMeta{}

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return meta, fmt.Errorf("invalid SVG XML: %w", err)
		}

		switch tok := token.(type) {
		case xml.StartElement:
			name := strings.ToLower(tok.Name.Local)
			if name == "svg" && !meta.FoundSVG {
				meta.FoundSVG = true
				for _, attr := range tok.Attr {
					switch attr.Name.Local {
					case "width":
						meta.Width = attr.Value
						meta.WidthPixels = parseSVGLengthPixels(attr.Value)
					case "height":
						meta.Height = attr.Value
						meta.HeightPixels = parseSVGLengthPixels(attr.Value)
					case "viewBox":
						meta.ViewBox = attr.Value
					case "xmlns":
						if attr.Value == "http://www.w3.org/2000/svg" {
							meta.HasXMLNS = true
						}
					}
				}
			}

			switch name {
			case "script":
				meta.Scripts++
			case "image":
				meta.RasterImages++
			case "text":
				meta.TextElements++
			case "filter":
				meta.Filters++
			case "mask":
				meta.Masks++
			case "clippath":
				meta.ClipPaths++
			}

			for _, attr := range tok.Attr {
				attrName := strings.ToLower(attr.Name.Local)
				if strings.HasPrefix(attrName, "on") {
					meta.EventAttrs++
				}
				if referencesExternalResource(attr.Value) {
					meta.ExternalRefs++
				}
			}
		}
	}

	if !meta.FoundSVG {
		return meta, fmt.Errorf("no root <svg> element found")
	}
	return meta, nil
}

var lengthPattern = regexp.MustCompile(`^\s*([0-9]+(?:\.[0-9]+)?)\s*([a-zA-Z%]*)\s*$`)

func parseSVGLengthPixels(value string) float64 {
	matches := lengthPattern.FindStringSubmatch(value)
	if matches == nil {
		return 0
	}
	n, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0
	}
	switch strings.ToLower(matches[2]) {
	case "", "px":
		return n
	case "in":
		return n * 96
	case "ft":
		return n * 12 * 96
	case "cm":
		return n * 96 / 2.54
	case "mm":
		return n * 96 / 25.4
	case "pt":
		return n * 96 / 72
	case "pc":
		return n * 16
	default:
		return 0
	}
}

func referencesExternalResource(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "//")
}
