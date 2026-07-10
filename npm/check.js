import { createRuntime } from "./runtime.js";

const runtime = createRuntime({
  wasmURL: new URL("../dist/pre-print-check-check.wasm", import.meta.url),
  globalName: "prePrintCheckCheck",
  capabilities: ["check"],
});

export const loadPrePrintCheck = runtime.loadPrePrintCheck;
export const check = runtime.check;
