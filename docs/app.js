const state = {
  wasmReady: false,
  currentSVG: "",
  originalSVG: "",
  overlaySVG: "",
  fileName: "",
  view: "original",
  report: null,
  urls: new Set(),
};

const el = {
  runtimeStatus: document.querySelector("#runtimeStatus"),
  summaryLine: document.querySelector("#summaryLine"),
  fileInput: document.querySelector("#fileInput"),
  sampleSelect: document.querySelector("#sampleSelect"),
  targetSelect: document.querySelector("#targetSelect"),
  customTargetWrap: document.querySelector("#customTargetWrap"),
  customTarget: document.querySelector("#customTarget"),
  unsafeToggle: document.querySelector("#unsafeToggle"),
  analyzeButton: document.querySelector("#analyzeButton"),
  fixSafeButton: document.querySelector("#fixSafeButton"),
  downloadSvgButton: document.querySelector("#downloadSvgButton"),
  downloadOverlayButton: document.querySelector("#downloadOverlayButton"),
  applyReportFixesButton: document.querySelector("#applyReportFixesButton"),
  clearLogButton: document.querySelector("#clearLogButton"),
  fileName: document.querySelector("#fileName"),
  svgPreview: document.querySelector("#svgPreview"),
  emptyVisual: document.querySelector("#emptyVisual"),
  sourceInput: document.querySelector("#sourceInput"),
  visualPane: document.querySelector("#visualPane"),
  reportSummary: document.querySelector("#reportSummary"),
  errorCount: document.querySelector("#errorCount"),
  warningCount: document.querySelector("#warningCount"),
  infoCount: document.querySelector("#infoCount"),
  metaGrid: document.querySelector("#metaGrid"),
  issuesList: document.querySelector("#issuesList"),
  activityLog: document.querySelector("#activityLog"),
};

const severityStyles = {
  error: "bg-rose-500 text-rose-700",
  warning: "bg-amber-500 text-amber-700",
  info: "bg-sky-500 text-sky-700",
};

document.addEventListener("DOMContentLoaded", () => {
  wireEvents();
  setControlsEnabled(false);
  render();
  bootWasm();
  if (window.lucide) {
    window.lucide.createIcons();
  }
});

function wireEvents() {
  el.fileInput.addEventListener("change", loadFile);
  el.sampleSelect.addEventListener("change", loadSample);
  el.targetSelect.addEventListener("change", () => {
    el.customTargetWrap.classList.toggle("hidden", el.targetSelect.value !== "custom");
    analyzeCurrent();
  });
  el.customTarget.addEventListener("change", analyzeCurrent);
  el.unsafeToggle.addEventListener("change", renderIssues);
  el.analyzeButton.addEventListener("click", analyzeCurrent);
  el.fixSafeButton.addEventListener("click", () => applyFixes(["metadata", "bleed"], false));
  el.applyReportFixesButton.addEventListener("click", applyReportFixes);
  el.downloadSvgButton.addEventListener("click", () => downloadText(state.currentSVG, fileStem() + ".svg", "image/svg+xml"));
  el.downloadOverlayButton.addEventListener("click", () => downloadText(state.overlaySVG, fileStem() + ".overlay.svg", "image/svg+xml"));
  el.clearLogButton.addEventListener("click", () => {
    el.activityLog.innerHTML = "";
  });
  el.sourceInput.addEventListener("input", () => {
    state.currentSVG = el.sourceInput.value;
    state.originalSVG = state.originalSVG || state.currentSVG;
    refreshPreview();
  });
  document.querySelectorAll("[data-view]").forEach((button) => {
    button.addEventListener("click", () => {
      state.view = button.dataset.view;
      render();
    });
  });
}

async function bootWasm() {
  try {
    setStatus("Loading", "bg-sky-100 text-sky-700", "bg-sky-500");
    const go = new Go();
    const response = await fetch("../dist/pre-print.wasm");
    const result = await instantiateWasm(response, go.importObject);
    go.run(result.instance);
    await waitFor(() => window.prePrintTools?.ready);
    state.wasmReady = true;
    setControlsEnabled(true);
    setStatus("Ready", "bg-emerald-100 text-emerald-700", "bg-emerald-500");
    el.summaryLine.textContent = "Local SVG preflight";
    addLog("WASM runtime ready");
  } catch (error) {
    setStatus("Build needed", "bg-amber-100 text-amber-800", "bg-amber-500");
    el.summaryLine.textContent = "Run make wasm, then refresh";
    addLog(error.message || String(error), "border-amber-400");
  }
}

async function instantiateWasm(response, importObject) {
  if (WebAssembly.instantiateStreaming) {
    try {
      return await WebAssembly.instantiateStreaming(response.clone(), importObject);
    } catch {
      // Some static servers do not send application/wasm.
    }
  }
  const bytes = await response.arrayBuffer();
  return WebAssembly.instantiate(bytes, importObject);
}

