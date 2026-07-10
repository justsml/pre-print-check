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

type inspectionDetails struct {
	endpoints []geometryEndpoint
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
	ReviewBleed               bool
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
	report, _, err := checkWithDetails(input, rawTarget)
	return report, err
}

func checkWithDetails(input []byte, rawTarget string) (Report, inspectionDetails, error) {
	target, err := ParseTarget(rawTarget)
	if err != nil {
		return Report{}, inspectionDetails{}, err
	}

	meta, details, err := inspectForTarget(input, target)
	if err != nil {
		return Report{}, inspectionDetails{}, err
	}

	report := Report{Target: target, Meta: meta}
	profile := issueProfileForTarget(target)
	report.addCoreIssues(profile)
	report.addTargetIssues()
	return report, details, nil
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
		r.addRankedIssue(SeverityWarning, "thin-stroke", thinStrokeMessage(r.Meta, r.Target), RankModerate)
	}
	if r.Meta.NearDisconnected > 0 && profile.ReviewDisconnectedJoins {
		r.addRankedIssue(SeverityWarning, "near-disconnected-lines", "SVG has stroked open line/path endpoints that visually read as connected but are not joined; use a polygon/closed path or join the nodes to avoid visible gaps or awkward overlaps at production scale", rankNearDisconnected(r.Meta.NearDisconnected))
	}
	if len(r.Meta.TextShapeOverlaps) > 0 && profile.ReviewArtworkComplexity {
		for _, overlap := range r.Meta.TextShapeOverlaps {
			r.addRankedIssue(SeverityWarning, "text-overlap-shapes", textOverlapMessage(overlap, r.Meta), RankModerate)
		}
	}
	if (r.Meta.SmallShapesSub1MM > 0 || r.Meta.SmallShapesSub2MM > 0) && profile.ReviewArtworkComplexity {
		r.addRankedIssue(SeverityWarning, "small-detail-durability", smallDetailMessage(r.Meta, r.Target), rankSmallDetails(r.Meta.SmallShapesSub1MM, r.Meta.SmallShapesSub2MM))
	}
	if r.Meta.BackgroundTransparency > 0 && (profile.RequirePrintColors || r.Target.WidthInches > 0) {
		r.addRankedIssue(SeverityWarning, "background-transparency", fmt.Sprintf("%s detected; flatten against the intended substrate or add an explicit opaque background before production proofing", plural(r.Meta.BackgroundTransparency, "background transparency issue", "background transparency issues")), RankModerate)
	}
	if r.Meta.MissingBleedShapes > 0 && profile.ReviewBleed {
		r.addRankedIssue(SeverityWarning, "missing-bleed", missingBleedMessage(r.Meta, r.Target), RankModerate)
	}
	if r.Meta.SafeAreaRiskShapes > 0 && profile.ReviewBleed {
		r.addRankedIssue(SeverityWarning, "safe-area-risk", safeAreaMessage(r.Meta, r.Target), rankSafeAreaRisk(r.Meta.SafeAreaRiskShapes))
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
		r.addIssue(SeverityInfo, "target-size-recommended", "no physical size was provided; assuming S/M/L production outputs at 3in, 8in, and 14in wide for size-sensitive checks")
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
			r.addRankedIssue(SeverityError, "print-effects-require-flattening", effectFlatteningMessage(r.Meta), RankHigh)
		}
	}

	switch material {
	case MaterialFabric:
		if r.Meta.Filters > 0 {
			r.addRankedIssue(SeverityWarning, "fabric-effects", effectFlatteningMessage(r.Meta), RankModerate)
		}
		if r.Meta.ColorValues > 0 && r.Meta.UniqueColors > 24 && profile.ReviewFabricColorCount {
			r.addRankedIssue(SeverityWarning, "many-fabric-colors", "many color values can increase setup complexity for screen print, embroidery, vinyl, or spot-color textile workflows", rankColorCount(r.Meta.UniqueColors))
		}
	case MaterialBanner, MaterialSignage, MaterialVehicleWrap:
		if r.Meta.RasterImages > 0 {
			r.addRankedIssue(SeverityInfo, "large-format-raster", "verify embedded raster images at final viewing distance and production scale", rankRasterImages(r.Meta.RasterImages))
		}
		if r.Meta.Filters > 0 || r.Meta.FilterRefs > 0 {
			r.addRankedIssue(SeverityWarning, "large-format-effects", effectFlatteningMessage(r.Meta), RankModerate)
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
		ReviewBleed:             target.WidthInches > 0,
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
		profile.ReviewBleed = true
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
		profile.ReviewBleed = true
	case MaterialFabric:
		profile.ReviewArtworkComplexity = true
		profile.ReviewRasterResolution = true
		profile.WarnExternalReferences = true
		profile.ReviewEffects = true
		profile.ReviewThinStrokes = true
		profile.ReviewDisconnectedJoins = true
		profile.ReviewBleed = true
		profile.ReviewFabricColorCount = true
	case MaterialBanner, MaterialSignage, MaterialVehicleWrap:
		profile.ReviewArtworkComplexity = true
		profile.ReviewRasterResolution = true
		profile.WarnExternalReferences = true
		profile.ReviewEffects = true
		profile.ReviewThinStrokes = true
		profile.ReviewDisconnectedJoins = true
		profile.ReviewBleed = true
	case MaterialVinyl, MaterialLaser, MaterialCNC, MaterialPlotter:
		profile.ReviewArtworkComplexity = true
		profile.ReviewRasterResolution = true
		profile.WarnExternalReferences = true
		profile.ReviewEffects = true
		profile.ReviewThinStrokes = true
		profile.ReviewDisconnectedJoins = true
		profile.RequirePureVectorGeometry = true
		if target.Material == MaterialVinyl {
			profile.ReviewBleed = true
		}
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
	meta, _, err := inspectWithOptions(input, Target{}, false)
	return meta, err
}

func inspectForTarget(input []byte, target Target) (SVGMeta, inspectionDetails, error) {
	return inspectWithOptions(input, target, true)
}

func inspectWithOptions(input []byte, target Target, productionDetails bool) (SVGMeta, inspectionDetails, error) {
	decoder := xml.NewDecoder(bytes.NewReader(input))
	meta := SVGMeta{}
	details := inspectionDetails{}
	colorSet := map[string]struct{}{}
	endpoints := []geometryEndpoint{}
	geometrySource := 0
	styleStack := []geometryStyle{defaultGeometryStyle()}
	var shapes []roughShape
	var polygons []roughShape
	var texts []roughText
	thinCounts := map[string]int{}
	currentText := (*textCapture)(nil)
	mmPerUnit := float64(0)

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return meta, details, fmt.Errorf("invalid SVG XML: %w", err)
		}

		switch tok := token.(type) {
		case xml.StartElement:
			name := strings.ToLower(tok.Name.Local)
			attrByName := attrsByName(tok.Attr)
			currentStyle := inheritedGeometryStyle(styleStack[len(styleStack)-1], attrByName)
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
				mmPerUnit = physicalMMPerSVGUnit(meta, target)
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
			endpoints = append(endpoints, endpointsFromElement(name, attrByName, currentStyle, &geometrySource)...)

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
				collectColorTokens(attrValue, colorSet)
			}

			if productionDetails {
				if isOverlayGeometryElement(name) && currentStyle.hasVisibleStroke() && strokeWidthLooksProductionThin(currentStyle.strokeWidth) {
					thinCounts[strokeWidthLabel(attrByName, currentStyle)]++
				}

				if b, ok := roughBBox(name, attrByName); ok {
					shapes = append(shapes, roughShape{kind: name, box: b})
					if name == "polygon" {
						polygons = append(polygons, roughShape{kind: name, box: b})
					}
					if mmPerUnit > 0 {
						maxMM := math.Max(b.width(), b.height()) * mmPerUnit
						if maxMM > 0 && maxMM < 1 {
							meta.SmallShapesSub1MM++
						}
						if maxMM > 0 && maxMM < 2 {
							meta.SmallShapesSub2MM++
						}
					}
				}

				if name == "text" {
					currentText = newTextCapture(attrByName, currentStyle)
				}
				if name == "fedropshadow" && largeShadowElement(attrByName) {
					meta.LargeShadows++
				}
				if backgroundTransparencyElement(name, attrByName, meta) {
					meta.BackgroundTransparency++
				}
			}
		case xml.CharData:
			meta.ExternalRefs += countExternalCSSResourceRefs(string(tok))
			if len(tok) <= maxColorCharDataScanBytes {
				collectColorTokens(string(tok), colorSet)
			}
			if productionDetails && currentText != nil {
				currentText.text.WriteString(string(tok))
			}
		case xml.EndElement:
			if productionDetails {
				name := strings.ToLower(tok.Name.Local)
				if name == "text" && currentText != nil {
					if text, ok := currentText.toRoughText(); ok {
						texts = append(texts, text)
					}
					currentText = nil
				}
			}
			if len(styleStack) > 1 {
				styleStack = styleStack[:len(styleStack)-1]
			}
		}
	}

	if !meta.FoundSVG {
		return meta, details, fmt.Errorf("no root <svg> element found")
	}
	meta.UniqueColors = len(colorSet)
	meta.NearDisconnected = countNearDisconnectedEndpoints(endpoints)
	details.endpoints = endpoints
	if productionDetails {
		meta.ThinStrokeSummaries = strokeSummaries(thinCounts)
		if len(meta.ThinStrokeSummaries) > 0 {
			meta.ThinStrokes = 0
			for _, summary := range meta.ThinStrokeSummaries {
				meta.ThinStrokes += summary.Count
			}
		}
		meta.TextShapeOverlaps = textPolygonOverlaps(texts, polygons)
		meta.SubtleEffects = subtleEffectCount(meta)
		meta.MissingBleedShapes, meta.SafeAreaRiskShapes = bleedAndSafeAreaRisks(meta, target, shapes, texts)
	}
	return meta, details, nil
}

