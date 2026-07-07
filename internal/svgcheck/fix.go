package svgcheck

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

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

func Fix(input []byte, opts FixOptions) (FixResult, error) {
	report, err := Check(input, opts.Target)
	if err != nil {
		return FixResult{}, err
	}
	categories, err := normalizeFixCategories(opts.Categories)
	if err != nil {
		return FixResult{}, err
	}

	out := input
	changes := []string{}
	skipped := []string{}

	var changed []string
	if categoryEnabled(categories, FixCategoryMetadata) {
		out, changed = fixMetadata(out)
		changes = append(changes, changed...)
	}

	meta, err := inspect(out)
	if err != nil {
		return FixResult{}, err
	}
	target, err := ParseTarget(opts.Target)
	if err != nil {
		return FixResult{}, err
	}
	enrichProductionDetails(out, target, &meta)

	if categoryEnabled(categories, FixCategoryBleed) {
		out, changed = fixBleed(out, target, meta)
		changes = append(changes, changed...)
	}

	if categoryEnabled(categories, FixCategorySafety) {
		if opts.Unsafe {
			out, changed = fixSafety(out)
			changes = append(changes, changed...)
		} else if meta.Scripts > 0 || meta.EventAttrs > 0 || reportHasAnyIssue(report, "script", "event-handler") {
			skipped = append(skipped, "safety fixes require --unsafe before removing scripts or inline event handlers")
		}
	}

	if categoryEnabled(categories, FixCategoryEffects) {
		if opts.Unsafe {
			out, changed = fixEffects(out)
			changes = append(changes, changed...)
		} else if meta.Filters > 0 || meta.FilterRefs > 0 || meta.Masks > 0 || meta.ClipPaths > 0 || meta.Opacity > 0 || meta.BlendModes > 0 || reportHasAnyIssue(report, "shadow-effect", "effects-may-not-output", "print-effects-require-flattening", "fabric-effects", "large-format-effects") {
			skipped = append(skipped, "effects fixes require --unsafe because removing filters, masks, opacity, or blend modes changes rendering")
		}
	}

	if categoryEnabled(categories, FixCategoryRaster) {
		if opts.Unsafe {
			out, changed = fixRaster(out)
			changes = append(changes, changed...)
		} else if meta.RasterImages > 0 || meta.InlineRasterImages > 0 || reportHasAnyIssue(report, "raster-image", "inline-raster-image", "raster-not-cuttable", "large-format-raster") {
			skipped = append(skipped, "raster fixes require --unsafe because removing embedded images changes artwork")
		}
	}

	skipped = append(skipped, advisoryFixNotes(report, categories)...)
	return FixResult{SVG: out, Changes: changes, Skipped: dedupeStrings(skipped)}, nil
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

var allFixCategories = []FixCategory{
	FixCategoryMetadata,
	FixCategorySafety,
	FixCategoryBleed,
	FixCategoryEffects,
	FixCategoryRaster,
	FixCategoryReferences,
	FixCategoryColors,
	FixCategoryStrokes,
	FixCategoryGeometry,
	FixCategoryTypography,
	FixCategoryDetail,
	FixCategorySizing,
	FixCategoryCutter,
}

func FixCategoryNames() []string {
	names := make([]string, 0, len(allFixCategories)+1)
	names = append(names, string(FixCategoryAll))
	for _, category := range allFixCategories {
		names = append(names, string(category))
	}
	return names
}

func ParseFixCategories(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return []string{string(FixCategoryAll)}, nil
	}
	parts, err := splitFixCategoryList(raw)
	if err != nil {
		return nil, err
	}
	categories := make([]string, 0, len(parts))
	for _, part := range parts {
		category, ok := normalizeFixCategory(part)
		if !ok {
			return nil, fmt.Errorf("unsupported fix category %q; supported categories: %s", part, strings.Join(FixCategoryNames(), ", "))
		}
		categories = append(categories, string(category))
	}
	if len(categories) == 0 {
		return []string{string(FixCategoryAll)}, nil
	}
	return categories, nil
}

