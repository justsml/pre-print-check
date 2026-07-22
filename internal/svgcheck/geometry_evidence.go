package svgcheck

import (
	"encoding/xml"
	"math"
	"strings"
)

type geometryEvidence struct {
	NearDisconnectedPairs []nearEndpointPair
	ThinShapes            []locatableShape
}

type geometryEvidenceCollector struct {
	target         Target
	styleStack     []geometryStyle
	endpoints      []geometryEndpoint
	geometrySource int
	shapes         []roughShape
	polygons       []roughShape
	texts          []roughText
	thinCounts     map[string]int
	thinShapes     []locatableShape
	currentText    *textCapture
	mmPerUnit      float64
}

func newGeometryEvidenceCollector(target Target) *geometryEvidenceCollector {
	return &geometryEvidenceCollector{
		target:     target,
		styleStack: []geometryStyle{defaultGeometryStyle()},
		thinCounts: map[string]int{},
	}
}

func (c *geometryEvidenceCollector) startElement(element xml.StartElement, attr map[string]string, meta *SVGMeta) {
	name := strings.ToLower(element.Name.Local)
	style := inheritedGeometryStyle(c.styleStack[len(c.styleStack)-1], attr)
	c.styleStack = append(c.styleStack, style)
	if name == "svg" && len(c.styleStack) == 2 {
		c.mmPerUnit = physicalMMPerSVGUnit(*meta, c.target)
	}

	c.endpoints = append(c.endpoints, endpointsFromElement(name, attr, style, &c.geometrySource)...)
	if isOverlayGeometryElement(name) && style.hasVisibleStroke() && strokeWidthLooksProductionThin(style.strokeWidth) {
		c.thinCounts[strokeWidthLabel(attr, style)]++
		c.thinShapes = append(c.thinShapes, locatableShape{name: name, attrs: attr})
	}

	if bounds, ok := roughBBox(name, attr); ok {
		c.shapes = append(c.shapes, roughShape{kind: name, box: bounds})
		if name == "polygon" {
			c.polygons = append(c.polygons, roughShape{kind: name, box: bounds})
		}
		if c.mmPerUnit > 0 {
			maxMM := math.Max(bounds.width(), bounds.height()) * c.mmPerUnit
			if maxMM > 0 && maxMM < 1 {
				meta.SmallShapesSub1MM++
			}
			if maxMM > 0 && maxMM < 2 {
				meta.SmallShapesSub2MM++
			}
		}
	}

	if name == "text" {
		c.currentText = newTextCapture(attr, style)
	}
	if name == "fedropshadow" && largeShadowElement(attr) {
		meta.LargeShadows++
	}
	if backgroundTransparencyElement(name, attr, *meta) {
		meta.BackgroundTransparency++
	}
}

func (c *geometryEvidenceCollector) characterData(data []byte) {
	if c.currentText != nil {
		c.currentText.text.Write(data)
	}
}

func (c *geometryEvidenceCollector) endElement(element xml.EndElement) {
	if strings.EqualFold(element.Name.Local, "text") && c.currentText != nil {
		if text, ok := c.currentText.toRoughText(); ok {
			c.texts = append(c.texts, text)
		}
		c.currentText = nil
	}
	if len(c.styleStack) > 1 {
		c.styleStack = c.styleStack[:len(c.styleStack)-1]
	}
}

func (c *geometryEvidenceCollector) finish(meta *SVGMeta) geometryEvidence {
	pairs := nearDisconnectedEndpointPairs(c.endpoints)
	if len(pairs) >= nearDisconnectedMinimumPairs {
		meta.NearDisconnected = len(pairs)
	}
	meta.ThinStrokeSummaries = strokeSummaries(c.thinCounts)
	if len(meta.ThinStrokeSummaries) > 0 {
		thinStrokes := 0
		for _, summary := range meta.ThinStrokeSummaries {
			thinStrokes += summary.Count
		}
		meta.ThinStrokes = thinStrokes
	}
	meta.TextShapeOverlaps = textPolygonOverlaps(c.texts, c.polygons)
	meta.SubtleEffects = subtleEffectCount(*meta)
	meta.MissingBleedShapes, meta.SafeAreaRiskShapes = bleedAndSafeAreaRisks(*meta, c.target, c.shapes, c.texts)

	return geometryEvidence{
		NearDisconnectedPairs: pairs,
		ThinShapes:            c.thinShapes,
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
