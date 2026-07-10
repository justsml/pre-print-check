import assert from "node:assert/strict";
import { createRequire } from "node:module";
import { check, fix, fixCategories, loadPrePrintCheck, overlay } from "pre-print-check";
import { check as checkOnly, loadPrePrintCheck as loadCheckOnly } from "pre-print-check/check";
import { fix as fixOnly, fixCategories as fixOnlyCategories, loadPrePrintCheck as loadFixOnly } from "pre-print-check/fix";

const require = createRequire(import.meta.url);
const unsafeSVG = `<svg width="100" height="50"><script>alert(1)</script><rect onclick="x()" width="100" height="50" /></svg>`;

assert.equal(typeof require("pre-print-check").check, "function");
assert.equal(typeof require("pre-print-check/check").check, "function");
assert.equal(typeof require("pre-print-check/fix").fix, "function");

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

const fullAPI = await loadPrePrintCheck();
assert.equal(typeof fullAPI.check, "function");
assert.equal(typeof fullAPI.fix, "function");
assert.equal(typeof fullAPI.overlay, "function");

const checkAPI = await loadCheckOnly();
assert.equal(typeof checkAPI.check, "function");
assert.equal(checkAPI.fix, undefined);
assert.equal(checkAPI.overlay, undefined);

const checkOnlyReport = await checkOnly(unsafeSVG, { target: "vinyl" });
assert.equal(checkOnlyReport.counts.errors, 2);

const fixAPI = await loadFixOnly();
assert.equal(typeof fixAPI.fix, "function");
assert.equal(typeof fixAPI.fixCategories, "function");
assert.equal(fixAPI.check, undefined);
assert.equal(fixAPI.overlay, undefined);

const scopedCategories = await fixOnlyCategories();
assert.ok(scopedCategories.includes("metadata"));

const scopedFixed = await fixOnly(unsafeSVG, {
  categories: ["metadata", "safety"],
  unsafe: true,
});
assert.equal(scopedFixed.svg.includes("<script"), false);
assert.ok(scopedFixed.changes.length >= 3);

console.log("pre-print-check WASM smoke test passed");