type roughShape struct {
	kind string
	box  box
}

type roughText struct {
	text string
	box  box
}

type box struct {
	x1, y1, x2, y2 float64
}

func enrichProductionDetails(input []byte, target Target, meta *SVGMeta) {
	decoder := xml.NewDecoder(bytes.NewReader(input))
	styleStack := []geometryStyle{defaultGeometryStyle()}
	var shapes []roughShape
	var polygons []roughShape
	var texts []roughText
	thinCounts := map[string]int{}
	currentText := (*textCapture)(nil)
	mmPerUnit := physicalMMPerSVGUnit(*meta, target)

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return
		}

		switch tok := token.(type) {
		case xml.StartElement:
			name := strings.ToLower(tok.Name.Local)
			attr := attrsByName(tok.Attr)
			style := inheritedGeometryStyle(styleStack[len(styleStack)-1], attr)
			styleStack = append(styleStack, style)

			if isOverlayGeometryElement(name) && style.hasVisibleStroke() && strokeWidthLooksProductionThin(style.strokeWidth) {
				thinCounts[strokeWidthLabel(attr, style)]++
			}

			if b, ok := roughBBox(name, attr); ok {
				shapes = append(shapes, roughShape{kind: name, box: b})
				if name == "polygon" {
					polygons = append(polygons, roughShape{kind: name, box: b})
				}
				if mmPerUnit > 0 {
					maxMM := math.Max(b.width(), b.height()) * mmPerUnit
					if maxMM > 0 && maxMM < 1 {
						meta.SmallShapesSub1MM++
					}
					if maxMM > 0 && maxMM < 2 {
						meta.SmallShapesSub2MM++
					}
				}
			}

			if name == "text" {
				currentText = newTextCapture(attr, style)
			}
			if name == "fedropshadow" && largeShadowElement(attr) {
				meta.LargeShadows++
			}
			if backgroundTransparencyElement(name, attr, *meta) {
				meta.BackgroundTransparency++
			}
		case xml.CharData:
			if currentText != nil {
				currentText.text.WriteString(string(tok))
			}
		case xml.EndElement:
			name := strings.ToLower(tok.Name.Local)
			if name == "text" && currentText != nil {
				if text, ok := currentText.toRoughText(); ok {
					texts = append(texts, text)
				}
				currentText = nil
			}
			if len(styleStack) > 1 {
				styleStack = styleStack[:len(styleStack)-1]
			}
		}
	}

	meta.ThinStrokeSummaries = strokeSummaries(thinCounts)
	if len(meta.ThinStrokeSummaries) > 0 {
		meta.ThinStrokes = 0
		for _, summary := range meta.ThinStrokeSummaries {
			meta.ThinStrokes += summary.Count
		}
	}
	meta.TextShapeOverlaps = textPolygonOverlaps(texts, polygons)
	meta.SubtleEffects = subtleEffectCount(*meta)
	meta.MissingBleedShapes, meta.SafeAreaRiskShapes = bleedAndSafeAreaRisks(*meta, target, shapes, texts)
}