func splitFixCategoryList(raw string) ([]string, error) {
	commaParts := strings.Split(raw, ",")
	var parts []string
	for _, commaPart := range commaParts {
		if strings.TrimSpace(commaPart) == "" {
			return nil, fmt.Errorf("empty fix category in %q", raw)
		}
		fields := strings.Fields(commaPart)
		if len(fields) == 0 {
			return nil, fmt.Errorf("empty fix category in %q", raw)
		}
		parts = append(parts, fields...)
	}
	return parts, nil
}

func normalizeFixCategories(raw []string) (map[FixCategory]bool, error) {
	if len(raw) == 0 {
		raw = []string{string(FixCategoryAll)}
	}
	enabled := map[FixCategory]bool{}
	for _, value := range raw {
		values, err := splitFixCategoryList(value)
		if err != nil {
			return nil, err
		}
		for _, splitValue := range values {
			category, ok := normalizeFixCategory(splitValue)
			if !ok {
				return nil, fmt.Errorf("unsupported fix category %q; supported categories: %s", splitValue, strings.Join(FixCategoryNames(), ", "))
			}
			if category == FixCategoryAll {
				for _, known := range allFixCategories {
					enabled[known] = true
				}
				continue
			}
			enabled[category] = true
		}
	}
	return enabled, nil
}

func normalizeFixCategory(raw string) (FixCategory, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "all":
		return FixCategoryAll, true
	case "metadata", "meta", "root":
		return FixCategoryMetadata, true
	case "safety", "security", "unsafe", "scripts", "events":
		return FixCategorySafety, true
	case "bleed", "trim", "safe-area", "safearea":
		return FixCategoryBleed, true
	case "effects", "effect", "filters", "filter", "transparency", "shadow", "shadows":
		return FixCategoryEffects, true
	case "raster", "rasters", "images", "image":
		return FixCategoryRaster, true
	case "references", "refs", "external":
		return FixCategoryReferences, true
	case "colors", "color", "cmyk", "rgb":
		return FixCategoryColors, true
	case "strokes", "stroke", "thin-strokes", "thin":
		return FixCategoryStrokes, true
	case "geometry", "joins", "lines", "near-disconnected":
		return FixCategoryGeometry, true
	case "typography", "type", "text", "fonts", "font":
		return FixCategoryTypography, true
	case "detail", "details", "small-detail", "durability":
		return FixCategoryDetail, true
	case "sizing", "size", "resolution", "ppi":
		return FixCategorySizing, true
	case "cutter", "cutters", "vinyl", "laser", "cnc", "plotter":
		return FixCategoryCutter, true
	default:
		return "", false
	}
}

func categoryEnabled(categories map[FixCategory]bool, category FixCategory) bool {
	return categories[category]
}

func fixMetadata(input []byte) ([]byte, []string) {
	out := input
	changes := []string{}

	meta, err := inspect(out)
	if err != nil {
		return input, nil
	}

	if !meta.HasXMLNS {
		updated, changed := addRootAttribute(out, `xmlns="http://www.w3.org/2000/svg"`)
		if changed {
			out = updated
			changes = append(changes, "added SVG namespace")
		}
	}

	meta, _ = inspect(out)
	if meta.ViewBox == "" && meta.WidthPixels > 0 && meta.HeightPixels > 0 {
		viewBox := fmt.Sprintf(`viewBox="0 0 %s %s"`, trimFloat(meta.WidthPixels), trimFloat(meta.HeightPixels))
		updated, changed := addRootAttribute(out, viewBox)
		if changed {
			out = updated
			changes = append(changes, "added viewBox derived from width and height")
		}
	}

	meta, _ = inspect(out)
	if (meta.Width == "" || meta.Height == "") && meta.ViewBox != "" {
		minX, minY, width, height := parseViewBoxOrDefault(meta.ViewBox)
		_ = minX
		_ = minY
		if width > 0 && height > 0 {
			if meta.Width == "" {
				updated, changed := addRootAttribute(out, fmt.Sprintf(`width="%s"`, trimFloat(width)))
				if changed {
					out = updated
					changes = append(changes, "added width derived from viewBox")
				}
			}
			meta, _ = inspect(out)
			if meta.Height == "" {
				updated, changed := addRootAttribute(out, fmt.Sprintf(`height="%s"`, trimFloat(height)))
				if changed {
					out = updated
					changes = append(changes, "added height derived from viewBox")
				}
			}
		}
	}

	return out, changes
}

