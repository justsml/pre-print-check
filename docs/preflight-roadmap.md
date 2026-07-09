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

## Shared Profile Mechanism

Gaps 5, 6, and 10 should share one explicit profile system rather than adding one-off flags for each scenario.

Proposed CLI:

```sh
pre-print check --target paper --profile pdfx-ready art.svg
pre-print check --target paper --profile flattening-risk art.svg
pre-print check --profile static-svg art.svg
pre-print check --target packaging --profile static-svg,pdfx-ready art.svg
```

Proposed Go API:

```go
type CheckOptions struct {
	Target   string
	Profiles []string
}

func CheckWithOptions(input []byte, opts CheckOptions) (Report, error)
```

Keep `Check(input, rawTarget)` as a compatibility wrapper around `CheckWithOptions`.

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

Owner: Text Color Agent, `gpt-5.5`, high thinking. Keep this work inside text and font inspection; do not take on PDF export, general transparency mapping, or cutter path geometry.

CLI/API surface:

- Extend existing `pre-print check --target ... FILE.svg`; no new flag is needed for v1.
- Keep `fix --fix typography` advisory-only. Do not outline text automatically.
- Extend `svgcheck.SVGMeta` with counts such as `TSpanElements`, `TextPathElements`, `FontFamilies`, `FontFaceRules`, `ExternalFontRefs`, and `EmbeddedFontData`.
- Mirror key counts into the WASM API metadata.
- Add a terminal/Markdown/HTML "Typography signals" summary when nonzero.

Implementation sketch:

- Extend `inspect` in `internal/svgcheck/check.go` to count `<tspan>`, `<textPath>`, and typography attributes.
- Parse direct attributes and inline `style` declarations for `font-family`, `font-size`, `font-weight`, `font-style`, and `font-stretch`.
- Inspect `<style>` character data for `@font-face`, `font-family`, and `url(...)`; count external font URLs without fetching them.
- Add target-profile booleans such as `ReviewTypography` and `RequireOutlinedText`.
- Preserve existing `text-not-outlined` for vinyl, laser, CNC, and plotter targets.

Issue codes:

- `live-text-for-print`: warning, rank high for paper, packaging, fabric, banner, signage, and vehicle wrap; rank moderate for generic physical-size targets.
- `font-family-dependency`: warning, rank moderate when live text uses non-generic families.
- `external-font-reference`: warning, rank high for print, packaging, fabric, and cutter handoff; rank moderate otherwise when external refs are reviewed.
- `embedded-font-data`: info or warning, rank low to moderate; message should mention opaque licensing and RIP behavior.
- `textpath-rendering-risk`: warning, rank moderate for print-like targets.
- `text-not-outlined`: keep existing code for cutter-like targets; consider ranking it high.

Tests and docs:

- Add table tests for paper, packaging, fabric, vinyl, and screen.
- Add fixture `print-edge-live-text-fonts.svg` with plain live text, `tspan`, `textPath`, inherited font-family, inline style font, external `@font-face`, and embedded data font.
- Assert screen output stays quiet for print-only typography warnings.
- Update README reported-issue bullets, limits, and web demo metadata if typography counts are surfaced.

Commit slices:

1. Add typography metadata collection and unit tests.
2. Add target-profile typography issue rules and ranks.
3. Update CLI/WASM report surfaces and docs.

### 4. Color-Management Profile Support

Problem: current RGB/CMYK detection is heuristic and does not give a useful color inventory for proofing or separations.

Owner: Text Color Agent, `gpt-5.5`, high thinking. This is inventory and proofing guidance, not ICC conversion or gamut validation.

CLI/API surface:

- Extend existing `check` reports with richer color signals; no new command is needed for v1.
- Add `SVGMeta` fields such as `RGBColors`, `NamedColors`, `CMYKColors`, `ICCColors`, `AlphaColors`, `GradientColorStops`, and possibly a compact `ColorSummary` slice later.
- Mirror count fields into WASM API metadata.
- Keep `fix --fix colors` advisory-only.

Implementation sketch:

- Replace or augment `colorSetFrom` with structured color collection from XML attributes, inline `style`, and `<style>` text.
- Classify values from `fill`, `stroke`, `stop-color`, `flood-color`, `lighting-color`, and `color`.
- Count alpha from `rgba`, `hsla`, 8-digit hex, `opacity`, `fill-opacity`, `stroke-opacity`, and `stop-opacity`.
- Detect `device-cmyk(...)`, `cmyk(...)`, and `icc-color(...)` separately instead of lumping all into `CMYKColors`.
- Preserve existing `UniqueColors`, `ColorValues`, and `rgb-colors-for-print` behavior where possible.

Issue codes:

- `rgb-colors-for-print`: existing warning, rank high for paper and packaging; moderate to high for fabric depending color count.
- `cmyk-in-svg`: existing warning, rank moderate; message should say SVG CMYK support is inconsistent.
- `icc-color-in-svg`: warning, rank moderate to high for press targets; final PDF/RIP must preserve profile intent.
- `transparent-color`: warning, rank moderate for print-like targets when alpha-bearing color is present.
- `named-color-for-print`: info, rank low; named SVG colors are web-oriented.
- `gradient-color-inventory`: info or warning, rank low to moderate; keep this about separations/proofing, not flattening.

Tests and docs:

- Add focused tests for hex, rgb, rgba, hsl/hsla, named color, `device-cmyk`, `icc-color`, gradient stops, and opacity.
- Add fixture `print-edge-color-management.svg`.
- Update expectations for existing `print-edge-cmyk-colors.svg`.
- Assert `screen` does not emit print-color warnings, while paper and packaging do.
- Update README to clarify that SVG is not a press color-management container.

Commit slices:

1. Add structured color inventory while preserving current issue behavior.
2. Add new color-management issue rules and ranking.
3. Update CLI/WASM summaries, README, and fixture coverage.

### 5. PDF/X Readiness Or Export Bridge

Problem: press workflows usually need a final PDF/X or printer-specific PDF proof, while this tool currently stops at SVG source inspection.

Owner: PDF Effects Agent, `gpt-5.5`, high thinking. Start with readiness; keep export and validation as optional explicit bridge commands.

CLI/API surface:

- Add `--profile pdfx-ready` to `check`.
- Later, consider explicit commands: `pdfx-tools`, `pdfx-validate proof.pdf`, and `pdfx-export --profile pdfx-4 -o proof.pdf art.svg`.
- Do not require external tools for normal `check`.
- Keep export/tool probing outside `internal/svgcheck`, likely under a future `internal/pdfx` package.

Implementation sketch:

- Add profile parsing plus `ProfilePDFXReady`.
- Add profile-gated readiness issues from existing and future metadata: missing `viewBox`, missing explicit physical size for paper/packaging, external refs, unsafe scripts/events, raster images with unknown proof status, live text, RGB/web color assumptions, filters, masks, opacity, blend modes, shadows, and background transparency.
- Phrase issues as "readiness blocker/risk before export", not "PDF/X invalid", because SVG source cannot prove final PDF conformance.
- Preserve existing issue codes such as `rgb-colors-for-print` and `print-effects-require-flattening`; add PDF/X readiness codes as summary/profile findings, not replacements.

Issue codes:

- `pdfx-readiness-blocker`: error, rank high, for likely dependable handoff blockers under `--profile pdfx-ready`.
- `pdfx-readiness-review`: warning, rank moderate, for proofing concerns that are not always blockers.
- `pdfx-export-tool-missing`: info, only for explicit export/tool commands.
- `pdfx-validation-unavailable`: info, only when validation was requested but no validator is configured.

Tests and docs:

- Add `TestPDFXReadyProfileFlagsExternalRefsAndEffects`.
- Add `TestPDFXReadyProfileDoesNotRunByDefault`.
- Add `TestPDFXReadyProfileSummarizesExistingPrintRisks`.
- Use small inline SVG fixtures for readiness; no PDF fixtures are needed in the first slice.
- Update README with `--profile pdfx-ready` examples and a clear "pre-export readiness, not PDF/X certification" note.

