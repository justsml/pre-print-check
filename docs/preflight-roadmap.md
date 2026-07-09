# Preflight Roadmap

This roadmap turns the current SVG checker into a production-profile preflight tool without pretending an SVG alone can prove every press, cutter, or RIP outcome.

`pre-print` should stay conservative by default:

- Keep checks explainable from the SVG source or explicitly provided target options.
- Keep fixes opt-in and narrow, especially when artwork appearance can change.
- Prefer target-specific guidance over universal pass/fail claims.
- Keep issue codes stable because reports may be used in scripts.

## Agent Handoff Matrix

| Gap | Scenario | Implementation agent | Model | Thinking | Notes |
| --- | --- | --- | --- | --- | --- |
| 1 | Bleed, trim, and safe-area checks | Press Geometry Agent | `gpt-5.4` | high | Shares target-size parsing with raster PPI work. |
| 2 | Real embedded raster PPI inspection | Press Geometry Agent | `gpt-5.4` | high | Needs image decoding and element-bound analysis. |
| 3 | Font and live text preflight | Text Color Agent | `gpt-5.5` | high | Needs CSS/font parsing judgment and print workflow nuance. |
| 4 | Color-management profile support | Text Color Agent | `gpt-5.5` | high | Needs structured color inventory and output-intent language. |
| 5 | PDF/X readiness or export bridge | PDF Effects Agent | `gpt-5.5` | high | Needs external tool boundary decisions. |
| 6 | Transparency, filters, masks, and flattening risk map | PDF Effects Agent | `gpt-5.5` | high | Builds on existing effect signal detection. |
| 7 | Overprint, knockout, white ink, underbase, and spot-layer scenarios | Text Color Agent | `gpt-5.5` | high | Especially useful for labels, stickers, packaging, and textiles. |
| 8 | Cutter, laser, and CNC toolpath health | Toolpath Agent | `gpt-5.4` | high | Needs deeper path geometry than current endpoint checks. |
| 9 | Hidden, nonprinting, off-canvas, or bloated content | Press Geometry Agent | `gpt-5.4` | high | Useful across print, cutter, and upload validation. |
| 10 | Static SVG interoperability profile | PDF Effects Agent | `gpt-5.5` | high | Defines a predictable print/static rendering subset. |

## Product Gaps

### 1. Bleed, Trim, And Safe-Area Checks

Problem: the checker sees root size metadata but cannot evaluate whether artwork has bleed, trim, or safe-area problems for a concrete product.

Initial shape:

- Add target options for physical width, height, bleed, and safe margin.
- Report content outside trim, content too close to trim, and backgrounds that stop at trim when bleed is required.
- Keep fixes limited to simple background expansion when the geometry is unambiguous.

Likely issue codes: `missing-bleed`, `unsafe-margin`, `content-outside-trim`, `target-size-mismatch`.

### 2. Real Embedded Raster PPI Inspection

Problem: current checks count raster images but do not decode local or inline images, inspect intrinsic dimensions, or compute effective PPI at production size.

Initial shape:

- Decode data URI rasters and local linked images without fetching network resources.
- Track image element bounds and transform scale where practical.
- Report low or modest effective PPI based on target material and viewing distance.

Likely issue codes: `low-raster-ppi`, `modest-raster-ppi`, `unknown-raster-size`, `external-raster-unmeasured`.

### 3. Font And Live Text Preflight For Print

Problem: live SVG text can substitute, reflow, disappear, or violate font handoff rules outside the authoring environment.

Initial shape:

- Detect `<text>`, `<tspan>`, `font-family`, `@font-face`, external font URLs, and embedded font data.
- Warn on live text for press-like targets and error or warn more strongly for cutter-like targets.
- Distinguish "text exists" from "font dependency exists" so reports are actionable.

Likely issue codes: `live-text-for-print`, `font-family-dependency`, `external-font-reference`, `embedded-font-data`.

### 4. Color-Management Profile Support

Problem: current RGB/CMYK detection is heuristic and does not give a useful color inventory for proofing or separations.

Initial shape:

- Parse color-bearing attributes and inline CSS into a structured inventory.
- Categorize RGB/web colors, CMYK-like syntax, ICC-linked colors, named colors, gradients, and alpha-bearing colors.
- Add target-specific language for press, packaging, textile, and spot-color workflows.

