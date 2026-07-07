package svgcheck

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"regexp"
	"strconv"
	"strings"
)

type OverlayOptions struct {
	Target string
}

type overlayData struct {
	Meta       SVGMeta
	Report     Report
	Endpoints  []geometryEndpoint
	ThinShapes []thinShape
}

type thinShape struct {
	name  string
	attrs map[string]string
}

func GenerateOverlay(input []byte, opts OverlayOptions) ([]byte, error) {
	report, err := Check(input, opts.Target)
	if err != nil {
		return nil, err
	}
	data, err := inspectOverlayData(input, opts.Target)
	if err != nil {
		return nil, err
	}
	data.Report = report

	var out strings.Builder
	viewBox := overlayViewBox(data.Meta)
	minX, minY, width, height := parseViewBoxOrDefault(viewBox)
	scale := overlayScale(width, height)
	panelWidth := width * 0.42
	if panelWidth < 180*scale {
		panelWidth = 180 * scale
	}
	if panelWidth > width*0.72 {
		panelWidth = width * 0.72
	}
	panelX := minX + width - panelWidth - 18*scale
	panelY := minY + 18*scale

	out.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	fmt.Fprintf(&out, `<svg xmlns="http://www.w3.org/2000/svg" width="%s" height="%s" viewBox="%s">`+"\n",
		escapeAttr(defaultString(data.Meta.Width, trimFloat(width))),
		escapeAttr(defaultString(data.Meta.Height, trimFloat(height))),
		escapeAttr(viewBox),
	)
	writeOverlayDefs(&out)
	fmt.Fprintf(&out, `<rect x="%s" y="%s" width="%s" height="%s" fill="#f8fafc"/>`+"\n",
		trimFloat(minX), trimFloat(minY), trimFloat(width), trimFloat(height))
	fmt.Fprintf(&out, `<g opacity="0.24"><svg x="%s" y="%s" width="%s" height="%s" viewBox="%s" preserveAspectRatio="xMidYMid meet">%s</svg></g>`+"\n",
		trimFloat(minX), trimFloat(minY), trimFloat(width), trimFloat(height), escapeAttr(viewBox), stripXMLDeclaration(input))

	out.WriteString(`<g id="pre-print-overlay" font-family="Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif">` + "\n")
	writeThinShapeHighlights(&out, data.ThinShapes, scale)
	writeNearDisconnectedHighlights(&out, data.Endpoints, scale)
	writeOverlayPanel(&out, data.Report, panelX, panelY, panelWidth, scale)
	out.WriteString("</g>\n</svg>\n")
	return []byte(out.String()), nil
}

func inspectOverlayData(input []byte, rawTarget string) (overlayData, error) {
	meta, err := inspect(input)
	if err != nil {
		return overlayData{}, err
	}
	target, err := ParseTarget(rawTarget)
	if err != nil {
		return overlayData{}, err
	}
	profile := issueProfileForTarget(target)
	decoder := xml.NewDecoder(bytes.NewReader(input))
	data := overlayData{Meta: meta}
	geometrySource := 0
	styleStack := []geometryStyle{defaultGeometryStyle()}

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return data, fmt.Errorf("invalid SVG XML: %w", err)
		}
		switch tok := token.(type) {
		case xml.StartElement:
			currentStyle := inheritedGeometryStyle(styleStack[len(styleStack)-1], tok.Attr)
			styleStack = append(styleStack, currentStyle)
			name := strings.ToLower(tok.Name.Local)
			if isOverlayGeometryElement(name) {
				if profile.ReviewDisconnectedJoins {
					data.Endpoints = append(data.Endpoints, endpointsFromElement(name, tok.Attr, currentStyle, &geometrySource)...)
				}
				if profile.ReviewThinStrokes && elementHasThinStroke(tok.Attr) {
					data.ThinShapes = append(data.ThinShapes, thinShape{name: name, attrs: attrsByName(tok.Attr)})
				}
			}
		case xml.EndElement:
			if len(styleStack) > 1 {
				styleStack = styleStack[:len(styleStack)-1]
			}
		default:
			continue
		}
	}

	return data, nil
}

func writeOverlayDefs(out *strings.Builder) {
	out.WriteString(`<defs>
  <filter id="ppt-overlay-shadow" x="-20%" y="-20%" width="140%" height="140%">
    <feDropShadow dx="0" dy="2" stdDeviation="3" flood-color="#0f172a" flood-opacity="0.22"/>
  </filter>
  <marker id="ppt-overlay-arrow" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
    <path d="M 0 0 L 10 5 L 0 10 z" fill="#e11d48"/>
  </marker>
</defs>
`)
}

func writeThinShapeHighlights(out *strings.Builder, shapes []thinShape, scale float64) {
	if len(shapes) == 0 {
		return
	}
	out.WriteString(`<g id="pre-print-thin-stroke-highlights">` + "\n")
	for _, shape := range shapes {
		writeHighlightedGeometry(out, shape, "#f59e0b", 3.2*scale, 0.78)
		writeHighlightedGeometry(out, shape, "#fff7ed", 1.35*scale, 0.92)
	}
	out.WriteString("</g>\n")
}

