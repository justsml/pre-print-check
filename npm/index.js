import { createRuntime } from "./runtime.js";

const runtime = createRuntime({
  wasmURL: new URL("../dist/pre-print-check.wasm", import.meta.url),
  globalName: "prePrintCheck",
  capabilities: ["check", "overlay", "fix", "fixCategories"],
});

export const loadPrePrintCheck = runtime.loadPrePrintCheck;
export const check = runtime.check;
export const overlay = runtime.overlay;
export const fix = runtime.fix;
export const fixCategories = runtime.fixCategories;