func fixSafety(input []byte) ([]byte, []string) {
	out := input
	changes := []string{}

	updated, removedScripts := removeElementByName(out, "script")
	if removedScripts > 0 {
		out = updated
		changes = append(changes, fmt.Sprintf("removed %d script element(s)", removedScripts))
	}
	updated, removedAttrs := removeEventHandlerAttrs(out)
	if removedAttrs > 0 {
		out = updated
		changes = append(changes, fmt.Sprintf("removed %d event handler attribute(s)", removedAttrs))
	}

	return out, changes
}

func fixEffects(input []byte) ([]byte, []string) {
	out := input
	changes := []string{}

	updated, removedFilters := removeElementByName(out, "filter")
	if removedFilters > 0 {
		out = updated
		changes = append(changes, fmt.Sprintf("removed %d filter element(s)", removedFilters))
	}
	updated, removedMasks := removeElementByName(out, "mask")
	if removedMasks > 0 {
		out = updated
		changes = append(changes, fmt.Sprintf("removed %d mask element(s)", removedMasks))
	}
	updated, removedClipPaths := removeElementByName(out, "clipPath")
	if removedClipPaths > 0 {
		out = updated
		changes = append(changes, fmt.Sprintf("removed %d clipPath element(s)", removedClipPaths))
	}
	updated, removedAttrs := removeAttrsByName(out, map[string]bool{
		"filter":         true,
		"mask":           true,
		"clip-path":      true,
		"opacity":        true,
		"fill-opacity":   true,
		"stroke-opacity": true,
		"style":          true,
	}, effectAttrShouldRemove)
	if removedAttrs > 0 {
		out = updated
		changes = append(changes, fmt.Sprintf("removed %d effect/transparency attribute(s)", removedAttrs))
	}

	return out, changes
}

func fixRaster(input []byte) ([]byte, []string) {
	updated, removed := removeElementByName(input, "image")
	if removed == 0 {
		return input, nil
	}
	return updated, []string{fmt.Sprintf("removed %d image element(s)", removed)}
}

func fixBleed(input []byte, target Target, meta SVGMeta) ([]byte, []string) {
	if meta.MissingBleedShapes == 0 {
		return input, nil
	}
	minX, minY, width, height := parseViewBoxOrDefault(overlayViewBox(meta))
	if width <= 0 || height <= 0 {
		return input, nil
	}
	bleedMargin, _ := productionMarginsInSVGUnits(meta, target, width, height)
	if bleedMargin <= 0 {
		return input, nil
	}
	canvas := box{x1: minX, y1: minY, x2: minX + width, y2: minY + height}
	updated, expanded := expandBleedRects(input, canvas, bleedMargin)
	if expanded == 0 {
		return input, nil
	}
	return updated, []string{fmt.Sprintf("expanded %d background rect(s) past trim for bleed", expanded)}
}

func expandBleedRects(input []byte, canvas box, bleedMargin float64) ([]byte, int) {
	return rewriteStartElements(input, func(start xml.StartElement) (xml.StartElement, bool) {
		if !strings.EqualFold(start.Name.Local, "rect") {
			return start, false
		}
		attr := attrsByName(start.Attr)
		b, ok := roughBBox("rect", attr)
		if !ok {
			return start, false
		}
		tolerance := mathMax(canvas.width(), canvas.height()) * 0.001
		if tolerance < 0.01 {
			tolerance = 0.01
		}
		shape := roughShape{kind: "rect", box: b}
		if !backgroundLikeShape(shape, canvas, tolerance) || boxExtendsBeyondCanvas(b, canvas, bleedMargin*0.5) {
			return start, false
		}
		setAttr(&start, "x", trimFloat(canvas.x1-bleedMargin))
		setAttr(&start, "y", trimFloat(canvas.y1-bleedMargin))
		setAttr(&start, "width", trimFloat(canvas.width()+bleedMargin*2))
		setAttr(&start, "height", trimFloat(canvas.height()+bleedMargin*2))
		return start, true
	})
}