func writeNearDisconnectedHighlights(out *strings.Builder, endpoints []geometryEndpoint, scale float64) {
	pairs := nearDisconnectedEndpointPairs(endpoints)
	if len(pairs) < nearDisconnectedMinimumPairs {
		return
	}
	out.WriteString(`<g id="pre-print-near-disconnected-highlights">` + "\n")
	for i, pair := range pairs {
		labelX := (pair.a.x+pair.b.x)/2 + 7*scale
		labelY := (pair.a.y+pair.b.y)/2 - 7*scale
		fmt.Fprintf(out, `<line x1="%s" y1="%s" x2="%s" y2="%s" stroke="#e11d48" stroke-width="%s" stroke-dasharray="%s %s" marker-start="url(#ppt-overlay-arrow)" marker-end="url(#ppt-overlay-arrow)" opacity="0.92"/>`+"\n",
			trimFloat(pair.a.x), trimFloat(pair.a.y), trimFloat(pair.b.x), trimFloat(pair.b.y), trimFloat(1.2*scale), trimFloat(3*scale), trimFloat(2*scale))
		for _, point := range []geometryEndpoint{pair.a, pair.b} {
			fmt.Fprintf(out, `<circle cx="%s" cy="%s" r="%s" fill="#fff1f2" stroke="#e11d48" stroke-width="%s" filter="url(#ppt-overlay-shadow)"/>`+"\n",
				trimFloat(point.x), trimFloat(point.y), trimFloat(4.5*scale), trimFloat(1.4*scale))
			fmt.Fprintf(out, `<circle cx="%s" cy="%s" r="%s" fill="#e11d48"/>`+"\n",
				trimFloat(point.x), trimFloat(point.y), trimFloat(1.35*scale))
		}
		fmt.Fprintf(out, `<g transform="translate(%s %s)"><rect x="0" y="%s" width="%s" height="%s" rx="%s" fill="#be123c" filter="url(#ppt-overlay-shadow)"/><text x="%s" y="%s" font-size="%s" font-weight="700" fill="#fff">gap %d</text></g>`+"\n",
			trimFloat(labelX), trimFloat(labelY), trimFloat(-11*scale), trimFloat(42*scale), trimFloat(16*scale), trimFloat(8*scale), trimFloat(7*scale), trimFloat(0.3*scale), trimFloat(8.5*scale), i+1)
	}
	out.WriteString("</g>\n")
}

func writeOverlayPanel(out *strings.Builder, report Report, x, y, width, scale float64) {
	errors, warnings, info := report.IssueCounts()
	lineHeight := 15 * scale
	height := (78 + float64(min(len(report.Issues), 6))*15) * scale
	fmt.Fprintf(out, `<g id="pre-print-overlay-panel" transform="translate(%s %s)" filter="url(#ppt-overlay-shadow)">`+"\n", trimFloat(x), trimFloat(y))
	fmt.Fprintf(out, `<rect x="0" y="0" width="%s" height="%s" rx="%s" fill="#0f172a" opacity="0.92"/>`+"\n", trimFloat(width), trimFloat(height), trimFloat(10*scale))
	fmt.Fprintf(out, `<text x="%s" y="%s" font-size="%s" font-weight="800" fill="#ffffff">Pre-print overlay</text>`+"\n", trimFloat(14*scale), trimFloat(23*scale), trimFloat(14*scale))
	fmt.Fprintf(out, `<text x="%s" y="%s" font-size="%s" fill="#cbd5e1">%d error(s) · %d warning(s) · %d note(s)</text>`+"\n", trimFloat(14*scale), trimFloat(42*scale), trimFloat(10*scale), errors, warnings, info)
	fmt.Fprintf(out, `<text x="%s" y="%s" font-size="%s" fill="#93c5fd">%s</text>`+"\n", trimFloat(14*scale), trimFloat(59*scale), trimFloat(9*scale), escapeText(overlayMetaSummary(report)))

	yCursor := 80 * scale
	for i, issue := range report.Issues {
		if i >= 6 {
			remaining := len(report.Issues) - i
			fmt.Fprintf(out, `<text x="%s" y="%s" font-size="%s" fill="#94a3b8">+ %d more in the terminal report</text>`+"\n", trimFloat(14*scale), trimFloat(yCursor), trimFloat(9*scale), remaining)
			break
		}
		color := severityColor(issue.Severity)
		rank := ""
		if issue.Rank != "" {
			rank = " / " + string(issue.Rank)
		}
		fmt.Fprintf(out, `<circle cx="%s" cy="%s" r="%s" fill="%s"/>`+"\n", trimFloat(18*scale), trimFloat(yCursor-3*scale), trimFloat(3*scale), color)
		fmt.Fprintf(out, `<text x="%s" y="%s" font-size="%s" fill="#e2e8f0">%s%s: %s</text>`+"\n",
			trimFloat(27*scale), trimFloat(yCursor), trimFloat(8.6*scale), escapeText(string(issue.Severity)), escapeText(rank), escapeText(issue.Code))
		yCursor += lineHeight
	}
	out.WriteString("</g>\n")
}

