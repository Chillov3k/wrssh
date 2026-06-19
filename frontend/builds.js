import {
  api,
  buildJumpTarget,
  byId,
  escapeAttribute,
  escapeHtml,
  initPage,
  loadSystemOptions,
  markSynced,
  withProjectQuery
} from "./shared.js";

const CUSTOM_ADDRESS_VALUE = "__custom__";
const FRONTEND_GOOS_FALLBACKS = ["linux", "windows", "darwin", "freebsd"];
const FRONTEND_GOARCH_FALLBACKS = ["amd64", "arm64", "386", "mips", "mipsle", "mips64", "mips64le"];

const form = byId("artifactForm");
const artifactBuilderPanel = byId("artifactBuilderPanel");
const connectBackHost = byId("connectBackHost");
const connectBackCustomField = byId("connectBackCustomField");
const connectBackCustomHost = byId("connectBackCustomHost");
const connectBackPort = byId("connectBackPort");
const goosSelect = byId("goosSelect");
const goarchSelect = byId("goarchSelect");
const output = byId("artifactCreateOutput");
const artifactNameInput = form.elements.namedItem("name");

const pageState = {
  systemOptions: null,
  ctx: null
};

form.addEventListener("submit", createArtifact);
form.addEventListener("change", syncCompressionOptions);
connectBackHost.addEventListener("change", handleAddressSelectionChange);
connectBackCustomHost.addEventListener("input", () => renderMeta(pageState.ctx));
connectBackPort.addEventListener("input", () => renderMeta(pageState.ctx));
goosSelect.addEventListener("change", syncArtifactNameRequirement);
if (artifactNameInput instanceof HTMLInputElement) {
  artifactNameInput.addEventListener("input", () => validateArtifactName(false));
}

initPage({
  title: "Builds",
  requireProject: true,
  load: async (ctx) => {
    pageState.ctx = ctx;
    pageState.systemOptions = await loadSystemOptions();
    populateBuildOptions();
    artifactBuilderPanel.classList.remove("hidden");
    renderMeta(ctx);
  }
}).catch((error) => {
  console.error(error);
});

function populateBuildOptions() {
  const options = pageState.systemOptions;
  populateSelect(connectBackHost, buildAddressItems(options), options.defaultInterface);

  populateSelect(goosSelect, mergedGOOSOptions(options).map((value) => ({
    value,
    label: value
  })), goosSelect.value || "linux");

  populateSelect(goarchSelect, mergedGOARCHOptions(options).map((value) => ({
    value,
    label: value
  })), goarchSelect.value || "amd64");

  if (!connectBackPort.value) {
    connectBackPort.value = options.defaultPort || "";
  }

  handleAddressSelectionChange();
  syncCompressionOptions();
  syncArtifactNameRequirement();
  applyBuildFlagHelp(options);
}

function mergedGOOSOptions(options) {
  return uniqueStrings([...(options.goos || []), ...FRONTEND_GOOS_FALLBACKS]);
}

function mergedGOARCHOptions(options) {
  return uniqueStrings([...(options.goarch || []), ...FRONTEND_GOARCH_FALLBACKS]);
}

function uniqueStrings(values) {
  const seen = new Set();
  const result = [];
  for (const rawValue of values) {
    const value = String(rawValue || "").trim();
    if (!value || seen.has(value)) {
      continue;
    }
    seen.add(value);
    result.push(value);
  }
  return result;
}

function buildAddressItems(options) {
  const items = (options.interfaces || []).map((item) => ({
    value: item.address,
    label: item.display
  }));

  items.push({
    value: CUSTOM_ADDRESS_VALUE,
    label: "custom · type another host/IP"
  });

  return items;
}

function populateSelect(select, items, preferredValue) {
  const currentValue = select.value;
  const targetValue = currentValue || preferredValue;
  select.innerHTML = items.map((item) => `<option value="${escapeAttribute(item.value)}">${escapeHtml(item.label)}</option>`).join("");
  if (targetValue && items.some((item) => item.value === targetValue)) {
    select.value = targetValue;
  }
}

function handleAddressSelectionChange() {
  const manual = connectBackHost.value === CUSTOM_ADDRESS_VALUE;
  connectBackCustomField.classList.toggle("hidden", !manual);
  connectBackCustomHost.toggleAttribute("required", manual);
  if (!manual) {
    connectBackCustomHost.value = "";
  }
  renderMeta(pageState.ctx);
}