Commit slices:

1. Add `CheckOptions` and `--profile` plumbing.
2. Add `pdfx-ready` profile findings using existing metadata.
3. Add docs and CLI/WASM report serialization updates.
4. Optionally add external tool probing.
5. Optionally add export/validate bridge commands.

### 6. Transparency, Filters, Masks, And Flattening Risk Map

Problem: soft effects are currently detected broadly, but the report does not identify the exact effect types or flattening risks.

Owner: PDF Effects Agent, `gpt-5.5`, high thinking. Build a precise risk inventory while avoiding claims about exact rasterization or visual equivalence.

CLI/API surface:

- Add `--profile flattening-risk`.
- Add summary fields to `SVGMeta` first; richer detailed slices can come later if reports need exact locations.
- Consider a later `EffectRisk` detail model with `Kind`, `Element`, `ID`, `Ref`, `Count`, `Severity`, and `Rank`.

Implementation sketch:

- Extend inspection to inventory filter definitions and primitive names, filter references and referenced IDs, masks and mask references, clip paths and references, opacity attributes, blend modes, CSS filters/shadows, and gradient stops with alpha below 1.
- Add helpers around existing `inspectAttrForPrintSignals` and `inspectStyle` instead of building a broad CSS parser.
- Add `rankFlatteningRisk` helpers by target and effect kind.
- Keep `fix --fix effects` unsafe and conservative.
- For vinyl, laser, CNC, and plotter, keep aggregate `effects-may-not-output`; optionally emit granular codes only when `--profile flattening-risk` is set.

Issue codes:

- `filter-flattening-risk`: error, rank high for paper/packaging and `pdfx-ready`.
- `mask-flattening-risk`: error, rank high for paper/packaging and `pdfx-ready`.
- `blend-mode-flattening-risk`: error, rank high for paper/packaging and `pdfx-ready`.
- `opacity-flattening-risk`: warning, rank moderate.
- `alpha-gradient-flattening-risk`: warning, rank moderate.
- `clip-path-output-risk`: warning, rank moderate.
- `css-effect-flattening-risk`: warning, rank moderate.

Tests and docs:

- Test filter definition/reference, mask definition/reference, clip-path, group opacity, fill opacity, blend mode, gradient stop opacity, and CSS shadow.
- Prove paper/packaging ranks severe effects high.
- Prove screen stays quiet without an explicit profile.
- Prove unsafe `fix --fix effects` still removes only supported constructs.
- Map new effect issue codes to unsafe effects fixes in WASM metadata.
- Update README to name effect categories rather than saying only "soft effects".

Commit slices:

1. Add effect inventory fields and tests.
2. Add granular flattening issue generation.
3. Wire CLI/WASM report output and fix-category mapping.
4. Update README and roadmap wording.

### 7. Overprint, Knockout, White Ink, Underbase, And Spot-Layer Scenarios

Problem: SVG does not model print overprint directly, but production files often rely on layer names, white ink, underbase, or spot-color conventions.

Owner: Text Color Agent, `gpt-5.5`, high thinking. Keep matching conservative and non-absolute because shop naming conventions vary widely.

CLI/API surface:

- Use existing targets: `packaging`, `vinyl`, `sticker`, `fabric`, `paper`, and `signage`.
- Add `SVGMeta` counts such as `WhiteInkCandidates`, `SpotLayerNames`, `UnderbaseCandidates`, `OverprintHints`, and `KnockoutHints`.
- Keep `fix --fix colors` and `fix --fix typography` advisory-only for these codes.
- Surface nonzero counts in Markdown/HTML and WASM metadata.

Implementation sketch:

- Inspect group/layer labels from `id`, `class`, `inkscape:label`, `data-name`, `aria-label`, and child `<title>`.
- Match conservative terms: `white ink`, `spot white`, `underbase`, `base white`, `pantone`, `pms`, `spot`, `varnish`, `dieline`, `cutcontour`, `overprint`, and `knockout`.
- Detect white fills/strokes on non-background elements, especially text or shapes over transparent/no explicit background.
- Detect explicit or pseudo attributes containing `overprint` or `knockout`, plus CSS `mix-blend-mode:multiply` only as an intent hint.
- Do not claim SVG can prove final separations, overprint, or knockout behavior.