func overlayMetaSummary(report Report) string {
	parts := []string{}
	if report.Meta.Width != "" && report.Meta.Height != "" {
		parts = append(parts, fmt.Sprintf("%s x %s", report.Meta.Width, report.Meta.Height))
	}
	if report.Meta.ViewBox != "" {
		parts = append(parts, "viewBox set")
	}
	if report.Target.Raw != "" {
		parts = append(parts, "target "+report.Target.Raw)
	}
	if len(parts) == 0 {
		return "Metadata available"
	}
	return strings.Join(parts, " / ")
}

func writeHighlightedGeometry(out *strings.Builder, shape thinShape, stroke string, strokeWidth, opacity float64) {
	switch shape.name {
	case "line":
		fmt.Fprintf(out, `<line x1="%s" y1="%s" x2="%s" y2="%s"%s fill="none" stroke="%s" stroke-width="%s" stroke-linecap="round" stroke-linejoin="round" opacity="%s" pointer-events="none"/>`+"\n",
			escapeAttr(shape.attrs["x1"]), escapeAttr(shape.attrs["y1"]), escapeAttr(shape.attrs["x2"]), escapeAttr(shape.attrs["y2"]), transformAttr(shape.attrs), stroke, trimFloat(strokeWidth), trimFloat(opacity))
	case "polyline", "polygon":
		fmt.Fprintf(out, `<%s points="%s"%s fill="none" stroke="%s" stroke-width="%s" stroke-linecap="round" stroke-linejoin="round" opacity="%s" pointer-events="none"/>`+"\n",
			shape.name, escapeAttr(shape.attrs["points"]), transformAttr(shape.attrs), stroke, trimFloat(strokeWidth), trimFloat(opacity))
	case "path":
		fmt.Fprintf(out, `<path d="%s"%s fill="none" stroke="%s" stroke-width="%s" stroke-linecap="round" stroke-linejoin="round" opacity="%s" pointer-events="none"/>`+"\n",
			escapeAttr(shape.attrs["d"]), transformAttr(shape.attrs), stroke, trimFloat(strokeWidth), trimFloat(opacity))
	}
}

func isOverlayGeometryElement(name string) bool {
	switch name {
	case "line", "polyline", "polygon", "path":
		return true
	default:
		return false
	}
}

func elementHasThinStroke(attrs []xml.Attr) bool {
	attr := attrsByName(attrs)
	if strokeWidthLooksThin(attr["stroke-width"]) {
		return true
	}
	return strokeWidthLooksThin(styleValue(attr["style"], "stroke-width"))
}

func styleValue(style, key string) string {
	for _, declaration := range strings.Split(style, ";") {
		parts := strings.SplitN(declaration, ":", 2)
		if len(parts) != 2 {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(parts[0]), key) {
			return strings.TrimSpace(parts[1])
		}
	}
	return ""
}

func overlayViewBox(meta SVGMeta) string {
	if meta.ViewBox != "" {
		return meta.ViewBox
	}
	width := meta.WidthPixels
	height := meta.HeightPixels
	if width <= 0 {
		width = 100
	}
	if height <= 0 {
		height = 100
	}
	return fmt.Sprintf("0 0 %s %s", trimFloat(width), trimFloat(height))
}

func parseViewBoxOrDefault(viewBox string) (float64, float64, float64, float64) {
	parts := strings.Fields(viewBox)
	if len(parts) == 4 {
		values := make([]float64, 4)
		for i, part := range parts {
			v, err := strconv.ParseFloat(part, 64)
			if err != nil {
				return 0, 0, 100, 100
			}
			values[i] = v
		}
		if values[2] > 0 && values[3] > 0 {
			return values[0], values[1], values[2], values[3]
		}
	}
	return 0, 0, 100, 100
}

func overlayScale(width, height float64) float64 {
	scale := mathMax(width, height) / 600
	if scale < 0.6 {
		return 0.6
	}
	if scale > 4 {
		return 4
	}
	return scale
}

func transformAttr(attrs map[string]string) string {
	if attrs["transform"] == "" {
		return ""
	}
	return fmt.Sprintf(` transform="%s"`, escapeAttr(attrs["transform"]))
}

func stripXMLDeclaration(input []byte) string {
	text := strings.TrimSpace(string(input))
	xmlDeclarationPattern := regexp.MustCompile(`(?is)^<\?xml[^>]*>\s*`)
	return xmlDeclarationPattern.ReplaceAllString(text, "")
}

func severityColor(severity Severity) string {
	switch severity {
	case SeverityError:
		return "#ef4444"
	case SeverityWarning:
		return "#f59e0b"
	default:
		return "#38bdf8"
	}
}

func defaultString(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func escapeAttr(value string) string {
	return html.EscapeString(value)
}

func escapeText(value string) string {
	return html.EscapeString(value)
}

func mathMax(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