type textCapture struct {
	x, y     float64
	fontSize float64
	text     strings.Builder
}

func newTextCapture(attr map[string]string, style geometryStyle) *textCapture {
	x, okX := parseCoordinate(defaultString(attr["x"], "0"))
	y, okY := parseCoordinate(defaultString(attr["y"], "0"))
	if !okX || !okY {
		return nil
	}
	fontSize := parseFontSize(attr["font-size"], attr["style"])
	if fontSize <= 0 {
		fontSize = 16
	}
	_ = style
	return &textCapture{x: x, y: y, fontSize: fontSize}
}

func (t *textCapture) toRoughText() (roughText, bool) {
	content := strings.TrimSpace(t.text.String())
	if content == "" {
		return roughText{}, false
	}
	width := float64(len([]rune(content))) * t.fontSize * 0.55
	return roughText{
		text: content,
		box:  box{x1: t.x, y1: t.y - t.fontSize, x2: t.x + width, y2: t.y + t.fontSize*0.25},
	}, true
}

func roughBBox(name string, attr map[string]string) (box, bool) {
	switch name {
	case "rect":
		x, _ := parseCoordinate(defaultString(attr["x"], "0"))
		y, _ := parseCoordinate(defaultString(attr["y"], "0"))
		w, okW := parseCoordinate(attr["width"])
		h, okH := parseCoordinate(attr["height"])
		if okW && okH && w > 0 && h > 0 {
			return box{x1: x, y1: y, x2: x + w, y2: y + h}, true
		}
	case "circle":
		cx, okX := parseCoordinate(attr["cx"])
		cy, okY := parseCoordinate(attr["cy"])
		r, okR := parseCoordinate(attr["r"])
		if okX && okY && okR && r > 0 {
			return box{x1: cx - r, y1: cy - r, x2: cx + r, y2: cy + r}, true
		}
	case "ellipse":
		cx, okX := parseCoordinate(attr["cx"])
		cy, okY := parseCoordinate(attr["cy"])
		rx, okRX := parseCoordinate(attr["rx"])
		ry, okRY := parseCoordinate(attr["ry"])
		if okX && okY && okRX && okRY && rx > 0 && ry > 0 {
			return box{x1: cx - rx, y1: cy - ry, x2: cx + rx, y2: cy + ry}, true
		}
	case "line":
		x1, ok1 := parseCoordinate(attr["x1"])
		y1, ok2 := parseCoordinate(attr["y1"])
		x2, ok3 := parseCoordinate(attr["x2"])
		y2, ok4 := parseCoordinate(attr["y2"])
		if ok1 && ok2 && ok3 && ok4 {
			return boxFromPoints([]geometryEndpoint{{x: x1, y: y1}, {x: x2, y: y2}}), true
		}
	case "polygon", "polyline":
		points := pointsFromNumberList(attr["points"])
		if len(points) > 0 {
			return boxFromPoints(points), true
		}
	case "path":
		points := roughPathPoints(attr["d"])
		if len(points) > 0 {
			return boxFromPoints(points), true
		}
	}
	return box{}, false
}

