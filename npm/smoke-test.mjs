import assert from "node:assert/strict";
import { check, fix, fixCategories, overlay } from "./index.js";

const unsafeSVG = `<svg width="100" height="50"><script>alert(1)</script><rect onclick="x()" width="100" height="50" /></svg>`;

const report = await check(unsafeSVG, { target: "vinyl" });
assert.equal(report.counts.errors, 2);
assert.ok(report.issues.some((issue) => issue.code === "script"));
assert.ok(report.issues.some((issue) => issue.code === "event-handler"));

const categories = await fixCategories();
assert.ok(categories.includes("metadata"));
assert.ok(categories.includes("safety"));

const fixed = await fix(unsafeSVG, {
  categories: ["metadata", "safety"],
  unsafe: true,
});
assert.ok(fixed.svg.includes(`xmlns="http://www.w3.org/2000/svg"`));
assert.ok(fixed.svg.includes(`viewBox="0 0 100 50"`));
assert.equal(fixed.svg.includes("<script"), false);
assert.equal(fixed.svg.includes("onclick"), false);
assert.ok(fixed.changes.length >= 3);

const overlaySVG = await overlay(fixed.svg, { target: "vinyl" });
assert.ok(overlaySVG.includes("<svg"));

console.log("pre-print-check WASM smoke test passed");