function waitFor(predicate) {
  return new Promise((resolve, reject) => {
    const started = performance.now();
    const tick = () => {
      if (predicate()) {
        resolve();
        return;
      }
      if (performance.now() - started > 4000) {
        reject(new Error("WASM API did not register"));
        return;
      }
      requestAnimationFrame(tick);
    };
    tick();
  });
}

async function loadFile() {
  const file = el.fileInput.files?.[0];
  if (!file) return;
  const text = await file.text();
  setSVG(text, file.name);
  addLog(`Loaded ${file.name}`);
  await analyzeCurrent();
}

async function loadSample() {
  if (!el.sampleSelect.value) return;
  const response = await fetch(el.sampleSelect.value);
  if (!response.ok) {
    addLog(`Could not load sample: ${response.status}`, "border-rose-400");
    return;
  }
  const text = await response.text();
  const name = el.sampleSelect.options[el.sampleSelect.selectedIndex].textContent.trim();
  setSVG(text, `${name}.svg`);
  addLog(`Loaded ${name}`);
  await analyzeCurrent();
}

function setSVG(svg, name) {
  state.currentSVG = svg;
  state.originalSVG = svg;
  state.overlaySVG = "";
  state.fileName = name || "untitled.svg";
  el.sourceInput.value = svg;
  refreshPreview();
  render();
}

async function analyzeCurrent() {
  if (!state.wasmReady || !state.currentSVG.trim()) {
    return;
  }
  const target = currentTarget();
  const checkResponse = callTool("check", state.currentSVG, { target });
  state.report = checkResponse.report;
  const overlayResponse = callTool("overlay", state.currentSVG, { target });
  state.overlaySVG = overlayResponse.overlay || "";
  addLog(`Checked ${target || "default target"}`);
  refreshPreview();
  render();
}

function callTool(name, svg, options) {
  const raw = window.prePrintTools[name](svg, options || {});
  const response = JSON.parse(raw);
  if (!response.ok) {
    throw new Error(response.error || `${name} failed`);
  }
  return response;
}

async function applyReportFixes() {
  const categories = state.report?.fixCategories || [];
  if (categories.length === 0) {
    addLog("No automatic fixes are available");
    return;
  }
  await applyFixes(categories, el.unsafeToggle.checked);
}

async function applyIssueFix(category, unsafeRequired) {
  if (!category) return;
  if (unsafeRequired && !el.unsafeToggle.checked) {
    addLog("Enable unsafe fixes for that change", "border-amber-400");
    return;
  }
  await applyFixes([category], unsafeRequired || el.unsafeToggle.checked);
}

async function applyFixes(categories, unsafe) {
  if (!state.wasmReady || !state.currentSVG.trim()) {
    return;
  }
  try {
    const response = callTool("fix", state.currentSVG, {
      target: currentTarget(),
      categories,
      unsafe,
    });
    state.currentSVG = response.svg || state.currentSVG;
    state.overlaySVG = response.overlay || "";
    state.report = response.report || state.report;
    el.sourceInput.value = state.currentSVG;
    refreshPreview();
    render();
    if (response.changes?.length) {
      response.changes.forEach((change) => addLog(change, "border-emerald-400"));
    } else {
      addLog("No SVG changes were needed");
    }
    response.skipped?.forEach((skipped) => addLog(skipped, "border-amber-400"));
  } catch (error) {
    addLog(error.message || String(error), "border-rose-400");
  }
}

function currentTarget() {
  if (el.targetSelect.value === "custom") {
    return el.customTarget.value.trim();
  }
  return el.targetSelect.value;
}

function refreshPreview() {
  let svg = state.currentSVG;
  if (state.view === "overlay" && state.overlaySVG) {
    svg = state.overlaySVG;
  }
  if (!svg.trim()) {
    el.svgPreview.removeAttribute("src");
    return;
  }
  const url = URL.createObjectURL(new Blob([svg], { type: "image/svg+xml" }));
  state.urls.add(url);
  el.svgPreview.src = url;
  setTimeout(() => {
    for (const old of state.urls) {
      if (old !== url) {
        URL.revokeObjectURL(old);
        state.urls.delete(old);
      }
    }
  }, 250);
}

function render() {
  document.querySelectorAll("[data-view]").forEach((button) => {
    button.classList.toggle("active", button.dataset.view === state.view);
  });

  const sourceMode = state.view === "source";
  el.visualPane.classList.toggle("hidden", sourceMode);
  el.sourceInput.classList.toggle("hidden", !sourceMode);
  el.emptyVisual.classList.toggle("hidden", Boolean(state.currentSVG.trim()));
  el.fileName.textContent = state.fileName || "No file selected";
  refreshPreview();
  renderReport();
  renderIssues();
  if (window.lucide) {
    window.lucide.createIcons();
  }
}