func pointsFromNumberList(value string) []geometryEndpoint {
	values := pathNumbers(value)
	if len(values) < 2 {
		return nil
	}
	points := make([]geometryEndpoint, 0, len(values)/2)
	for i := 0; i+1 < len(values); i += 2 {
		points = append(points, geometryEndpoint{x: values[i], y: values[i+1]})
	}
	return points
}

func roughPathPoints(value string) []geometryEndpoint {
	values := pathNumbers(value)
	if len(values) < 2 {
		return nil
	}
	points := make([]geometryEndpoint, 0, len(values)/2)
	for i := 0; i+1 < len(values); i += 2 {
		points = append(points, geometryEndpoint{x: values[i], y: values[i+1]})
	}
	return points
}

func boxFromPoints(points []geometryEndpoint) box {
	b := box{x1: points[0].x, y1: points[0].y, x2: points[0].x, y2: points[0].y}
	for _, point := range points[1:] {
		b.x1 = math.Min(b.x1, point.x)
		b.y1 = math.Min(b.y1, point.y)
		b.x2 = math.Max(b.x2, point.x)
		b.y2 = math.Max(b.y2, point.y)
	}
	return b
}

func (b box) width() float64  { return math.Abs(b.x2 - b.x1) }
func (b box) height() float64 { return math.Abs(b.y2 - b.y1) }

func boxesOverlap(a, b box) bool {
	return a.x1 < b.x2 && a.x2 > b.x1 && a.y1 < b.y2 && a.y2 > b.y1
}

func textPolygonOverlaps(texts []roughText, polygons []roughShape) []TextShapeOverlap {
	var overlaps []TextShapeOverlap
	for _, text := range texts {
		count := 0
		for _, polygon := range polygons {
			if boxesOverlap(text.box, polygon.box) {
				count++
			}
		}
		if count > 0 {
			overlaps = append(overlaps, TextShapeOverlap{
				Text:       truncateText(text.text, 48),
				ShapeType:  "polygon",
				ShapeCount: count,
			})
		}
	}
	return overlaps
}

