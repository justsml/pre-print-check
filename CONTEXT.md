# SVG Preflight

SVG Preflight identifies production risks in SVG artwork and applies narrowly scoped repairs without claiming to prove a downstream press, cutter, engraver, or RIP result.

## Language

**Analysis**:
The collected metadata, geometry evidence, and locatable artwork facts derived from one SVG for a target.
_Avoid_: Inspection details, scan result

**Geometry Evidence**:
The interpreted paths, styles, placement, physical measurements, and locatable relationships derived from SVG artwork during analysis.
_Avoid_: Raw geometry, overlay data

**Finding**:
A stable, coded production risk or informational observation supported by an analysis.
_Avoid_: Error message, check result

**Report**:
The findings, metadata, target interpretation, and summaries produced from an SVG analysis.
_Avoid_: Analysis, raw scan

**Portable Report**:
The stable serialized projection of a report consumed by WASM, npm, browser, and other non-Go adapters.
_Avoid_: Web report, API report

**Remediation**:
The operational policy attached to a finding: whether it has an automatic fix, which fix category owns it, whether it requires unsafe mode, and which manual guidance applies.
_Avoid_: Fix action, issue mapping

**Target**:
The intended output material, physical size, or raster resolution used to interpret an SVG analysis.
_Avoid_: Destination

**Profile**:
An explicit set of additional interoperability or production-readiness constraints applied alongside a target.
_Avoid_: Target, preset
