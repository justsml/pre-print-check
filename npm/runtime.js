const defaultWasmExecURL = new URL("../dist/wasm_exec.js", import.meta.url);

export function createRuntime(config) {
  let runtimePromise;

  async function loadPrePrintCheck(options = {}) {
    if (!runtimePromise) {
      runtimePromise = bootRuntime(config, options);
    }
    return runtimePromise;
  }

  return {
    loadPrePrintCheck,
    async check(svg, options = {}) {
      const api = await loadPrePrintCheck();
      return api.check(svg, options);
    },
    async overlay(svg, options = {}) {
      const api = await loadPrePrintCheck();
      return api.overlay(svg, options);
    },
    async fix(svg, options = {}) {
      const api = await loadPrePrintCheck();
      return api.fix(svg, options);
    },
    async fixCategories() {
      const api = await loadPrePrintCheck();
      return api.fixCategories();
    },
  };

  async function bootRuntime(runtimeConfig, options) {
    const globalName = runtimeConfig.globalName;
    await loadGoRuntime(options.wasmExecURL || runtimeConfig.wasmExecURL || defaultWasmExecURL);

    if (globalThis[globalName]?.ready) {
      return createAPI(globalThis[globalName], runtimeConfig.capabilities);
    }

    const GoCtor = globalThis.Go;
    if (typeof GoCtor !== "function") {
      throw new Error("Go WASM runtime did not register globalThis.Go");
    }

    const go = new GoCtor();
    const wasmURL = options.wasmURL || runtimeConfig.wasmURL;
    const { instance } = await instantiateWasm(wasmURL, go.importObject);
    const runPromise = go.run(instance);
    runPromise.catch((error) => {
      runtimePromise = undefined;
      setTimeout(() => {
        throw error;
      }, 0);
    });

    await waitForReady(globalName, options.timeoutMs || 4000);
    return createAPI(globalThis[globalName], runtimeConfig.capabilities);
  }
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
    throw new Error(`Could not load pre-print-check WASM: ${response.status} ${response.statusText}`);
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

function createAPI(rawAPI, capabilities) {
  const api = {};
  if (capabilities.includes("check")) {
    api.check = (svg, options = {}) => call(rawAPI, "check", svg, options).report;
  }
  if (capabilities.includes("overlay")) {
    api.overlay = (svg, options = {}) => call(rawAPI, "overlay", svg, options).overlay;
  }
  if (capabilities.includes("fix")) {
    api.fix = (svg, options = {}) => {
      const response = call(rawAPI, "fix", svg, normalizeFixOptions(options));
      return {
        svg: response.svg,
        changes: response.changes || [],
        skipped: response.skipped || [],
      };
    };
  }
  if (capabilities.includes("fixCategories")) {
    api.fixCategories = () => call(rawAPI, "fixCategories", "", {}).fixCategories || [];
  }
  return api;
}

function call(rawAPI, name, svg, options) {
  const fn = rawAPI?.[name];
  if (typeof fn !== "function") {
    throw new Error(`pre-print-check WASM API is missing ${name}()`);
  }
  const response = JSON.parse(fn(String(svg ?? ""), options || {}));
  if (!response.ok) {
    const error = new Error(response.error || `pre-print-check ${name} failed`);
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

function waitForReady(globalName, timeoutMs) {
  return new Promise((resolve, reject) => {
    const started = Date.now();
    const tick = () => {
      if (globalThis[globalName]?.ready) {
        resolve();
        return;
      }
      if (Date.now() - started > timeoutMs) {
        reject(new Error("pre-print-check WASM API did not become ready"));
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
