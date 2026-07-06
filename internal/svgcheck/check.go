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

type FindingRank string

const (
	RankHigh     FindingRank = "high"
	RankModerate FindingRank = "moderate"
	RankLow      FindingRank = "low"
)

type Issue struct {
	Severity Severity
	Code     string
	Message  string
	Rank     FindingRank
}

type Report struct {
	Path   string
	Target Target
	Meta   SVGMeta
	Issues []Issue
}

type SVGMeta struct {
	FoundSVG           bool
	Width              string
	Height             string
	WidthPixels        float64
	HeightPixels       float64
	ViewBox            string
	HasXMLNS           bool
	Scripts            int
	EventAttrs         int
	ExternalRefs       int
	RasterImages       int
	InlineRasterImages int
	TextElements       int
	Filters            int
	FilterRefs         int
	Shadows            int
	Masks              int
	ClipPaths          int
	Opacity            int
	BlendModes         int
	ColorValues        int
	UniqueColors       int
	CMYKColors         int
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
		r.addRankedIssue(SeverityWarning, "raster-image", "SVG embeds or references raster images; verify effective resolution for the final size", rankRasterImages(r.Meta.RasterImages))
	}
	if r.Meta.InlineRasterImages > 0 {
		r.addRankedIssue(SeverityWarning, "inline-raster-image", "SVG includes base64/data URI raster images; inspect them closely because resolution and color space are opaque to downstream print tools", rankRasterImages(r.Meta.InlineRasterImages))
	}
	if r.Meta.UniqueColors > 0 {
		r.addRankedIssue(SeverityInfo, "color-count", fmt.Sprintf("SVG uses about %d unique color value(s); many colors can complicate separations, spot-color conversion, and production review", r.Meta.UniqueColors), rankColorCount(r.Meta.UniqueColors))
	}
	if r.Meta.Shadows > 0 {
		r.addRankedIssue(SeverityWarning, "shadow-effect", "SVG appears to use shadow-style effects; these often rasterize, flatten, or separate unpredictably in print workflows", RankHigh)
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
			r.addRankedIssue(SeverityError, "raster-not-cuttable", "cutting and engraving targets need paths, not raster image elements", RankHigh)
		}
		if r.Meta.TextElements > 0 {
			r.addIssue(SeverityWarning, "text-not-outlined", "convert text to outlines/paths before sending to cutters or engravers")
		}
		if r.Meta.Filters > 0 || r.Meta.FilterRefs > 0 || r.Meta.Masks > 0 || r.Meta.ClipPaths > 0 || r.Meta.Opacity > 0 || r.Meta.BlendModes > 0 {
			r.addRankedIssue(SeverityError, "effects-may-not-output", "filters, masks, clipping paths, opacity, and blend modes are not reliable cutter/engraver geometry", RankHigh)
		}
	}

	switch material {
	case MaterialPaper, MaterialPackaging:
		if r.Meta.ColorValues > 0 && r.Meta.CMYKColors == 0 {
			r.addRankedIssue(SeverityWarning, "rgb-colors-for-print", "SVG color values are RGB/web oriented; convert and proof in the printer's required CMYK or spot-color workflow before press", RankHigh)
		}
		if r.Meta.CMYKColors > 0 {
			r.addIssue(SeverityWarning, "cmyk-in-svg", "CMYK-like color values were found, but SVG support is inconsistent; confirm the final PDF or RIP preserves intended print colors")
		}
		if r.Meta.Filters > 0 || r.Meta.FilterRefs > 0 || r.Meta.BlendModes > 0 {
			r.addRankedIssue(SeverityError, "print-effects-require-flattening", "filters, filter references, and blend modes should be flattened or proofed in a press-ready PDF workflow", RankHigh)
		}
	case MaterialFabric:
		if r.Meta.Filters > 0 {
			r.addRankedIssue(SeverityWarning, "fabric-effects", "soft effects such as filters may rasterize or separate poorly for fabric production", RankModerate)
		}
		if r.Meta.ColorValues > 0 && r.Meta.UniqueColors > 24 {
			r.addRankedIssue(SeverityWarning, "many-fabric-colors", "many color values can increase setup complexity for screen print, embroidery, vinyl, or spot-color textile workflows", rankColorCount(r.Meta.UniqueColors))
		}
	case MaterialBanner, MaterialSignage, MaterialVehicleWrap:
		if r.Meta.RasterImages > 0 {
			r.addRankedIssue(SeverityInfo, "large-format-raster", "verify embedded raster images at final viewing distance and production scale", rankRasterImages(r.Meta.RasterImages))
		}
		if r.Meta.Filters > 0 || r.Meta.FilterRefs > 0 {
			r.addRankedIssue(SeverityWarning, "large-format-effects", "filters and shadows may be rasterized by the RIP; proof them at production scale", RankModerate)
		}
	}

	if material == MaterialPackaging && r.Meta.ExternalRefs > 0 {
		r.addIssue(SeverityWarning, "packaging-external-reference", "package artwork should be self-contained for handoff and archiving")
	}
}

func (r *Report) addIssue(severity Severity, code, message string) {
	r.addRankedIssue(severity, code, message, "")
}