func bleedAndSafeAreaRisks(meta SVGMeta, target Target, shapes []roughShape, texts []roughText) (missingBleedShapes, safeAreaRiskShapes int) {
	if len(shapes) == 0 && len(texts) == 0 {
		return 0, 0
	}
	minX, minY, width, height := parseViewBoxOrDefault(overlayViewBox(meta))
	if width <= 0 || height <= 0 {
		return 0, 0
	}
	canvas := box{x1: minX, y1: minY, x2: minX + width, y2: minY + height}
	bleedMargin, safeMargin := productionMarginsInSVGUnits(meta, target, width, height)
	tolerance := mathMax(width, height) * 0.001
	if tolerance < 0.01 {
		tolerance = 0.01
	}

	for _, shape := range shapes {
		if boxIsEmpty(shape.box) {
			continue
		}
		background := backgroundLikeShape(shape, canvas, tolerance)
		if background && boxTouchesCanvasEdge(shape.box, canvas, tolerance) && !boxExtendsBeyondCanvas(shape.box, canvas, bleedMargin*0.5) {
			missingBleedShapes++
		}
		if !background && boxInsideCanvas(shape.box, canvas, tolerance) && boxNearCanvasEdge(shape.box, canvas, safeMargin) {
			safeAreaRiskShapes++
		}
	}

	for _, text := range texts {
		if boxIsEmpty(text.box) {
			continue
		}
		if boxInsideCanvas(text.box, canvas, tolerance) && boxNearCanvasEdge(text.box, canvas, safeMargin) {
			safeAreaRiskShapes++
		}
	}

	return missingBleedShapes, safeAreaRiskShapes
}

func productionMarginsInSVGUnits(meta SVGMeta, target Target, canvasWidth, canvasHeight float64) (bleedMargin, safeMargin float64) {
	const (
		commonBleedMM = 3.0
		commonSafeMM  = 5.0
	)
	if mmPerUnit := physicalMMPerSVGUnit(meta, target); mmPerUnit > 0 {
		return commonBleedMM / mmPerUnit, commonSafeMM / mmPerUnit
	}
	shortSide := math.Min(canvasWidth, canvasHeight)
	return shortSide * 0.025, shortSide * 0.04
}

func backgroundLikeShape(shape roughShape, canvas box, tolerance float64) bool {
	switch shape.kind {
	case "rect", "path", "polygon":
	default:
		return false
	}
	coverageWidth := shape.box.width() >= canvas.width()*0.9
	coverageHeight := shape.box.height() >= canvas.height()*0.9
	return coverageWidth && coverageHeight && boxTouchesCanvasEdge(shape.box, canvas, tolerance)
}

func boxTouchesCanvasEdge(b, canvas box, tolerance float64) bool {
	return math.Abs(b.x1-canvas.x1) <= tolerance ||
		math.Abs(b.y1-canvas.y1) <= tolerance ||
		math.Abs(b.x2-canvas.x2) <= tolerance ||
		math.Abs(b.y2-canvas.y2) <= tolerance
}

func boxExtendsBeyondCanvas(b, canvas box, minimum float64) bool {
	return b.x1 <= canvas.x1-minimum ||
		b.y1 <= canvas.y1-minimum ||
		b.x2 >= canvas.x2+minimum ||
		b.y2 >= canvas.y2+minimum
}

func boxNearCanvasEdge(b, canvas box, margin float64) bool {
	return b.x1-canvas.x1 < margin ||
		b.y1-canvas.y1 < margin ||
		canvas.x2-b.x2 < margin ||
		canvas.y2-b.y2 < margin
}

func boxInsideCanvas(b, canvas box, tolerance float64) bool {
	return b.x1 >= canvas.x1-tolerance &&
		b.y1 >= canvas.y1-tolerance &&
		b.x2 <= canvas.x2+tolerance &&
		b.y2 <= canvas.y2+tolerance
}

func boxIsEmpty(b box) bool {
	return b.width() == 0 && b.height() == 0
}

func physicalMMPerSVGUnit(meta SVGMeta, target Target) float64 {
	if meta.WidthPixels <= 0 {
		return 0
	}
	if widths := target.PhysicalWidthsInches(); len(widths) > 0 {
		return smallestFloat(widths) * 25.4 / meta.WidthPixels
	}
	if inches := parseSVGLengthInches(meta.Width); inches > 0 {
		return inches * 25.4 / meta.WidthPixels
	}
	return 0
}

func parseSVGLengthInches(value string) float64 {
	n, unit, ok := parseSVGLength(value)
	if !ok {
		return 0
	}
	switch strings.ToLower(unit) {
	case "in":
		return n
	case "ft":
		return n * 12
	case "cm":
		return n / 2.54
	case "mm":
		return n / 25.4
	case "pt":
		return n / 72
	case "pc":
		return n / 6
	default:
		return 0
	}
}

func parseFontSize(attrValue, style string) float64 {
	if attrValue == "" {
		attrValue = styleValue(style, "font-size")
	}
	return parseSVGLengthPixels(attrValue)
}

func strokeWidthLooksProductionThin(width float64) bool {
	return width > 0 && width <= 1.5
}