func advisoryFixNotes(report Report, categories map[FixCategory]bool) []string {
	type advisory struct {
		category FixCategory
		codes    []string
		message  string
	}
	advisories := []advisory{
		{FixCategoryReferences, []string{"external-reference", "packaging-external-reference"}, "external references need manual packaging or embedding"},
		{FixCategoryColors, []string{"color-count", "rgb-colors-for-print", "cmyk-in-svg", "many-fabric-colors"}, "color fixes need manual CMYK/spot-color conversion and proofing"},
		{FixCategoryStrokes, []string{"thin-stroke"}, "thin strokes need manual review before thickening or outlining"},
		{FixCategoryGeometry, []string{"near-disconnected-lines"}, "near-disconnected line joins need manual node joining or shape rebuilding"},
		{FixCategoryTypography, []string{"text-not-outlined", "text-overlap-shapes"}, "text fixes need manual outlining, knockouts, or layout changes"},
		{FixCategoryDetail, []string{"small-detail-durability"}, "small-detail durability fixes need manual simplification or production-method changes"},
		{FixCategorySizing, []string{"low-effective-ppi", "modest-effective-ppi", "oversized-for-target", "target-size-recommended"}, "sizing/resolution fixes need a chosen physical output size or higher-resolution source art"},
		{FixCategoryCutter, []string{"raster-not-cuttable", "effects-may-not-output"}, "cutter fixes may need manual path reconstruction; use --unsafe --fix raster,effects only to strip incompatible content"},
		{FixCategoryBleed, []string{"safe-area-risk"}, "safe-area fixes need manual layout movement inward from trim"},
		{FixCategoryEffects, []string{"background-transparency"}, "background transparency fixes need the intended substrate/background color"},
	}

	var notes []string
	for _, item := range advisories {
		if !categories[item.category] {
			continue
		}
		if reportHasAnyIssue(report, item.codes...) {
			notes = append(notes, item.message)
		}
	}
	return notes
}

func reportHasAnyIssue(report Report, codes ...string) bool {
	for _, issue := range report.Issues {
		for _, code := range codes {
			if issue.Code == code {
				return true
			}
		}
	}
	return false
}

func removeAttrsByName(input []byte, names map[string]bool, shouldRemove func(xml.Attr) bool) ([]byte, int) {
	removed := 0
	updated, rewritten := rewriteStartElements(input, func(start xml.StartElement) (xml.StartElement, bool) {
		attrs := start.Attr[:0]
		changed := false
		for _, attr := range start.Attr {
			name := strings.ToLower(attr.Name.Local)
			if names[name] && shouldRemove(attr) {
				removed++
				changed = true
				continue
			}
			attrs = append(attrs, attr)
		}
		start.Attr = attrs
		return start, changed
	})
	if rewritten == 0 {
		return input, 0
	}
	return updated, removed
}

func effectAttrShouldRemove(attr xml.Attr) bool {
	if !strings.EqualFold(attr.Name.Local, "style") {
		return true
	}
	return styleContainsAny(attr.Value, "filter", "opacity", "mix-blend-mode", "background-blend-mode")
}

func styleContainsAny(style string, names ...string) bool {
	for _, declaration := range strings.Split(style, ";") {
		parts := strings.SplitN(declaration, ":", 2)
		if len(parts) != 2 {
			continue
		}
		property := strings.ToLower(strings.TrimSpace(parts[0]))
		for _, name := range names {
			if property == name {
				return true
			}
		}
	}
	return false
}

func rewriteStartElements(input []byte, rewrite func(xml.StartElement) (xml.StartElement, bool)) ([]byte, int) {
	decoder := xml.NewDecoder(bytes.NewReader(input))
	var out bytes.Buffer
	encoder := xml.NewEncoder(&out)
	changes := 0

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return input, 0
		}

		if start, ok := token.(xml.StartElement); ok {
			updated, changed := rewrite(start)
			if changed {
				changes++
				token = updated
			}
		}
		token = localizeXMLToken(token)

		if err := encoder.EncodeToken(token); err != nil {
			return input, 0
		}
	}

	if changes == 0 {
		return input, 0
	}
	if err := encoder.Flush(); err != nil {
		return input, 0
	}
	return out.Bytes(), changes
}