function renderReport() {
  const report = state.report;
  el.reportSummary.textContent = report?.friendlySummary || "Ready";
  el.errorCount.textContent = report?.counts?.errors ?? 0;
  el.warningCount.textContent = report?.counts?.warnings ?? 0;
  el.infoCount.textContent = report?.counts?.info ?? 0;

  const meta = report?.meta || {};
  const rows = [
    ["Width", meta.width || "-"],
    ["Height", meta.height || "-"],
    ["ViewBox", meta.viewBox || "-"],
    ["Colors", meta.uniqueColors ?? 0],
    ["Raster", (meta.rasterImages ?? 0) + (meta.inlineRasterImages ?? 0)],
    ["Effects", (meta.filters ?? 0) + (meta.filterRefs ?? 0) + (meta.shadows ?? 0)],
    ["Thin", meta.thinStrokes ?? 0],
    ["Gaps", meta.nearDisconnected ?? 0],
  ];
  el.metaGrid.innerHTML = rows.map(([term, value]) => `
    <div>
      <dt>${escapeHTML(term)}</dt>
      <dd>${escapeHTML(String(value))}</dd>
    </div>
  `).join("");
}

function renderIssues() {
  const issues = state.report?.issues || [];
  if (issues.length === 0) {
    el.issuesList.innerHTML = `<div class="rounded-lg border border-dashed border-slate-300 bg-slate-50 p-4 text-sm text-slate-500">No issues reported</div>`;
    return;
  }

  el.issuesList.innerHTML = issues.map((issue, index) => {
    const styles = severityStyles[issue.severity] || severityStyles.info;
    const canApply = issue.automaticFix && (!issue.unsafeRequired || el.unsafeToggle.checked);
    const actionText = issue.automaticFix
      ? issue.unsafeRequired && !el.unsafeToggle.checked
        ? "Needs unsafe"
        : `Apply ${issue.fixCategory}`
      : "Manual";
    return `
      <article class="issue-card" tabindex="0">
        <div class="flex items-start justify-between gap-3">
          <div class="min-w-0">
            <div class="flex items-center gap-2">
              <span class="severity-mark ${styles.split(" ")[0]}"></span>
              <span class="truncate text-sm font-semibold text-slate-950">${escapeHTML(issue.code)}</span>
            </div>
            <div class="mt-1 flex flex-wrap items-center gap-2 text-xs font-semibold uppercase text-slate-500">
              <span class="${styles.split(" ")[1]}">${escapeHTML(issue.severity)}</span>
              ${issue.rank ? `<span>${escapeHTML(issue.rank)}</span>` : ""}
            </div>
          </div>
          <button
            class="command-button h-8 ${canApply ? "bg-white text-slate-800 hover:bg-slate-50" : "bg-slate-100 text-slate-400"}"
            data-issue-fix="${index}"
            ${canApply ? "" : "disabled"}
          >
            <i data-lucide="${issue.automaticFix ? "wand-sparkles" : "hand"}" class="h-3.5 w-3.5"></i>
            ${escapeHTML(actionText)}
          </button>
        </div>
        <p class="issue-message">${escapeHTML(issue.message)}</p>
      </article>
    `;
  }).join("");

  el.issuesList.querySelectorAll("[data-issue-fix]").forEach((button) => {
    button.addEventListener("click", () => {
      const issue = issues[Number(button.dataset.issueFix)];
      applyIssueFix(issue.fixCategory, issue.unsafeRequired);
    });
  });
}

function setControlsEnabled(enabled) {
  [
    el.fileInput,
    el.sampleSelect,
    el.targetSelect,
    el.customTarget,
    el.unsafeToggle,
    el.analyzeButton,
    el.fixSafeButton,
    el.downloadSvgButton,
    el.downloadOverlayButton,
    el.applyReportFixesButton,
  ].forEach((node) => {
    node.disabled = !enabled;
  });
}

function setStatus(text, pillClass, dotClass) {
  el.runtimeStatus.className = `status-pill ${pillClass}`;
  el.runtimeStatus.innerHTML = `<span class="status-dot ${dotClass}"></span>${escapeHTML(text)}`;
}

function addLog(message, border = "border-slate-300") {
  const item = document.createElement("div");
  item.className = `activity-item ${border}`;
  item.textContent = message;
  el.activityLog.prepend(item);
}

function downloadText(text, filename, type) {
  if (!text?.trim()) {
    addLog("Nothing to download", "border-amber-400");
    return;
  }
  const url = URL.createObjectURL(new Blob([text], { type }));
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = filename;
  anchor.click();
  URL.revokeObjectURL(url);
}

function fileStem() {
  return (state.fileName || "art").replace(/\.svg$/i, "") || "art";
}

function escapeHTML(value) {
  return value.replace(/[&<>"']/g, (char) => ({
    "&": "&amp;",
    "<": "&lt;",
    ">": "&gt;",
    '"': "&quot;",
    "'": "&#39;",
  }[char]));
}