func strokeWidthLabel(attr map[string]string, style geometryStyle) string {
	if value := attr["stroke-width"]; value != "" {
		return value
	}
	if value := styleValue(attr["style"], "stroke-width"); value != "" {
		return value
	}
	return fmt.Sprintf("%s px", trimFloat(style.strokeWidth))
}

func strokeSummaries(counts map[string]int) []StrokeWidthSummary {
	summaries := make([]StrokeWidthSummary, 0, len(counts))
	for width, count := range counts {
		summaries = append(summaries, StrokeWidthSummary{Width: width, Count: count})
	}
	return summaries
}

func thinStrokeMessage(meta SVGMeta, target Target) string {
	if len(meta.ThinStrokeSummaries) > 0 {
		primary := meta.ThinStrokeSummaries[0]
		return fmt.Sprintf("There are %s with %s stroke width, which may not scale cleanly%s; consider thickening strokes, outlining them, or simplifying hairline detail for the production method", plural(primary.Count, "stroked line/path element", "stroked line/path elements"), primary.Width, targetPhrase(target))
	}
	return fmt.Sprintf("There are %s, which may disappear, break up, or image unpredictably%s; consider thickening strokes or converting critical strokes to outlined shapes", plural(meta.ThinStrokes, "very thin stroked element", "very thin stroked elements"), targetPhrase(target))
}

func targetPhrase(target Target) string {
	if target.WidthInches > 0 && target.Material != "" {
		return fmt.Sprintf(" at %.1fin on %s", target.WidthInches, target.Material)
	}
	if target.WidthInches > 0 {
		return fmt.Sprintf(" at %.1fin wide", target.WidthInches)
	}
	if target.Material != "" {
		if widths := target.PhysicalWidthsInches(); len(widths) > 0 {
			return fmt.Sprintf(" across assumed S/M/L outputs (%s, %s, and %s wide) on %s", trimFloat(widths[0])+"in", trimFloat(widths[1])+"in", trimFloat(widths[2])+"in", target.Material)
		}
		return fmt.Sprintf(" on %s", target.Material)
	}
	return ""
}

func textOverlapMessage(overlap TextShapeOverlap, meta SVGMeta) string {
	tonePhrase := "limited-color"
	if meta.UniqueColors > 0 && meta.UniqueColors <= 3 {
		tonePhrase = fmt.Sprintf("%d-tone", meta.UniqueColors)
	}
	return fmt.Sprintf("The text %q overlaps %s; it may not stay clear in %s printing. Add knockout/outline contrast, move the text, or merge the interaction intentionally in the production artwork", overlap.Text, plural(overlap.ShapeCount, overlap.ShapeType, overlap.ShapeType+"s"), tonePhrase)
}

func smallDetailMessage(meta SVGMeta, target Target) string {
	return fmt.Sprintf("Durability: design features many small elements: %s and %s%s. Such precise transfers can limit material choices; simplify tiny islands, enlarge detail, or choose a production method/material that can hold fine features", plural(meta.SmallShapesSub1MM, "sub-1mm shape", "sub-1mm shapes"), plural(meta.SmallShapesSub2MM, "sub-2mm shape", "sub-2mm shapes"), targetPhrase(target))
}

func missingBleedMessage(meta SVGMeta, target Target) string {
	return fmt.Sprintf("Bleed: %s reach the trim edge but do not extend beyond it%s. Add the printer-required bleed, commonly 0.125in/3mm, or confirm this is not intended as full-bleed artwork", plural(meta.MissingBleedShapes, "background/edge element", "background/edge elements"), targetPhrase(target))
}

func safeAreaMessage(meta SVGMeta, target Target) string {
	return fmt.Sprintf("Safe area: %s sit close to the trim edge%s. Keep text, logos, and critical detail inside the printer template's safe area, or intentionally extend background art past bleed", plural(meta.SafeAreaRiskShapes, "non-background element", "non-background elements"), targetPhrase(target))
}

func rankSafeAreaRisk(count int) FindingRank {
	switch {
	case count >= 10:
		return RankHigh
	case count >= 3:
		return RankModerate
	default:
		return RankLow
	}
}

func rankSmallDetails(sub1MM, sub2MM int) FindingRank {
	switch {
	case sub1MM >= 50 || sub2MM >= 300:
		return RankHigh
	case sub1MM >= 10 || sub2MM >= 75:
		return RankModerate
	default:
		return RankLow
	}
}

