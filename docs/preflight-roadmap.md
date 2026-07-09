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
	Width    string
	Height   string
	Bleed    string
	SafeMargin string
	BaseDir  string
}

func CheckWithOptions(input []byte, opts CheckOptions) (Report, error)
```

Keep `Check(input, rawTarget)` as a compatibility wrapper around `CheckWithOptions`.

## Product Gaps

### 1. Bleed, Trim, And Safe-Area Checks

Problem: the checker sees root size metadata but cannot evaluate whether artwork has bleed, trim, or safe-area problems for a concrete product.

Owner: Press Geometry Agent, `gpt-5.4`, high thinking. Implement this with gap 2 or in stacked branches because both need target-size resolution, unit conversion, and element-placement math.

CLI/API surface:

- Add `--width`, `--height`, `--bleed`, and `--safe-margin` to `check` and `fix`.
- Example: `pre-print check --target paper --width 4in --height 6in --bleed 0.125in --safe-margin 0.25in art.svg`.
- Add matching `CheckOptions` and `FixOptions` fields.
- Keep width-only target parsing backward compatible.

Implementation sketch:

- Extend `Target` with `HeightInches`, `BleedInches`, and `SafeMarginInches`.
- Add shared length parsing helpers in `internal/svgcheck/target.go` instead of duplicating unit conversion.
- Move layout math into `layout.go` or `production_layout.go`.
- Replace count-only bleed/safe-area helpers with an analyzer that returns a trim box, bleed margin, safe margin, background-at-trim counts, non-background-near-trim counts, and non-background-outside-trim counts.
- Treat the root `viewBox` as trim by default. Do not invent a separate bleed box unless the user supplies bleed.
- Keep `fix` limited to simple full-background rectangle expansion when the geometry is unambiguous.

Issue codes:

- `missing-bleed`: keep existing-style warning, rank moderate.
- `safe-area-risk`: warning, rank by count and distance. Keep this stable instead of renaming to `unsafe-margin`.
- `content-outside-trim`: warning, usually rank high; suppress for background-like bleed elements.
- `target-size-mismatch`: warning, rank moderate, when requested width/height aspect ratio materially disagrees with the SVG.

Tests and docs:

- Add `layout_test.go`.
- Cover missing bleed on edge-to-edge background, explicit overhang pass, text/logo too close to trim, non-background art outside trim, and target aspect mismatch.
- Add fixtures such as `print-edge-missing-bleed.svg`, `print-edge-safe-area.svg`, and `print-edge-content-outside-trim.svg`.
- Update README with new size flags and the rule that `viewBox` is treated as trim.

Commit slices:

1. Add options/flag plumbing and shared unit parsing.
2. Add trim/bleed/safe analyzers and issue codes.
3. Wire bleed fix to the richer target model and update tests/docs.

### 2. Real Embedded Raster PPI Inspection

Problem: current checks count raster images but do not decode local or inline images, inspect intrinsic dimensions, or compute effective PPI at production size.

Owner: Press Geometry Agent, `gpt-5.4`, high thinking. Build on gap 1's target-size and placement model.

CLI/API surface:

- No extra required flags beyond the shared sizing flags.
- Add `BaseDir` to checker options so `CheckFile` can resolve local `href="image.png"` safely relative to the SVG.
- Keep network fetches disabled; only decode `data:` URIs and local relative files.

Implementation sketch:

- Add `internal/svgcheck/raster.go`.
- Scan `<image>` elements and capture `href`, `width`, `height`, `x`, `y`, and `transform`.
- Decode intrinsic raster dimensions from inline and local sources using stdlib `image`, PNG, JPEG, and GIF decoders first.
- Consider `golang.org/x/image` support for WebP/TIFF/BMP only if broader fixture coverage justifies the dependency.
- Compute effective PPI from placed size after scale. Limit transform support to axis-aligned `scale`, `translate`, and `matrix`; for rotate/skew, estimate conservatively or mark unknown.
- Keep detailed per-image structs internal and emit aggregate issues to avoid noisy reports.
- Leave `raster-not-cuttable` unchanged for cutter-like targets.

Issue codes:

- `low-raster-ppi`: warning, usually rank high.
- `modest-raster-ppi`: info, rank moderate.
- `unknown-raster-size`: warning, rank moderate, when placement exists but intrinsic pixels cannot be determined.
- `external-raster-unmeasured`: info, rank low, for remote URLs or unresolved local refs.
- Keep coarse `raster-image` and `inline-raster-image` presence signals only if detailed measurement is unavailable.

Tests and docs:

- Add `raster_test.go`.
- Add tiny real fixtures under `internal/svgcheck/testdata/raster/`, such as `blue-300x300.png`, `photo-600x300.jpg`, and `icon-120x120.gif`.
- Test inline PNG at good PPI, inline PNG at low PPI, local linked raster via `CheckFile`, remote URL unmeasured, and transformed image scale.
- Update README to explain measured inline/local raster PPI and confirm remote resources are never fetched.
- Narrow the current README limitation from "true image DPI/effective PPI" to unresolved/external images and transform ambiguity.

Commit slices:

1. Add path-aware checker options and raster inventory structs.
2. Add inline/local raster decoding plus effective PPI calculation and thresholds.
3. Add tests/fixtures and update README/demo copy.

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

Owner: Toolpath Agent, `gpt-5.4`, high thinking for geometry/parser slices. Keep `fix --fix cutter` advisory-only in v1.

CLI/API surface:

- Extend existing `pre-print check --target vinyl|laser|cnc|plotter`.
- Reuse overlay output to highlight locatable toolpath findings.
- Extend `SVGMeta` and WASM `apiMeta` with counters such as `DuplicateToolpaths`, `DegenerateToolpathSegments`, `FilledOpenPaths`, `SelfIntersectingToolpaths`, and `DenseToolpaths`.
- Do not add a generic `open-toolpath` issue in v1. Open centerlines can be valid for engraving/scoring; flag only open paths that visually imply a closed cut shape.

Implementation sketch:

- Add profile booleans such as `ReviewToolpathHealth` and `ReviewFilledOpenPaths` for vinyl, laser, CNC, and plotter.
- Add `internal/svgcheck/toolpath.go`.
- Build a reusable geometry pass that produces normalized contours/segments from `line`, `polyline`, `polygon`, and `path`.
- Include affine transform-stack support for element/group `transform`; this is a release blocker for reliable toolpath health.
- Canonicalize segments so reversed duplicates match.
- Add bounded curve/arc flattening for `C`, `Q`, `S`, `T`, and `A` path commands.
- Track contour metadata: closed/open, filled/stroked, segment count, total length, and minimum segment length.
- Keep existing `near-disconnected-lines`, `thin-stroke`, and `small-detail-durability` behavior intact.
- Add overlay highlight groups such as `pre-print-duplicate-toolpath-highlights`, `pre-print-degenerate-toolpath-highlights`, and `pre-print-self-intersection-highlights`.

Issue codes:

- `duplicate-toolpath`: error, rank high, for overlapping duplicate segments in either direction.
- `degenerate-toolpath-segment`: warning, rank low/moderate/high by count.
- `filled-open-path`: error for vinyl/plotter and warning for laser/CNC; rank moderate/high.
- `self-intersecting-toolpath`: warning, rank moderate/high by intersection count.
- `dense-toolpath`: info, rank moderate/high when segment count or very short median segment length suggests stutter, burn, chatter, or rough cutting.

Tests and docs:

- Add `toolpath_test.go`.
- Add fixtures: `toolpath-duplicate-segments.svg`, `toolpath-degenerate-segments.svg`, `toolpath-filled-open-path.svg`, `toolpath-self-intersection.svg`, `toolpath-dense-nodes.svg`, and `toolpath-transformed-duplicates.svg`.
- Assert issue code, severity, rank, and `SVGMeta` counters.
- Add overlay tests that assert highlight group IDs/issue presence, not full SVG equality.
- Update README to say v1 is check-only, not toolpath auto-repair.

Commit slices:

1. Add transformed contour/segment extraction without user-visible behavior changes.
2. Add objective blocker checks: duplicate, degenerate, and filled-open paths.
3. Add advisory checks: self-intersecting and dense toolpaths.
4. Wire metadata counters, overlay highlights, and WASM/API exposure.
5. Update README/docs/test corpus.
6. Later, investigate a narrow `--fix cutter --unsafe` for exact duplicate removal only.

### 9. Hidden, Nonprinting, Off-Canvas, Or Bloated Content

Problem: hidden or off-canvas content can unexpectedly print, affect bounds, bloat files, or slow downstream tools.

Owner: Press Geometry Agent, `gpt-5.4`, high thinking. Reuse gap 1's trim/viewBox helpers and keep cleanup fixes separate from detection.

CLI/API surface:

- No initial new flags; run advisory detection by default in `check`.
- Do not add auto-cleanup to `fix` in the first slice.
- If cleanup lands later, make it a separate category, likely `cleanup`, and probably gate it with `--unsafe`.

Implementation sketch:

- Add `internal/svgcheck/visibility.go`.
- Classify hidden content via `display:none`, `visibility:hidden`, zero opacity, zero-size geometry, and empty paths.
- Detect elements fully outside the trim/viewBox by a meaningful distance.
- Collect IDs defined in `<defs>` and IDs referenced via `url(#id)`, `href="#id"`, and `xlink:href="#id"`.
- Keep detection conservative: `opacity:0` and `display:none` are strong signals; `fill="none"` alone is not.
- Treat zero-area bounds plus no stroke as a stronger empty-geometry signal than zero bounds alone.
- Start bloat detection with lightweight heuristics: large inline data URIs in hidden/off-canvas elements, many unused defs, or hidden/off-canvas content that appears to be a meaningful share of the file.

Issue codes:

- `hidden-content`: info, rank by count/bytes.
- `invisible-content`: info, rank low.
- `off-canvas-content`: warning, rank moderate/high when far outside trim or numerous.
- `unused-defs`: info, rank low.
- `empty-geometry`: info, rank low.
- `bloated-content`: warning, rank moderate/high when hidden or unreachable payload is materially large.

Tests and docs:

- Add `visibility_test.go`.
- Cover `display:none` groups, `visibility:hidden` shapes, zero-opacity placed images, empty paths, zero-size rects, unused defs, and off-canvas art far beyond the viewBox.
- Generate large hidden data URIs in test code rather than checking in a giant SVG fixture.
- Update README to frame these as advisory cleanup signals.

Commit slices:

1. Add hidden/invisible/empty/off-canvas inventory and issues.
2. Add defs reference tracking and `unused-defs`.
3. Add heuristic `bloated-content`, tests, and README/demo updates.

Sequencing:

1. Add shared options plumbing for `CheckOptions`, width, height, bleed, safe margin, and base dir.
2. Build the gap 1 geometry model.
3. Build gap 2 raster measurement on top of that model.
4. Build gap 9 visibility/bloat checks reusing the same trim and bounds helpers.

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

## Cross-Cutting Test Strategy

- Prefer semantic report assertions over full rendered-report snapshots.
- Keep behavior tests in `internal/svgcheck`; keep CLI tests focused on argument parsing, exit codes, and write paths.
- For fixer work, use round-trip tests: `Check`, then `Fix`, then `Check`, and assert only intended issues disappear.
- Add fuzz targets for untrusted SVG handling on `Check`, `Fix`, and `GenerateOverlay` after the geometry and profile APIs settle.
- Maintain at least one transformed fixture and one malformed-but-parseable fixture in the shared corpus because they exercise many roadmap gaps without redefining their scope.
