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
  targetLine: document.querySelector("#targetLine"),
  errorCount: document.querySelector("#errorCount"),
  warningCount: document.querySelector("#warningCount"),
  infoCount: document.querySelector("#infoCount"),
  metaGrid: document.querySelector("#metaGrid"),
  issuesList: document.querySelector("#issuesList"),
  activityLog: document.querySelector("#activityLog"),
};

document.addEventListener("DOMContentLoaded", () => {
  wireEvents();
  wireDragDrop();
  setControlsEnabled(false);
  render();
  bootWasm();
  refreshIcons();
});

function wireEvents() {
  el.fileInput.addEventListener("change", loadFile);
  el.sampleSelect.addEventListener("change", loadSample);
  el.targetSelect.addEventListener("change", () => {
    el.customTargetWrap.classList.toggle("hidden", el.targetSelect.value !== "custom");
    analyzeCurrent();
  });
  el.customTarget.addEventListener("change", analyzeCurrent);
  el.customTarget.addEventListener("keydown", (event) => {
    if (event.key === "Enter") analyzeCurrent();
  });
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
  document.addEventListener("keydown", (event) => {
    if ((event.metaKey || event.ctrlKey) && event.key === "Enter") {
      event.preventDefault();
      analyzeCurrent();
    }
  });
}

function wireDragDrop() {
  const pane = el.visualPane;
  let depth = 0;
  pane.addEventListener("dragenter", (event) => {
    event.preventDefault();
    depth++;
    pane.classList.add("is-dragging");
  });
  pane.addEventListener("dragover", (event) => {
    event.preventDefault();
    event.dataTransfer.dropEffect = "copy";
  });
  pane.addEventListener("dragleave", () => {
    depth = Math.max(0, depth - 1);
    if (depth === 0) pane.classList.remove("is-dragging");
  });
  pane.addEventListener("drop", async (event) => {
    event.preventDefault();
    depth = 0;
    pane.classList.remove("is-dragging");
    const file = event.dataTransfer.files?.[0];
    if (!file) return;
    if (!/\.svg$/i.test(file.name) && file.type !== "image/svg+xml") {
      addLog("Drop an .svg file", "error");
      return;
    }
    const text = await file.text();
    setSVG(text, file.name);
    addLog(`Loaded ${file.name}`);
    await analyzeCurrent();
  });
}

async function bootWasm() {
  try {
    setStatus("Loading", "loading");
    el.summaryLine.textContent = "loading runtime…";
    const go = new Go();
    const response = await fetch("../dist/pre-print.wasm");
    const result = await instantiateWasm(response, go.importObject);
    go.run(result.instance);
    await waitFor(() => window.prePrintTools?.ready);
    state.wasmReady = true;
    setControlsEnabled(true);
    setStatus("Ready", "ready");
    el.summaryLine.textContent = "Local SVG preflight";
    addLog("WASM runtime ready", "ok");
  } catch (error) {
    setStatus("Build needed", "error");
    el.summaryLine.textContent = "Run make wasm, then refresh";
    addLog(error.message || String(error), "error");
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
    addLog(`Could not load sample: ${response.status}`, "error");
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
    addLog("No automatic fixes are available", "warn");
    return;
  }
  await applyFixes(categories, el.unsafeToggle.checked);
}

async function applyIssueFix(category, unsafeRequired) {
  if (!category) return;
  if (unsafeRequired && !el.unsafeToggle.checked) {
    addLog("Enable unsafe fixes for that change", "warn");
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
      response.changes.forEach((change) => addLog(change, "ok"));
    } else {
      addLog("No SVG changes were needed", "warn");
    }
    response.skipped?.forEach((skipped) => addLog(skipped, "warn"));
  } catch (error) {
    addLog(error.message || String(error), "error");
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
  refreshIcons();
}

function renderReport() {
  const report = state.report;
  el.reportSummary.textContent = report?.friendlySummary || "Ready";
  el.errorCount.textContent = report?.counts?.errors ?? 0;
  el.warningCount.textContent = report?.counts?.warnings ?? 0;
  el.infoCount.textContent = report?.counts?.info ?? 0;
  el.targetLine.textContent = report?.targetDetails || "";

  const meta = report?.meta || {};
  const raster = (meta.rasterImages ?? 0) + (meta.inlineRasterImages ?? 0);
  const effects = (meta.filters ?? 0) + (meta.filterRefs ?? 0) + (meta.shadows ?? 0);
  const rows = [
    ["Width", meta.width || "-"],
    ["Height", meta.height || "-"],
    ["ViewBox", meta.viewBox || "-"],
    ["Colors", meta.uniqueColors ?? 0],
    ["Raster", raster],
    ["Effects", effects],
    ["Thin", meta.thinStrokes ?? 0],
    ["Gaps", meta.nearDisconnected ?? 0],
  ];
  el.metaGrid.innerHTML = rows.map(([term, value]) => {
    const isZero = value === 0 || value === "0";
    return `
      <div>
        <dt>${escapeHTML(term)}</dt>
        <dd${isZero ? ' data-zero="true"' : ""}>${escapeHTML(String(value))}</dd>
      </div>
    `;
  }).join("");
}

