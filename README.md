# pre-print

`pre-print` is a small Go CLI for checking and repairing SVGs before they head to print, cutter, engraving, large-format, or web pipelines.

It is a lightweight preflight assistant, not a replacement for a printer proof or a production RIP. Its job is to catch common file risks early: unsafe SVG content, missing sizing metadata, raster artwork hidden inside vectors, RGB/web color assumptions, soft effects that may flatten poorly, and target-specific incompatibilities.

## Build

```sh
make build
./pre-print check --target vinyl art.svg
```

## Commands

```sh
./pre-print check --target paper art.svg
./pre-print check --target vinyl --format md art.svg
./pre-print check --target 20ft --format html art.svg
./pre-print check --target 4k art.svg
./pre-print fix -o art.fixed.svg art.svg
./pre-print fix --unsafe art.svg
```

`check` produces a friendly preflight summary in terminal text by default, or Markdown/HTML with `--format md` or `--format html`.

It reports issues such as:

- missing `viewBox`, missing SVG namespace, and missing explicit size
- scripts and inline event handlers
- external references that may fail offline or during print handoff
- raster and inline base64 raster images inside the SVG
- subjective ranked findings such as color count and raster-image count
- shadow-style effects, filters, masks, clipping, opacity, and blend modes
- rough target-size effective resolution concerns
- target-specific production risks, such as raster art in cutter/engraver output or RGB/web color assumptions for paper/packaging output

Targets may be estimated sizes/resolutions such as `20ft`, `1.2m`, `90in`, `4k`, or `8k`, or common output intents/materials:

- `screen` / `web`
- `paper` / `print`
- `fabric` / `textile` / `apparel`
- `vinyl` / `sticker` / `decal`
- `banner`
- `signage`
- `vehicle-wrap`
- `packaging` / `label`
- `laser` / `engraving`
- `cnc`
- `plotter`

`fix` currently makes conservative changes by default:

- add the SVG namespace when missing
- add a `viewBox` from numeric `width` and `height` when missing

With `--unsafe`, it may also remove script elements and inline event handler attributes.

## Preflight model

The checker follows a practical print-shop split:

- **Objective blockers** are things the tool can detect directly, such as scripts, event handlers, missing root metadata, external resources, image elements, filter elements, and target/output incompatibilities.
- **Subjective findings** are ranked as `low`, `moderate`, or `high` because they need human judgment. Examples include the number of colors, number of raster images, and soft visual effects.
- **Target-specific rules** get stricter when the output demands it. Paper and packaging targets warn about RGB/web color assumptions and error on effects that should be flattened or proofed in a press-ready PDF workflow. Vinyl, laser, CNC, and plotter targets error on raster/effect content because those workflows usually need clean path geometry.

The core prepress ideas are intentionally practical: confirm the file size, keep important content inside the safe area, include bleed/crop requirements in the final print artifact, use high-resolution raster art when raster art is unavoidable, avoid RGB surprises in print workflows, outline/package fonts when handing off editable files, and proof effects/transparency before production.

## Limits

SVG is a web/vector interchange format, so some production facts cannot be proven from the SVG alone:

- true image DPI/effective PPI for embedded base64 images
- final CMYK/spot-color conversion quality
- bleed, trim, safe zone, folds, gutters, and imposition
- font licensing or font availability outside the SVG
- RIP-specific handling of transparency, blend modes, masks, filters, and shadows

For press output, use this tool before exporting or reviewing the final press-ready PDF. Then confirm the printer's product template, bleed, trim, safe zone, color, font, image-resolution, and proofing requirements.