function syncCompressionOptions() {
  const upxField = form.elements.namedItem("upx");
  const lzmaField = form.elements.namedItem("lzma");
  if (!(upxField instanceof HTMLInputElement) || !(lzmaField instanceof HTMLInputElement)) {
    return;
  }

  lzmaField.disabled = !upxField.checked;
  if (!upxField.checked) {
    lzmaField.checked = false;
  }
}

function applyBuildFlagHelp(options) {
  const help = options?.buildFlagHelp || {};
  document.querySelectorAll("[data-build-help]").forEach((node) => {
    const key = node.getAttribute("data-build-help") || "";
    const text = String(help[key] || "").trim();
    node.classList.toggle("hidden", text === "");
    if (!text) {
      node.removeAttribute("title");
      node.removeAttribute("aria-label");
      return;
    }
    node.setAttribute("title", text);
    node.setAttribute("aria-label", text);
  });
}

async function createArtifact(event) {
  event.preventDefault();

  const formData = new FormData(form);
  const payload = Object.fromEntries(formData.entries());
  ["sharedObject", "garble", "upx", "lzma", "rawDownload", "useHostHeader", "noHistorySave", "busyBoxFallback", "pscan", "execass"].forEach((key) => {
    payload[key] = formData.get(key) === "on";
  });
  payload.buildTags = ["pscan", "execass"].filter((tag) => payload[tag]);
  payload.project = pageState.ctx?.project || "";

  if (!validateArtifactName(true)) {
    return;
  }

  payload.connectBackHost = selectedConnectBackHost();
  if (!payload.connectBackHost) {
    output.textContent = "Choose a reachable host/interface address or type a custom one.";
    return;
  }

  if (payload.lzma && !payload.upx) {
    output.textContent = "LZMA exists in rssh only together with UPX. Enable UPX or disable LZMA.";
    return;
  }
  if (payload.busyBoxFallback && payload.goos !== "linux") {
    output.textContent = "BusyBox fallback is only available for Linux artifacts.";
    return;
  }

  output.textContent = "Building artifact...";
  try {
    const response = await api(withProjectQuery("/api/artifacts", pageState.ctx?.project || ""), {
      method: "POST",
      body: JSON.stringify(payload)
    });
    const lines = [];
    lines.push("Artifact created.");
    if (response.downloadUrl) {
      lines.push(`Download URL: ${response.downloadUrl}`);
    }
    if (response.callbackAddress) {
      lines.push(`Client callback: ${response.callbackAddress}`);
    }
    if (response.project) {
      lines.push(`Project: ${response.project}`);
    }
    output.textContent = lines.join("\n");
    renderMeta(pageState.ctx);
  } catch (error) {
    output.textContent = error.message;
  }
}

function selectedConnectBackHost() {
  if (connectBackHost.value === CUSTOM_ADDRESS_VALUE) {
    return connectBackCustomHost.value.trim();
  }
  return connectBackHost.value.trim();
}

function syncArtifactNameRequirement() {
  if (!(artifactNameInput instanceof HTMLInputElement)) {
    return;
  }

  artifactNameInput.placeholder = isWindowsBuild() ? "agent.exe" : "Name of agent";
  validateArtifactName(false);
}

function validateArtifactName(showValidation) {
  if (!(artifactNameInput instanceof HTMLInputElement)) {
    return true;
  }

  const message = "Windows artifact name must end with .exe, for example love.exe.";
  const invalid = isWindowsBuild() && !artifactNameInput.value.trim().toLowerCase().endsWith(".exe");
  artifactNameInput.setCustomValidity(invalid ? message : "");

  if (!invalid) {
    return true;
  }

  if (showValidation) {
    output.textContent = message;
    artifactNameInput.reportValidity();
  }
  return false;
}

function isWindowsBuild() {
  return goosSelect.value.trim().toLowerCase() === "windows";
}

function renderMeta(ctx) {
  if (!ctx) {
    return;
  }

  markSynced(ctx, `advertised listener ${buildJumpTarget({
    defaultInterface: selectedConnectBackHost() || pageState.systemOptions?.defaultInterface,
    defaultPort: connectBackPort.value || pageState.systemOptions?.defaultPort
  })} · project ${ctx.project}`);
}