Issue codes:

- `white-ink-review`: warning, rank high for packaging, vinyl, and fabric; rank moderate for paper and signage.
- `spot-layer-review`: warning, rank high when spot-like layer names appear in packaging, label, sticker, or fabric targets; rank moderate otherwise.
- `underbase-review`: warning, rank high for fabric, vinyl, and packaging.
- `overprint-intent-unverified`: warning, rank high when explicit overprint/knockout hints are found; rank moderate for blend/name-only hints.
- `white-on-transparent-review`: optional narrow warning, rank moderate, when white art exists without an opaque background.

Tests and docs:

- Add fixture `print-edge-spot-white-underbase.svg`.
- Include white text on transparent background, `inkscape:label="Spot White"`, `id="PANTONE_185_C"`, `class="underbase"`, `data-overprint="true"`, and `style="mix-blend-mode:multiply"`.
- Assert target-specific rank differences for packaging/vinyl/fabric versus screen.
- Assert normal white page backgrounds do not trigger `white-ink-review`.
- Update README with spot/white ink limitations and final PDF/RIP proof language.

Commit slices:

1. Add layer/name/white/underbase/overprint hint inventory.
2. Add target-specific issue rules and false-positive guards for white backgrounds.
3. Add fixtures/tests, WASM/CLI report mapping, and docs.

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

Owner: PDF Effects Agent, `gpt-5.5`, high thinking. This profile should be explicit and stricter than normal web SVG validation.

CLI/API surface:

- Add `--profile static-svg`.
- Accept aliases `print-static` and `upload-static`.
- Reuse the shared profile parsing from PDF/X and flattening-risk work.
- Do not change default behavior for `screen` or web targets.

Implementation sketch:

- Add `ProfileStaticSVG`.
- Extend `SVGMeta` with `AnimationElements`, `AnimatedAttrs`, `ForeignObjects`, `RemoteResources`, `UnsupportedStaticCSS`, and `FragmentReferenceRisks`.
- Detect `<animate>`, `<animateTransform>`, `<animateMotion>`, `<set>`, event attributes, `<foreignObject>`, remote `href`/`src`/CSS URLs, CSS animation/transition properties, and a small denylist of static-renderer-fragile CSS such as `position`, `z-index`, `display:flex`, `display:grid`, `backdrop-filter`, and `mix-blend-mode`.
- Optionally detect unresolved `url(#missing)` after collecting IDs.
- Do not take on hidden-content, off-canvas content, unused defs, or bloat unless directly tied to static renderer interoperability.

Issue codes:

- `animation-not-static`: warning, rank high.
- `foreignobject-not-static`: warning, rank high.
- `remote-resource-not-static`: warning, rank moderate.
- `interactive-svg-not-static`: error, rank high, when scripts or event handlers appear under the static profile.
- `unsupported-css-for-static-svg`: warning, rank moderate.
- `renderer-sensitive-reference`: info, rank low, for unresolved or unusual local references if implemented.

Tests and docs:

- Add `TestStaticSVGProfileFlagsAnimationAndForeignObject`.
- Add `TestStaticSVGProfileFlagsRemoteResources`.
- Add `TestStaticSVGProfileDoesNotPunishScreenByDefault`.
- Add `TestStaticSVGProfileFlagsUnsupportedCSS`.
- Prefer inline SVG snippets; add `static-profile.svg` only if examples become hard to read.
- Update README with `--profile static-svg` and explain that it is stricter than normal SVG validation.
- Add a web demo profile selector only after CLI/API support lands.

Commit slices:

1. Add profile parsing shared with PDF/X and flattening.
2. Add static SVG metadata detection.
3. Add static profile issues and tests.
4. Wire CLI/WASM profile output.
5. Update README and roadmap.

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