Likely issue codes: `rgb-colors-for-print`, `cmyk-in-svg`, `icc-color-in-svg`, `transparent-color`, `many-spot-like-colors`.

### 5. PDF/X Readiness Or Export Bridge

Problem: press workflows usually need a final PDF/X or printer-specific PDF proof, while this tool currently stops at SVG source inspection.

Initial shape:

- Add a readiness report that explains whether the SVG has known blockers before export.
- Keep actual export or PDF validation behind explicit commands and external-tool detection.
- Treat missing tools as advisory, not a required dependency for ordinary SVG checks.

Likely issue codes: `pdfx-readiness-blocker`, `pdfx-export-tool-missing`, `pdfx-validation-unavailable`.

### 6. Transparency, Filters, Masks, And Flattening Risk Map

Problem: soft effects are currently detected broadly, but the report does not identify the exact effect types or flattening risks.

Initial shape:

- Inventory filters, filter references, masks, clip paths, opacity, blend modes, gradients with alpha, and CSS shadows.
- Rank by target material and whether the effect is structural, visual, or likely to rasterize.
- Keep unsafe removal gated behind `--unsafe`.

Likely issue codes: `filter-flattening-risk`, `mask-flattening-risk`, `opacity-flattening-risk`, `blend-mode-flattening-risk`.

### 7. Overprint, Knockout, White Ink, Underbase, And Spot-Layer Scenarios

Problem: SVG does not model print overprint directly, but production files often rely on layer names, white ink, underbase, or spot-color conventions.

Initial shape:

- Detect white fills/strokes, spot-like names, layer/group labels, and opacity/blend constructs that can imply knockout or overprint expectations.
- Add packaging, label, sticker, and textile guidance without claiming the SVG can prove final separations.
- Encourage final PDF/RIP proof where the SVG cannot encode the intent.

Likely issue codes: `white-ink-review`, `spot-layer-review`, `underbase-review`, `overprint-intent-unverified`.

### 8. Cutter, Laser, And CNC Toolpath Health

Problem: cutter-like targets need clean geometry, and current checks only cover raster/effect blockers, thin strokes, and some near-disconnected endpoints.

Initial shape:

- Parse path commands enough to detect open paths, duplicate paths, self-intersections, tiny islands, overlapping cut lines, and stroke-only geometry.
- Add target-specific thresholds for vinyl, plotter, laser, and CNC.
- Keep operation-color conventions configurable because shops vary.

Likely issue codes: `open-cut-path`, `duplicate-cut-path`, `self-intersecting-path`, `tiny-island`, `stroke-needs-outline`.

### 9. Hidden, Nonprinting, Off-Canvas, Or Bloated Content

Problem: hidden or off-canvas content can unexpectedly print, affect bounds, bloat files, or slow downstream tools.

Initial shape:

- Detect `display:none`, `visibility:hidden`, zero opacity, zero-size objects, empty paths, unused defs, and content far outside the viewBox.
- Report file bloat signals without rewriting authoring data by default.
- Keep cleanup fixes separate from preflight checks.

Likely issue codes: `hidden-content`, `invisible-content`, `off-canvas-content`, `unused-defs`, `empty-geometry`.

### 10. Static SVG Interoperability Profile

Problem: many SVG features are valid for browsers but fragile in print, PDF export, static preview, or upload validators.

Initial shape:

- Add a `static-svg` or `print-static` profile that warns on animation, interactive elements, remote resources, `foreignObject`, unsupported CSS, and renderer-sensitive references.
- Keep the profile explicit so screen/web users are not punished for valid web SVG.
- Use the profile as a foundation for upload validation and deterministic rendering.

Likely issue codes: `animation-not-static`, `foreignobject-not-static`, `remote-resource-not-static`, `unsupported-css-for-static-svg`.

## Suggested Commit Order

1. Add roadmap and agent handoffs.
2. Add target-size option parsing for width, height, bleed, and safe margin.
3. Add raster PPI decoding and report fields.
4. Add font/text and color inventory structures.
5. Add effect risk inventory and static SVG profile.
6. Add cutter path geometry checks.
7. Add PDF/X readiness/export bridge as an optional integration layer.

## Documentation Updates Per Feature

Every feature should update:

- `README.md` command examples and limits.
- This roadmap if scope changes while implementing.
- Any web demo copy under `docs/` when new report fields appear there.
- Tests beside the package they cover.