func effectFlatteningMessage(meta SVGMeta) string {
	subtle := meta.SubtleEffects
	if subtle == 0 {
		subtle = meta.Filters + meta.FilterRefs + meta.BlendModes + meta.Masks + meta.ClipPaths
	}
	largeShadow := meta.LargeShadows
	if largeShadow == 0 && meta.Shadows > 0 {
		largeShadow = 1
	}
	return fmt.Sprintf("%s and %s may degrade unevenly or require flattening, spot-white/underbase planning, or print methods/materials that support soft gradients and transparency", plural(subtle, "subtle effect", "subtle effects"), plural(largeShadow, "large shadow", "large shadows"))
}

func subtleEffectCount(meta SVGMeta) int {
	count := meta.Filters + meta.FilterRefs + meta.BlendModes + meta.Masks + meta.ClipPaths
	if count > meta.LargeShadows {
		return count - meta.LargeShadows
	}
	return count
}

func largeShadowElement(attr map[string]string) bool {
	stdDeviation := firstFloat(attr["stddeviation"])
	dx := math.Abs(firstFloat(attr["dx"]))
	dy := math.Abs(firstFloat(attr["dy"]))
	return stdDeviation >= 2 || dx >= 3 || dy >= 3
}

func firstFloat(value string) float64 {
	n, ok := firstPathNumber(value)
	if !ok {
		return 0
	}
	return n
}

func backgroundTransparencyElement(name string, attr map[string]string, meta SVGMeta) bool {
	if name != "rect" {
		return false
	}
	x, _ := parseCoordinate(defaultString(attr["x"], "0"))
	y, _ := parseCoordinate(defaultString(attr["y"], "0"))
	w, okW := parseCoordinate(attr["width"])
	h, okH := parseCoordinate(attr["height"])
	if !okW || !okH || x != 0 || y != 0 {
		return false
	}
	if meta.WidthPixels > 0 && w < meta.WidthPixels*0.9 {
		return false
	}
	if meta.HeightPixels > 0 && h < meta.HeightPixels*0.9 {
		return false
	}
	return opacityValue(attr["opacity"]) < 1 || opacityValue(attr["fill-opacity"]) < 1 || rgbaAlphaLessThanOne(attr["fill"])
}

func opacityValue(value string) float64 {
	if value == "" {
		return 1
	}
	value = strings.TrimSpace(value)
	if strings.HasSuffix(value, "%") {
		n, err := strconv.ParseFloat(strings.TrimSuffix(value, "%"), 64)
		if err != nil {
			return 1
		}
		return n / 100
	}
	n, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 1
	}
	return n
}

func rgbaAlphaLessThanOne(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	if !strings.HasPrefix(lower, "rgba(") {
		return false
	}
	values := pathNumbers(lower)
	if len(values) < 4 {
		return false
	}
	return values[3] < 1
}

func truncateText(value string, maxRunes int) string {
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes-1]) + "..."
}

func plural(count int, singular, plural string) string {
	if count == 1 {
		return fmt.Sprintf("1 %s", singular)
	}
	return fmt.Sprintf("%d %s", count, plural)
}

func smallestFloat(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	smallest := values[0]
	for _, value := range values[1:] {
		if value < smallest {
			smallest = value
		}
	}
	return smallest
}

func parseSVGLengthPixels(value string) float64 {
	pixels, _ := parseSVGLengthPixelsValue(value)
	return pixels
}

func parseSVGLengthPixelsValue(value string) (float64, bool) {
	n, unit, ok := parseSVGLength(value)
	if !ok {
		return 0, false
	}
	switch strings.ToLower(unit) {
	case "", "px":
		return n, true
	case "in":
		return n * 96, true
	case "ft":
		return n * 12 * 96, true
	case "cm":
		return n * 96 / 2.54, true
	case "mm":
		return n * 96 / 25.4, true
	case "pt":
		return n * 96 / 72, true
	case "pc":
		return n * 16, true
	default:
		return 0, false
	}
}

func parseSVGLength(value string) (float64, string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, "", false
	}

	i := 0
	digitsBeforeDot := 0
	for i < len(value) && value[i] >= '0' && value[i] <= '9' {
		i++
		digitsBeforeDot++
	}

	digitsAfterDot := 0
	if i < len(value) && value[i] == '.' {
		i++
		for i < len(value) && value[i] >= '0' && value[i] <= '9' {
			i++
			digitsAfterDot++
		}
	}
	if digitsBeforeDot == 0 && digitsAfterDot == 0 {
		return 0, "", false
	}
	if digitsAfterDot == 0 && i > 0 && value[i-1] == '.' {
		return 0, "", false
	}

	number := value[:i]
	for i < len(value) && (value[i] == ' ' || value[i] == '\t' || value[i] == '\n' || value[i] == '\r') {
		i++
	}
	unitStart := i
	for i < len(value) && ((value[i] >= 'a' && value[i] <= 'z') || (value[i] >= 'A' && value[i] <= 'Z') || value[i] == '%') {
		i++
	}
	if i != len(value) {
		return 0, "", false
	}

	n, err := strconv.ParseFloat(number, 64)
	if err != nil {
		return 0, "", false
	}
	return n, value[unitStart:i], true
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
	visitStyleDeclarations(style, func(rawName, value string) {
		name := strings.ToLower(rawName)
		inspectAttrForPrintSignals(name, value, meta)
	})
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

