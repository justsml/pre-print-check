package svgcheck

import (
	"fmt"
	"html"
	"strconv"
	"strings"
)

type OverlayOptions struct {
	Target string
}

type overlayData struct {
	Meta                  SVGMeta
	Report                Report
	NearDisconnectedPairs []nearEndpointPair
	ThinShapes            []locatableShape
}

func GenerateOverlay(input []byte, opts OverlayOptions) ([]byte, error) {
	report, analysis, err := checkWithDetails(input, opts.Target)
	if err != nil {
		return nil, err
	}
	data := overlayData{Meta: report.Meta, Report: report}
	if reportHasAnyIssue(report, "near-disconnected-lines") {
		data.NearDisconnectedPairs = analysis.Geometry.NearDisconnectedPairs
	}
	if reportHasAnyIssue(report, "thin-stroke") {
		data.ThinShapes = analysis.Geometry.ThinShapes
	}

	var out strings.Builder
	viewBox := overlayViewBox(data.Meta)
	minX, minY, width, height := parseViewBoxOrDefault(viewBox)
	scale := overlayScale(width, height)
	panelScale := scale * 1.25
	panelWidth := width * 0.50
	if panelWidth < 280*panelScale {
		panelWidth = 280 * panelScale
	}
	if panelWidth > width*0.72 {
		panelWidth = width * 0.72
	}
	panelX := minX + width - panelWidth - 18*panelScale
	panelY := minY + 18*panelScale

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

	out.WriteString(`<g id="pre-print-check-overlay" font-family="Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif">` + "\n")
	writeThinShapeHighlights(&out, data.ThinShapes, scale)
	writeNearDisconnectedHighlights(&out, data.NearDisconnectedPairs, scale)
	writeOverlayPanel(&out, data.Report, panelX, panelY, panelWidth, panelScale)
	out.WriteString("</g>\n</svg>\n")
	return []byte(out.String()), nil
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

func writeThinShapeHighlights(out *strings.Builder, shapes []locatableShape, scale float64) {
	if len(shapes) == 0 {
		return
	}
	out.WriteString(`<g id="pre-print-check-thin-stroke-highlights">` + "\n")
	for _, shape := range shapes {
		writeHighlightedGeometry(out, shape, "#f59e0b", 3.2*scale, 0.78)
		writeHighlightedGeometry(out, shape, "#fff7ed", 1.35*scale, 0.92)
	}
	out.WriteString("</g>\n")
}

func writeNearDisconnectedHighlights(out *strings.Builder, pairs []nearEndpointPair, scale float64) {
	if len(pairs) < nearDisconnectedMinimumPairs {
		return
	}
	out.WriteString(`<g id="pre-print-check-near-disconnected-highlights">` + "\n")
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
	lineHeight := 18 * scale
	height := (96 + float64(min(len(report.Issues), 6))*18) * scale
	fmt.Fprintf(out, `<g id="pre-print-check-overlay-panel" transform="translate(%s %s)" filter="url(#ppt-overlay-shadow)">`+"\n", trimFloat(x), trimFloat(y))
	fmt.Fprintf(out, `<rect x="0" y="0" width="%s" height="%s" rx="%s" fill="#0f172a" opacity="0.92"/>`+"\n", trimFloat(width), trimFloat(height), trimFloat(10*scale))
	fmt.Fprintf(out, `<text x="%s" y="%s" font-size="%s" font-weight="800" fill="#ffffff">Pre-print overlay</text>`+"\n", trimFloat(16*scale), trimFloat(27*scale), trimFloat(16*scale))
	fmt.Fprintf(out, `<text x="%s" y="%s" font-size="%s" fill="#cbd5e1">%d error(s) · %d warning(s) · %d note(s)</text>`+"\n", trimFloat(16*scale), trimFloat(50*scale), trimFloat(12*scale), errors, warnings, info)
	fmt.Fprintf(out, `<text x="%s" y="%s" font-size="%s" fill="#93c5fd">%s</text>`+"\n", trimFloat(16*scale), trimFloat(70*scale), trimFloat(10.5*scale), escapeText(overlayMetaSummary(report)))

	yCursor := 96 * scale
	for i, issue := range report.Issues {
		if i >= 6 {
			remaining := len(report.Issues) - i
			fmt.Fprintf(out, `<text x="%s" y="%s" font-size="%s" fill="#94a3b8">+ %d more in the terminal report</text>`+"\n", trimFloat(16*scale), trimFloat(yCursor), trimFloat(10.5*scale), remaining)
			break
		}
		color := severityColor(issue.Severity)
		rank := ""
		if issue.Rank != "" {
			rank = " / " + string(issue.Rank)
		}
		fmt.Fprintf(out, `<circle cx="%s" cy="%s" r="%s" fill="%s"/>`+"\n", trimFloat(21*scale), trimFloat(yCursor-4*scale), trimFloat(3.5*scale), color)
		fmt.Fprintf(out, `<text x="%s" y="%s" font-size="%s" fill="#e2e8f0">%s%s: %s</text>`+"\n",
			trimFloat(32*scale), trimFloat(yCursor), trimFloat(10.5*scale), escapeText(string(issue.Severity)), escapeText(rank), escapeText(issue.Code))
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

func writeHighlightedGeometry(out *strings.Builder, shape locatableShape, stroke string, strokeWidth, opacity float64) {
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

func styleValue(style, key string) string {
	value := ""
	visitStyleDeclarations(style, func(name, declarationValue string) {
		if value == "" && strings.EqualFold(name, key) {
			value = declarationValue
		}
	})
	return value
}

func visitStyleDeclarations(style string, visit func(name, value string)) {
	for start := 0; start < len(style); {
		end := strings.IndexByte(style[start:], ';')
		if end < 0 {
			end = len(style)
		} else {
			end += start
		}

		declaration := style[start:end]
		if colon := strings.IndexByte(declaration, ':'); colon >= 0 {
			name := strings.TrimSpace(declaration[:colon])
			value := strings.TrimSpace(declaration[colon+1:])
			if name != "" {
				visit(name, value)
			}
		}

		if end == len(style) {
			break
		}
		start = end + 1
	}
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
	if len(text) < len("<?xml") || !strings.EqualFold(text[:len("<?xml")], "<?xml") {
		return text
	}
	if end := strings.Index(text, "?>"); end >= 0 {
		return strings.TrimSpace(text[end+len("?>"):])
	}
	return text
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
