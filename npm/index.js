const defaultWasmURL = new URL("../dist/pre-print.wasm", import.meta.url);
const defaultWasmExecURL = new URL("../dist/wasm_exec.js", import.meta.url);

let runtimePromise;

export async function loadPrePrint(options = {}) {
  if (!runtimePromise) {
    runtimePromise = bootRuntime(options);
  }
  return runtimePromise;
}

export async function check(svg, options = {}) {
  const api = await loadPrePrint();
  return api.check(svg, options);
}

export async function overlay(svg, options = {}) {
  const api = await loadPrePrint();
  return api.overlay(svg, options);
}

export async function fix(svg, options = {}) {
  const api = await loadPrePrint();
  return api.fix(svg, options);
}

export async function fixCategories() {
  const api = await loadPrePrint();
  return api.fixCategories();
}

async function bootRuntime(options) {
  await loadGoRuntime(options.wasmExecURL || defaultWasmExecURL);

  if (globalThis.prePrintTools?.ready) {
    return createAPI(globalThis.prePrintTools);
  }

  const GoCtor = globalThis.Go;
  if (typeof GoCtor !== "function") {
    throw new Error("Go WASM runtime did not register globalThis.Go");
  }

  const go = new GoCtor();
  const wasmURL = options.wasmURL || defaultWasmURL;
  const { instance } = await instantiateWasm(wasmURL, go.importObject);
  const runPromise = go.run(instance);
  runPromise.catch((error) => {
    runtimePromise = undefined;
    setTimeout(() => {
      throw error;
    }, 0);
  });

  await waitForReady(options.timeoutMs || 4000);
  return createAPI(globalThis.prePrintTools);
}

async function loadGoRuntime(wasmExecURL) {
  if (typeof globalThis.Go === "function") {
    return;
  }
  await import(toImportSpecifier(wasmExecURL));
}

async function instantiateWasm(wasmURL, importObject) {
  const url = toURL(wasmURL);

  if (isNode() && url.protocol === "file:") {
    const { readFile } = await import("node:fs/promises");
    const bytes = await readFile(url);
    return WebAssembly.instantiate(bytes, importObject);
  }

  const response = await fetch(url);
  if (!response.ok) {
    throw new Error(`Could not load pre-print WASM: ${response.status} ${response.statusText}`);
  }
  if (WebAssembly.instantiateStreaming) {
    try {
      return await WebAssembly.instantiateStreaming(response.clone(), importObject);
    } catch {
      // Static servers often omit application/wasm. Fall back to bytes.
    }
  }
  const bytes = await response.arrayBuffer();
  return WebAssembly.instantiate(bytes, importObject);
}

function createAPI(rawAPI) {
  return {
    check(svg, options = {}) {
      return call(rawAPI, "check", svg, options).report;
    },
    overlay(svg, options = {}) {
      return call(rawAPI, "overlay", svg, options).overlay;
    },
    fix(svg, options = {}) {
      const response = call(rawAPI, "fix", svg, normalizeFixOptions(options));
      return {
        svg: response.svg,
        changes: response.changes || [],
        skipped: response.skipped || [],
        report: response.report,
        overlay: response.overlay || "",
      };
    },
    fixCategories() {
      return call(rawAPI, "fixCategories", "", {}).fixCategories || [];
    },
  };
}

function call(rawAPI, name, svg, options) {
  const fn = rawAPI?.[name];
  if (typeof fn !== "function") {
    throw new Error(`pre-print WASM API is missing ${name}()`);
  }
  const response = JSON.parse(fn(String(svg ?? ""), options || {}));
  if (!response.ok) {
    const error = new Error(response.error || `pre-print ${name} failed`);
    error.response = response;
    throw error;
  }
  return response;
}

function normalizeFixOptions(options) {
  if (!options.categories && options.fix) {
    return { ...options, categories: options.fix };
  }
  return options;
}

function waitForReady(timeoutMs) {
  return new Promise((resolve, reject) => {
    const started = Date.now();
    const tick = () => {
      if (globalThis.prePrintTools?.ready) {
        resolve();
        return;
      }
      if (Date.now() - started > timeoutMs) {
        reject(new Error("pre-print WASM API did not become ready"));
        return;
      }
      setTimeout(tick, 0);
    };
    tick();
  });
}

function toImportSpecifier(value) {
  if (value instanceof URL) {
    return value.href;
  }
  return String(value);
}

function toURL(value) {
  if (value instanceof URL) {
    return value;
  }
  return new URL(String(value), globalThis.location?.href || import.meta.url);
}

function isNode() {
  return typeof process !== "undefined" && Boolean(process.versions?.node);
}