const maxColorCharDataScanBytes = 64 * 1024

func collectColorTokens(value string, set map[string]struct{}) {
	if !mayContainColorToken(value) {
		return
	}
	for _, match := range colorTokenPattern.FindAllString(value, -1) {
		set[strings.ToLower(match)] = struct{}{}
	}
}

func mayContainColorToken(value string) bool {
	for i := 0; i < len(value); i++ {
		switch asciiLower(value[i]) {
		case '#':
			return true
		case 'r':
			if hasASCIIInsensitivePrefix(value[i:], "rgb") {
				return true
			}
		case 'h':
			if hasASCIIInsensitivePrefix(value[i:], "hsl") {
				return true
			}
		case 'c':
			if hasASCIIInsensitivePrefix(value[i:], "cmyk") {
				return true
			}
		case 'i':
			if hasASCIIInsensitivePrefix(value[i:], "icc-color") {
				return true
			}
		}
	}
	return false
}

func hasASCIIInsensitivePrefix(value, prefix string) bool {
	if len(value) < len(prefix) {
		return false
	}
	for i := 0; i < len(prefix); i++ {
		if asciiLower(value[i]) != prefix[i] {
			return false
		}
	}
	return true
}

func asciiLower(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + ('a' - 'A')
	}
	return b
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

func endpointsFromElement(name string, attr map[string]string, style geometryStyle, sourceCounter *int) []geometryEndpoint {
	if !style.hasVisibleStroke() {
		return nil
	}

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
	return parseSVGLengthPixelsValue(value)
}

func endpointsFromPointList(points string, closed bool, style geometryStyle, sourceCounter *int) []geometryEndpoint {
	values := pathNumbers(points)
	if len(values) < 4 || len(values)%2 != 0 {
		return nil
	}
	first := geometryEndpoint{x: values[0], y: values[1]}
	last := geometryEndpoint{x: values[len(values)-2], y: values[len(values)-1]}
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

func pathNumbers(value string) []float64 {
	var numbers []float64
	for i := 0; i < len(value); {
		start, end, ok := pathNumberBounds(value, i)
		if !ok {
			i++
			continue
		}
		n, err := strconv.ParseFloat(value[start:end], 64)
		if err == nil {
			numbers = append(numbers, n)
		}
		i = end
	}
	return numbers
}

func firstPathNumber(value string) (float64, bool) {
	for i := 0; i < len(value); {
		start, end, ok := pathNumberBounds(value, i)
		if !ok {
			i++
			continue
		}
		n, err := strconv.ParseFloat(value[start:end], 64)
		return n, err == nil
	}
	return 0, false
}

func pathNumberBounds(value string, startAt int) (start, end int, ok bool) {
	i := startAt
	if i >= len(value) {
		return 0, 0, false
	}
	if value[i] == '+' || value[i] == '-' {
		i++
		if i >= len(value) {
			return 0, 0, false
		}
	}

	digitsBeforeDot := 0
	for i < len(value) && value[i] >= '0' && value[i] <= '9' {
		i++
		digitsBeforeDot++
	}

	digitsAfterDot := 0
	if i < len(value) && value[i] == '.' {
		i++
		for i < len(value) && value[i] >= '0' && value[i] <= '9' {
			i++
			digitsAfterDot++
		}
	}
	if digitsBeforeDot == 0 && digitsAfterDot == 0 {
		return 0, 0, false
	}

	if i < len(value) && (value[i] == 'e' || value[i] == 'E') {
		expStart := i
		i++
		if i < len(value) && (value[i] == '+' || value[i] == '-') {
			i++
		}
		expDigits := 0
		for i < len(value) && value[i] >= '0' && value[i] <= '9' {
			i++
			expDigits++
		}
		if expDigits == 0 {
			i = expStart
		}
	}

	return startAt, i, true
}

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

var pathTokenPattern = regexp.MustCompile(`[AaCcHhLlMmQqSsTtVvZz]|[-+]?(?:\d*\.\d+|\d+\.?)(?:[eE][-+]?\d+)?`)

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

func inheritedGeometryStyle(parent geometryStyle, attr map[string]string) geometryStyle {
	style := parent
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
	pixels, ok := parseSVGLengthPixelsValue(value)
	if !ok {
		return fallback
	}
	return pixels
}