function renderIssues() {
  const issues = state.report?.issues || [];
  if (issues.length === 0) {
    const clean = Boolean(state.report);
    el.issuesList.innerHTML = clean
      ? `<div class="issues-empty is-clean">
          <i data-lucide="check-circle-2" class="h-5 w-5"></i>
          No issues reported
        </div>`
      : `<div class="issues-empty">Analyze an SVG to see issues</div>`;
    refreshIcons();
    return;
  }

  el.issuesList.innerHTML = issues.map((issue, index) => {
    const sev = issue.severity || "info";
    const canApply = issue.automaticFix && (!issue.unsafeRequired || el.unsafeToggle.checked);
    const actionKind = issue.automaticFix
      ? (issue.unsafeRequired && !el.unsafeToggle.checked ? "locked" : "apply")
      : "manual";
    const actionText = issue.automaticFix
      ? issue.unsafeRequired && !el.unsafeToggle.checked
        ? "Needs unsafe"
        : `Apply ${issue.fixCategory}`
      : "Manual";
    const actionIcon = issue.automaticFix ? "wand-sparkles" : "hand";
    return `
      <article class="issue-card" data-sev="${escapeHTML(sev)}" data-expanded="false" tabindex="0">
        <div class="issue-head">
          <div class="issue-main">
            <div class="issue-code-row">
              <span class="severity-mark"></span>
              <span class="issue-code">${escapeHTML(issue.code)}</span>
            </div>
            <div class="issue-tags">
              <span class="issue-tag" data-sev="${escapeHTML(sev)}">${escapeHTML(issue.severity)}</span>
              ${issue.rank ? `<span class="issue-tag is-rank">${escapeHTML(issue.rank)}</span>` : ""}
            </div>
          </div>
          <button
            class="issue-action"
            data-kind="${actionKind}"
            data-issue-fix="${index}"
            ${canApply ? "" : "disabled"}
          >
            <i data-lucide="${actionIcon}" class="h-3.5 w-3.5"></i>
            ${escapeHTML(actionText)}
          </button>
        </div>
        <div class="issue-message">${escapeHTML(issue.message)}</div>
        <button class="issue-toggle" data-issue-toggle="${index}" aria-label="Toggle details"></button>
      </article>
    `;
  }).join("");

  el.issuesList.querySelectorAll("[data-issue-fix]").forEach((button) => {
    button.addEventListener("click", (event) => {
      event.stopPropagation();
      const issue = issues[Number(button.dataset.issueFix)];
      applyIssueFix(issue.fixCategory, issue.unsafeRequired);
    });
  });
  el.issuesList.querySelectorAll("[data-issue-toggle]").forEach((button) => {
    button.addEventListener("click", (event) => {
      event.stopPropagation();
      const card = button.closest(".issue-card");
      const expanded = card.dataset.expanded === "true";
      card.dataset.expanded = expanded ? "false" : "true";
    });
  });
  el.issuesList.querySelectorAll(".issue-card").forEach((card) => {
    card.addEventListener("keydown", (event) => {
      if (event.key === "Enter" || event.key === " ") {
        if (event.target === card) {
          event.preventDefault();
          const expanded = card.dataset.expanded === "true";
          card.dataset.expanded = expanded ? "false" : "true";
        }
      }
    });
  });
  refreshIcons();
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

function setStatus(text, state) {
  el.runtimeStatus.dataset.state = state;
  let dotColor = "";
  if (state === "ready") dotColor = "ok";
  else if (state === "error") dotColor = "error";
  else if (state === "loading") dotColor = "info";
  el.runtimeStatus.innerHTML = `<span class="status-dot" data-level="${dotColor}"></span>${escapeHTML(text)}`;
}

function addLog(message, level = "info") {
  const item = document.createElement("div");
  item.className = "activity-item";
  item.dataset.level = level;
  item.textContent = message;
  el.activityLog.prepend(item);
}

function downloadText(text, filename, type) {
  if (!text?.trim()) {
    addLog("Nothing to download", "warn");
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

function refreshIcons() {
  if (window.lucide) {
    window.lucide.createIcons();
  }
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
