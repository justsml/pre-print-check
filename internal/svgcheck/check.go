package svgcheck

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"math"
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
	ThinStrokes        int
	NearDisconnected   int
	ColorValues        int
	UniqueColors       int
	CMYKColors         int
}

type issueProfile struct {
	Material                  MaterialTarget
	ReviewArtworkComplexity   bool
	ReviewRasterResolution    bool
	WarnExternalReferences    bool
	RequireSelfContained      bool
	RequirePrintColors        bool
	RequireFlattenedEffects   bool
	ReviewEffects             bool
	ReviewThinStrokes         bool
	ReviewDisconnectedJoins   bool
	ReviewFabricColorCount    bool
	RequirePureVectorGeometry bool
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
	profile := issueProfileForTarget(target)
	report.addCoreIssues(profile)
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

func (r *Report) addCoreIssues(profile issueProfile) {
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
	if r.Meta.ExternalRefs > 0 && profile.WarnExternalReferences {
		r.addIssue(SeverityWarning, "external-reference", "SVG references external resources that may not render offline or in print")
	}
	if r.Meta.RasterImages > 0 && profile.ReviewRasterResolution && !profile.RequirePureVectorGeometry {
		r.addRankedIssue(SeverityWarning, "raster-image", "SVG embeds or references raster images; verify effective resolution for the final size", rankRasterImages(r.Meta.RasterImages))
	}
	if r.Meta.InlineRasterImages > 0 && profile.ReviewRasterResolution {
		r.addRankedIssue(SeverityWarning, "inline-raster-image", "SVG includes base64/data URI raster images; inspect them closely because resolution and color space are opaque to downstream print tools", rankRasterImages(r.Meta.InlineRasterImages))
	}
	if r.Meta.UniqueColors > 0 && profile.ReviewArtworkComplexity {
		r.addRankedIssue(SeverityInfo, "color-count", fmt.Sprintf("SVG uses about %d unique color value(s); many colors can complicate separations, spot-color conversion, and production review", r.Meta.UniqueColors), rankColorCount(r.Meta.UniqueColors))
	}
	if r.Meta.Shadows > 0 && profile.ReviewEffects {
		r.addRankedIssue(SeverityWarning, "shadow-effect", "SVG appears to use shadow-style effects; these often rasterize, flatten, or separate unpredictably in print workflows", RankHigh)
	}
}

func (r *Report) addTargetIssues() {
	if r.Target.Raw == "" {
		return
	}

	profile := issueProfileForTarget(r.Target)
	if r.Meta.ThinStrokes > 0 && profile.ReviewThinStrokes {
		r.addRankedIssue(SeverityWarning, "thin-stroke", "SVG contains very thin strokes that may disappear, break up, or image unpredictably in print production", RankModerate)
	}
	if r.Meta.NearDisconnected > 0 && profile.ReviewDisconnectedJoins {
		r.addRankedIssue(SeverityWarning, "near-disconnected-lines", "SVG has stroked open line/path endpoints that visually read as connected but are not joined; use a polygon/closed path or join the nodes to avoid visible gaps or awkward overlaps at production scale", rankNearDisconnected(r.Meta.NearDisconnected))
	}

	if r.Target.Material != "" {
		r.addMaterialIssues(profile)
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

func (r *Report) addMaterialIssues(profile issueProfile) {
	material := r.Target.Material
	if material.NeedsPhysicalSize() && r.Target.WidthInches == 0 {
		r.addIssue(SeverityInfo, "target-size-recommended", "provide a physical size target as well when checking effective raster resolution")
	}

	if profile.RequirePureVectorGeometry {
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

	if profile.RequirePrintColors {
		if r.Meta.ColorValues > 0 && r.Meta.CMYKColors == 0 {
			r.addRankedIssue(SeverityWarning, "rgb-colors-for-print", "SVG color values are RGB/web oriented; convert and proof in the printer's required CMYK or spot-color workflow before press", RankHigh)
		}
		if r.Meta.CMYKColors > 0 {
			r.addIssue(SeverityWarning, "cmyk-in-svg", "CMYK-like color values were found, but SVG support is inconsistent; confirm the final PDF or RIP preserves intended print colors")
		}
	}

	if profile.RequireFlattenedEffects {
		if r.Meta.Filters > 0 || r.Meta.FilterRefs > 0 || r.Meta.BlendModes > 0 {
			r.addRankedIssue(SeverityError, "print-effects-require-flattening", "filters, filter references, and blend modes should be flattened or proofed in a press-ready PDF workflow", RankHigh)
		}
	}

	switch material {
	case MaterialFabric:
		if r.Meta.Filters > 0 {
			r.addRankedIssue(SeverityWarning, "fabric-effects", "soft effects such as filters may rasterize or separate poorly for fabric production", RankModerate)
		}
		if r.Meta.ColorValues > 0 && r.Meta.UniqueColors > 24 && profile.ReviewFabricColorCount {
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

	if profile.RequireSelfContained && r.Meta.ExternalRefs > 0 {
		r.addIssue(SeverityWarning, "packaging-external-reference", "package artwork should be self-contained for handoff and archiving")
	}
}

func issueProfileForTarget(target Target) issueProfile {
	profile := issueProfile{
		Material:                target.Material,
		ReviewArtworkComplexity: target.Raw == "" || target.WidthInches > 0,
		ReviewRasterResolution:  target.Raw == "" || target.WidthInches > 0,
		WarnExternalReferences:  target.Raw == "" || target.WidthInches > 0,
		ReviewEffects:           target.Raw == "" || target.WidthInches > 0,
		ReviewThinStrokes:       target.WidthInches > 0,
		ReviewDisconnectedJoins: target.WidthInches > 0,
	}

	switch target.Material {
	case MaterialScreen:
		profile.ReviewArtworkComplexity = false
		profile.ReviewRasterResolution = false
		profile.WarnExternalReferences = false
		profile.ReviewEffects = false
	case MaterialPaper:
		profile.ReviewArtworkComplexity = true
		profile.ReviewRasterResolution = true
		profile.WarnExternalReferences = true
		profile.RequirePrintColors = true
		profile.RequireFlattenedEffects = true
		profile.ReviewEffects = true
		profile.ReviewThinStrokes = true
		profile.ReviewDisconnectedJoins = true
	case MaterialPackaging:
		profile.ReviewArtworkComplexity = true
		profile.ReviewRasterResolution = true
		profile.WarnExternalReferences = true
		profile.RequireSelfContained = true
		profile.RequirePrintColors = true
		profile.RequireFlattenedEffects = true
		profile.ReviewEffects = true
		profile.ReviewThinStrokes = true
		profile.ReviewDisconnectedJoins = true
	case MaterialFabric:
		profile.ReviewArtworkComplexity = true
		profile.ReviewRasterResolution = true
		profile.WarnExternalReferences = true
		profile.ReviewEffects = true
		profile.ReviewThinStrokes = true
		profile.ReviewDisconnectedJoins = true
		profile.ReviewFabricColorCount = true
	case MaterialBanner, MaterialSignage, MaterialVehicleWrap:
		profile.ReviewArtworkComplexity = true
		profile.ReviewRasterResolution = true
		profile.WarnExternalReferences = true
		profile.ReviewEffects = true
		profile.ReviewThinStrokes = true
		profile.ReviewDisconnectedJoins = true
	case MaterialVinyl, MaterialLaser, MaterialCNC, MaterialPlotter:
		profile.ReviewArtworkComplexity = true
		profile.ReviewRasterResolution = true
		profile.WarnExternalReferences = true
		profile.ReviewEffects = true
		profile.ReviewThinStrokes = true
		profile.ReviewDisconnectedJoins = true
		profile.RequirePureVectorGeometry = true
	}

	return profile
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
	endpoints := []geometryEndpoint{}
	geometrySource := 0
	styleStack := []geometryStyle{defaultGeometryStyle()}

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
			currentStyle := inheritedGeometryStyle(styleStack[len(styleStack)-1], tok.Attr)
			styleStack = append(styleStack, currentStyle)
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
			endpoints = append(endpoints, endpointsFromElement(name, tok.Attr, currentStyle, &geometrySource)...)

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
		case xml.CharData:
			meta.ExternalRefs += countExternalCSSResourceRefs(string(tok))
		case xml.EndElement:
			if len(styleStack) > 1 {
				styleStack = styleStack[:len(styleStack)-1]
			}
		}
	}

	if !meta.FoundSVG {
		return meta, fmt.Errorf("no root <svg> element found")
	}
	meta.UniqueColors = len(colorSetFrom(input))
	meta.NearDisconnected = countNearDisconnectedEndpoints(endpoints)
	return meta, nil
}

var lengthPattern = regexp.MustCompile(`^\s*([0-9]*\.?[0-9]+)\s*([a-zA-Z%]*)\s*$`)

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

var externalCSSResourcePattern = regexp.MustCompile(`(?i)\burl\(\s*['"]?(?:https?:)?//`)

func countExternalCSSResourceRefs(value string) int {
	return len(externalCSSResourcePattern.FindAllString(value, -1))
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
	case "stroke-width":
		if strokeWidthLooksThin(value) {
			meta.ThinStrokes++
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

func strokeWidthLooksThin(value string) bool {
	pixels := parseSVGLengthPixels(value)
	return pixels > 0 && pixels < 0.35
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

func rankNearDisconnected(count int) FindingRank {
	switch {
	case count >= 6:
		return RankHigh
	case count >= 3:
		return RankModerate
	default:
		return RankLow
	}
}

type geometryEndpoint struct {
	x, y        float64
	source      int
	strokeWidth float64
}

type geometryStyle struct {
	stroke      string
	strokeWidth float64
}

func endpointsFromElement(name string, attrs []xml.Attr, style geometryStyle, sourceCounter *int) []geometryEndpoint {
	if !style.hasVisibleStroke() {
		return nil
	}

	attr := attrsByName(attrs)
	switch name {
	case "line":
		x1, ok1 := parseCoordinate(attr["x1"])
		y1, ok2 := parseCoordinate(attr["y1"])
		x2, ok3 := parseCoordinate(attr["x2"])
		y2, ok4 := parseCoordinate(attr["y2"])
		if ok1 && ok2 && ok3 && ok4 {
			source := nextGeometrySource(sourceCounter)
			return []geometryEndpoint{{x: x1, y: y1, source: source, strokeWidth: style.strokeWidth}, {x: x2, y: y2, source: source, strokeWidth: style.strokeWidth}}
		}
	case "polyline":
		return endpointsFromPointList(attr["points"], false, style, sourceCounter)
	case "polygon":
		return endpointsFromPointList(attr["points"], true, style, sourceCounter)
	case "path":
		return endpointsFromPathData(attr["d"], style, sourceCounter)
	}
	return nil
}

func nextGeometrySource(sourceCounter *int) int {
	(*sourceCounter)++
	return *sourceCounter
}

func attrsByName(attrs []xml.Attr) map[string]string {
	out := map[string]string{}
	for _, attr := range attrs {
		out[strings.ToLower(attr.Name.Local)] = attr.Value
	}
	return out
}

func parseCoordinate(value string) (float64, bool) {
	if lengthPattern.FindStringSubmatch(value) == nil {
		return 0, false
	}
	return parseSVGLengthPixels(value), true
}

func endpointsFromPointList(points string, closed bool, style geometryStyle, sourceCounter *int) []geometryEndpoint {
	values := pathNumberPattern.FindAllString(points, -1)
	if len(values) < 4 || len(values)%2 != 0 {
		return nil
	}
	first, okFirst := pointFromNumberStrings(values[0], values[1])
	last, okLast := pointFromNumberStrings(values[len(values)-2], values[len(values)-1])
	if !okFirst || !okLast {
		return nil
	}
	if closed || pointsNearlyEqual(first, last, connectedEndpointTolerance) {
		return nil
	}
	source := nextGeometrySource(sourceCounter)
	first.source = source
	last.source = source
	first.strokeWidth = style.strokeWidth
	last.strokeWidth = style.strokeWidth
	return []geometryEndpoint{first, last}
}

var pathNumberPattern = regexp.MustCompile(`[-+]?(?:\d*\.\d+|\d+\.?)(?:[eE][-+]?\d+)?`)
var pathTokenPattern = regexp.MustCompile(`[AaCcHhLlMmQqSsTtVvZz]|[-+]?(?:\d*\.\d+|\d+\.?)(?:[eE][-+]?\d+)?`)

func endpointsFromPathData(data string, style geometryStyle, sourceCounter *int) []geometryEndpoint {
	tokens := pathTokenPattern.FindAllString(data, -1)
	if len(tokens) == 0 {
		return nil
	}

	var endpoints []geometryEndpoint
	var cmd byte
	var cur, subpathStart geometryEndpoint
	var subpathSource int
	var subpathOpen bool
	var subpathStartSet bool

	for i := 0; i < len(tokens); {
		if isPathCommand(tokens[i]) {
			cmd = tokens[i][0]
			i++
			if cmd == 'Z' || cmd == 'z' {
				subpathOpen = false
				cur = subpathStart
				continue
			}
		}
		if cmd == 0 {
			break
		}

		switch cmd {
		case 'M', 'm':
			if subpathOpen {
				subpathStart.source = subpathSource
				cur.source = subpathSource
				subpathStart.strokeWidth = style.strokeWidth
				cur.strokeWidth = style.strokeWidth
				endpoints = append(endpoints, subpathStart, cur)
			}
			if i+1 >= len(tokens) {
				return endpoints
			}
			x, y, ok := parsePathPair(tokens[i], tokens[i+1])
			if !ok {
				return endpoints
			}
			if cmd == 'm' && subpathStartSet {
				x += cur.x
				y += cur.y
			}
			cur = geometryEndpoint{x: x, y: y}
			subpathStart = cur
			subpathSource = nextGeometrySource(sourceCounter)
			subpathOpen = true
			subpathStartSet = true
			i += 2
			if cmd == 'M' {
				cmd = 'L'
			} else {
				cmd = 'l'
			}
		case 'L', 'l', 'T', 't':
			if i+1 >= len(tokens) {
				return endpoints
			}
			x, y, ok := parsePathPair(tokens[i], tokens[i+1])
			if !ok {
				return endpoints
			}
			if isRelativePathCommand(cmd) {
				x += cur.x
				y += cur.y
			}
			cur = geometryEndpoint{x: x, y: y}
			i += 2
		case 'H', 'h':
			if i >= len(tokens) {
				return endpoints
			}
			x, ok := parsePathNumber(tokens[i])
			if !ok {
				return endpoints
			}
			if isRelativePathCommand(cmd) {
				x += cur.x
			}
			cur.x = x
			i++
		case 'V', 'v':
			if i >= len(tokens) {
				return endpoints
			}
			y, ok := parsePathNumber(tokens[i])
			if !ok {
				return endpoints
			}
			if isRelativePathCommand(cmd) {
				y += cur.y
			}
			cur.y = y
			i++
		case 'C', 'c':
			if i+5 >= len(tokens) {
				return endpoints
			}
			x, y, ok := parsePathPair(tokens[i+4], tokens[i+5])
			if !ok {
				return endpoints
			}
			if isRelativePathCommand(cmd) {
				x += cur.x
				y += cur.y
			}
			cur = geometryEndpoint{x: x, y: y}
			i += 6
		case 'S', 's', 'Q', 'q':
			if i+3 >= len(tokens) {
				return endpoints
			}
			x, y, ok := parsePathPair(tokens[i+2], tokens[i+3])
			if !ok {
				return endpoints
			}
			if isRelativePathCommand(cmd) {
				x += cur.x
				y += cur.y
			}
			cur = geometryEndpoint{x: x, y: y}
			i += 4
		case 'A', 'a':
			if i+6 >= len(tokens) {
				return endpoints
			}
			x, y, ok := parsePathPair(tokens[i+5], tokens[i+6])
			if !ok {
				return endpoints
			}
			if isRelativePathCommand(cmd) {
				x += cur.x
				y += cur.y
			}
			cur = geometryEndpoint{x: x, y: y}
			i += 7
		default:
			return endpoints
		}
	}

	if subpathOpen {
		subpathStart.source = subpathSource
		cur.source = subpathSource
		subpathStart.strokeWidth = style.strokeWidth
		cur.strokeWidth = style.strokeWidth
		endpoints = append(endpoints, subpathStart, cur)
	}
	return endpoints
}

func pointFromNumberStrings(xValue, yValue string) (geometryEndpoint, bool) {
	x, okX := parsePathNumber(xValue)
	y, okY := parsePathNumber(yValue)
	return geometryEndpoint{x: x, y: y}, okX && okY
}

func parsePathPair(xValue, yValue string) (float64, float64, bool) {
	x, okX := parsePathNumber(xValue)
	y, okY := parsePathNumber(yValue)
	return x, y, okX && okY
}

func parsePathNumber(value string) (float64, bool) {
	n, err := strconv.ParseFloat(value, 64)
	return n, err == nil
}

func isPathCommand(value string) bool {
	return len(value) == 1 && strings.ContainsAny(value, "AaCcHhLlMmQqSsTtVvZz")
}

func isRelativePathCommand(cmd byte) bool {
	return cmd >= 'a' && cmd <= 'z'
}

const (
	connectedEndpointTolerance   = 0.01
	nearEndpointMinDistance      = 0.05
	nearEndpointMaxDistance      = 12.0
	nearDisconnectedMinimumPairs = 2
)

func countNearDisconnectedEndpoints(endpoints []geometryEndpoint) int {
	pairs := nearDisconnectedEndpointPairs(endpoints)
	if len(pairs) < nearDisconnectedMinimumPairs {
		return 0
	}
	return len(pairs)
}

type nearEndpointPair struct {
	a, b     geometryEndpoint
	distance float64
}

func nearDisconnectedEndpointPairs(endpoints []geometryEndpoint) []nearEndpointPair {
	var pairs []nearEndpointPair
	seenSources := map[[2]int]struct{}{}
	nearestBySource := map[[2]int]nearEndpointPair{}

	for i := 0; i < len(endpoints); i++ {
		for j := i + 1; j < len(endpoints); j++ {
			if endpoints[i].source == endpoints[j].source {
				continue
			}
			sourceKey := orderedSourceKey(endpoints[i].source, endpoints[j].source)
			distance := endpointDistance(endpoints[i], endpoints[j])
			if !looksVisuallyConnectedButUnjoined(endpoints[i], endpoints[j], distance) {
				continue
			}
			pair := nearEndpointPair{a: endpoints[i], b: endpoints[j], distance: distance}
			current, ok := nearestBySource[sourceKey]
			if !ok || distance < current.distance {
				nearestBySource[sourceKey] = pair
			}
			seenSources[sourceKey] = struct{}{}
		}
	}

	for sourceKey := range seenSources {
		pairs = append(pairs, nearestBySource[sourceKey])
	}
	return pairs
}

func orderedSourceKey(a, b int) [2]int {
	if a < b {
		return [2]int{a, b}
	}
	return [2]int{b, a}
}

func endpointDistance(a, b geometryEndpoint) float64 {
	return math.Hypot(a.x-b.x, a.y-b.y)
}

func pointsNearlyEqual(a, b geometryEndpoint, tolerance float64) bool {
	return endpointDistance(a, b) <= tolerance
}

func looksVisuallyConnectedButUnjoined(a, b geometryEndpoint, distance float64) bool {
	if distance < nearEndpointMinDistance {
		return false
	}
	strokeWidth := math.Max(a.strokeWidth, b.strokeWidth)
	if strokeWidth <= 0 {
		return false
	}
	return distance <= math.Min(nearEndpointMaxDistance, strokeWidth*nearEndpointStrokeRatio)
}

const nearEndpointStrokeRatio = 0.75

func defaultGeometryStyle() geometryStyle {
	return geometryStyle{
		stroke:      "none",
		strokeWidth: 1,
	}
}

func (s geometryStyle) hasVisibleStroke() bool {
	return s.stroke != "" && strings.ToLower(strings.TrimSpace(s.stroke)) != "none" && s.strokeWidth > 0
}

func inheritedGeometryStyle(parent geometryStyle, attrs []xml.Attr) geometryStyle {
	style := parent
	attr := attrsByName(attrs)
	if value := styleValue(attr["style"], "stroke"); value != "" {
		style.stroke = value
	}
	if value := attr["stroke"]; value != "" {
		style.stroke = value
	}
	if value := styleValue(attr["style"], "stroke-width"); value != "" {
		style.strokeWidth = parseStrokeWidthOrDefault(value, style.strokeWidth)
	}
	if value := attr["stroke-width"]; value != "" {
		style.strokeWidth = parseStrokeWidthOrDefault(value, style.strokeWidth)
	}
	return style
}

func parseStrokeWidthOrDefault(value string, fallback float64) float64 {
	if lengthPattern.FindStringSubmatch(value) == nil {
		return fallback
	}
	return parseSVGLengthPixels(value)
}