func (r *Report) addRankedIssue(severity Severity, code, message string, rank FindingRank) {
	r.Issues = append(r.Issues, Issue{
		Severity: severity,
		Code:     code,
		Message:  message,
		Rank:     rank,
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
				if elementLooksLikeShadow(tok) {
					meta.Shadows++
				}
			case "fedropshadow":
				meta.Shadows++
			case "mask":
				meta.Masks++
			case "clippath":
				meta.ClipPaths++
			}

			for _, attr := range tok.Attr {
				attrName := strings.ToLower(attr.Name.Local)
				attrValue := strings.TrimSpace(attr.Value)
				if strings.HasPrefix(attrName, "on") {
					meta.EventAttrs++
				}
				if attrName == "href" || attrName == "xlink:href" {
					if isInlineRasterRef(attrValue) {
						meta.InlineRasterImages++
					}
				}
				if isResourceReferenceAttr(attrName) && referencesExternalResource(attrValue) {
					meta.ExternalRefs++
				}
				inspectAttrForPrintSignals(attrName, attrValue, &meta)
			}
		}
	}

	if !meta.FoundSVG {
		return meta, fmt.Errorf("no root <svg> element found")
	}
	meta.UniqueColors = len(colorSetFrom(input))
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

func isResourceReferenceAttr(name string) bool {
	switch name {
	case "href", "src":
		return true
	default:
		return false
	}
}

func isInlineRasterRef(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(lower, "data:image/png") ||
		strings.HasPrefix(lower, "data:image/jpeg") ||
		strings.HasPrefix(lower, "data:image/jpg") ||
		strings.HasPrefix(lower, "data:image/gif") ||
		strings.HasPrefix(lower, "data:image/webp") ||
		strings.HasPrefix(lower, "data:image/tiff") ||
		strings.HasPrefix(lower, "data:image/bmp")
}

func inspectAttrForPrintSignals(name, value string, meta *SVGMeta) {
	lowerValue := strings.ToLower(value)
	switch name {
	case "fill", "stroke", "stop-color", "flood-color", "lighting-color", "color":
		if isLikelyColorValue(value) {
			meta.ColorValues++
		}
	case "style":
		inspectStyle(value, meta)
	case "filter":
		if value != "" && strings.ToLower(value) != "none" {
			meta.FilterRefs++
			if strings.Contains(lowerValue, "shadow") {
				meta.Shadows++
			}
		}
	case "opacity", "fill-opacity", "stroke-opacity", "stop-opacity":
		if value != "" && value != "1" && strings.ToLower(value) != "100%" {
			meta.Opacity++
		}
	}

	if strings.Contains(lowerValue, "device-cmyk") || strings.Contains(lowerValue, "cmyk(") || strings.Contains(lowerValue, "icc-color(") {
		meta.CMYKColors++
	}
	if strings.Contains(lowerValue, "drop-shadow") || strings.Contains(lowerValue, "box-shadow") || strings.Contains(lowerValue, "text-shadow") {
		meta.Shadows++
	}
	if strings.Contains(lowerValue, "mix-blend-mode") || strings.Contains(lowerValue, "background-blend-mode") {
		meta.BlendModes++
	}
}

func inspectStyle(style string, meta *SVGMeta) {
	for _, declaration := range strings.Split(style, ";") {
		parts := strings.SplitN(declaration, ":", 2)
		if len(parts) != 2 {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(parts[0]))
		value := strings.TrimSpace(parts[1])
		inspectAttrForPrintSignals(name, value, meta)
	}
}

func elementLooksLikeShadow(element xml.StartElement) bool {
	for _, attr := range element.Attr {
		value := strings.ToLower(attr.Value)
		if strings.Contains(value, "shadow") {
			return true
		}
	}
	return false
}

func isLikelyColorValue(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	if lower == "" || lower == "none" || lower == "currentcolor" || strings.HasPrefix(lower, "url(") {
		return false
	}
	return strings.HasPrefix(lower, "#") ||
		strings.HasPrefix(lower, "rgb(") ||
		strings.HasPrefix(lower, "rgba(") ||
		strings.HasPrefix(lower, "hsl(") ||
		strings.HasPrefix(lower, "hsla(") ||
		strings.HasPrefix(lower, "color(") ||
		strings.Contains(lower, "cmyk(") ||
		strings.Contains(lower, "icc-color(") ||
		isNamedSVGColor(lower)
}

var colorTokenPattern = regexp.MustCompile(`(?i)(#[0-9a-f]{3,8}\b|rgba?\([^)]+\)|hsla?\([^)]+\)|device-cmyk\([^)]+\)|cmyk\([^)]+\)|icc-color\([^)]+\))`)

func colorSetFrom(input []byte) map[string]struct{} {
	set := map[string]struct{}{}
	for _, match := range colorTokenPattern.FindAll(input, -1) {
		set[strings.ToLower(string(match))] = struct{}{}
	}
	return set
}

func isNamedSVGColor(value string) bool {
	switch value {
	case "black", "silver", "gray", "white", "maroon", "red", "purple", "fuchsia", "green", "lime", "olive", "yellow", "navy", "blue", "teal", "aqua", "orange":
		return true
	default:
		return false
	}
}

func rankRasterImages(count int) FindingRank {
	switch {
	case count >= 5:
		return RankHigh
	case count >= 2:
		return RankModerate
	default:
		return RankLow
	}
}

func rankColorCount(count int) FindingRank {
	switch {
	case count > 48:
		return RankHigh
	case count > 12:
		return RankModerate
	default:
		return RankLow
	}
}