func setAttr(start *xml.StartElement, name, value string) {
	for i := range start.Attr {
		if start.Attr[i].Name.Space == "" && strings.EqualFold(start.Attr[i].Name.Local, name) {
			start.Attr[i].Value = value
			return
		}
	}
	start.Attr = append(start.Attr, xml.Attr{Name: xml.Name{Local: name}, Value: value})
}

func dedupeStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func removeElementByName(input []byte, localName string) ([]byte, int) {
	names := map[string]bool{strings.ToLower(localName): true}
	return removeElementsByName(input, names)
}

func removeElementsByName(input []byte, localNames map[string]bool) ([]byte, int) {
	decoder := xml.NewDecoder(bytes.NewReader(input))
	var out bytes.Buffer
	encoder := xml.NewEncoder(&out)
	removed := 0
	skipDepth := 0

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return input, 0
		}

		if skipDepth > 0 {
			switch token.(type) {
			case xml.StartElement:
				skipDepth++
			case xml.EndElement:
				skipDepth--
			}
			continue
		}

		if start, ok := token.(xml.StartElement); ok && localNames[strings.ToLower(start.Name.Local)] {
			removed++
			skipDepth = 1
			continue
		}
		token = localizeXMLToken(token)

		if err := encoder.EncodeToken(token); err != nil {
			return input, 0
		}
	}

	if removed == 0 {
		return input, 0
	}
	if err := encoder.Flush(); err != nil {
		return input, 0
	}
	return out.Bytes(), removed
}

func removeEventHandlerAttrs(input []byte) ([]byte, int) {
	decoder := xml.NewDecoder(bytes.NewReader(input))
	var out bytes.Buffer
	encoder := xml.NewEncoder(&out)
	removed := 0

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return input, 0
		}

		if start, ok := token.(xml.StartElement); ok {
			attrs := start.Attr[:0]
			for _, attr := range start.Attr {
				if isEventHandlerAttr(attr) {
					removed++
					continue
				}
				attrs = append(attrs, attr)
			}
			start.Attr = attrs
			token = start
		}
		token = localizeXMLToken(token)

		if err := encoder.EncodeToken(token); err != nil {
			return input, 0
		}
	}

	if removed == 0 {
		return input, 0
	}
	if err := encoder.Flush(); err != nil {
		return input, 0
	}
	return out.Bytes(), removed
}

func isEventHandlerAttr(attr xml.Attr) bool {
	return attr.Name.Space == "" && strings.HasPrefix(strings.ToLower(attr.Name.Local), "on")
}

func localizeXMLToken(token xml.Token) xml.Token {
	switch tok := token.(type) {
	case xml.StartElement:
		tok.Name.Space = ""
		for i := range tok.Attr {
			if tok.Attr[i].Name.Space == "http://www.w3.org/2000/xmlns/" {
				tok.Attr[i].Name.Space = "xmlns"
			}
		}
		return tok
	case xml.EndElement:
		tok.Name.Space = ""
		return tok
	default:
		return token
	}
}

func trimFloat(v float64) string {
	if v == float64(int64(v)) {
		return fmt.Sprintf("%.0f", v)
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.3f", v), "0"), ".")
}

func addRootAttribute(input []byte, attr string) ([]byte, bool) {
	loc := bytes.Index(bytes.ToLower(input), []byte("<svg"))
	if loc == -1 {
		return input, false
	}
	insertAt := loc + len("<svg")
	if insertAt >= len(input) {
		return input, false
	}

	prefix := " " + attr
	if input[insertAt] != '>' {
		prefix += " "
	}

	out := append([]byte{}, input[:insertAt]...)
	out = append(out, []byte(prefix)...)
	out = append(out, input[insertAt:]...)
	return out, true
}
