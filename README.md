# pre-print-tools

A small Go CLI for checking and repairing SVGs before they head to print or web pipelines.

## Commands

```sh
go run . check --target 20ft art.svg
go run . check --target 4k art.svg
go run . check --target vinyl art.svg
go run . fix -o art.fixed.svg art.svg
go run . fix --unsafe art.svg
```

`check` reports issues such as missing `viewBox`, missing SVG namespace, scripts, inline event handlers, external references, raster images, and rough target-size resolution concerns.

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
