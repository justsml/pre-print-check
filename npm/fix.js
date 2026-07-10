import { createRuntime } from "./runtime.js";

const runtime = createRuntime({
  wasmURL: new URL("../dist/pre-print-check-fix.wasm", import.meta.url),
  globalName: "prePrintCheckFix",
  capabilities: ["fix", "fixCategories"],
});

export const loadPrePrintCheck = runtime.loadPrePrintCheck;
export const fix = runtime.fix;
export const fixCategories = runtime.fixCategories;
