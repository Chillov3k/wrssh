import {
  api,
  appPath,
  buildJumpTarget,
  copyFromButton,
  escapeAttribute,
  escapeHtml,
  formatDate,
  hostCommandTemplates,
  hostPageHref,
  initPage,
  loadHost,
  loadProjects,
  loadSystemOptions,
  makeClientRow,
  markSynced,
  parsePlatform,
  resolveConnection,
  websocketURL,
  viewHref,
  withProjectQuery
} from "./shared.js";
import { TerminalView } from "./terminal.js";

const search = new URLSearchParams(window.location.search);
const stableId = search.get("stableId") || "";

const elements = {
  hostDetailEmpty: document.getElementById("hostDetailEmpty"),
  hostDetailContent: document.getElementById("hostDetailContent"),
  detailHostname: document.getElementById("detailHostname"),
  detailHostnameMeta: document.getElementById("detailHostnameMeta"),
  detailSubline: document.getElementById("detailSubline"),
  detailStatusBadge: document.getElementById("detailStatusBadge"),
  detailIp: document.getElementById("detailIp"),
  detailPlatform: document.getElementById("detailPlatform"),
  detailAdded: document.getElementById("detailAdded"),
  detailTagsValue: document.getElementById("detailTagsValue"),
  commandTemplates: document.getElementById("commandTemplates"),
  openTerminalButton: document.getElementById("openTerminalButton"),
  terminalTitle: document.getElementById("terminalTitle"),
  terminalViewport: document.getElementById("terminalViewport"),
  terminalOutput: document.getElementById("terminalOutput"),
  sendCtrlCButton: document.getElementById("sendCtrlCButton"),
  closeTerminalButton: document.getElementById("closeTerminalButton"),
  backToHostsLink: document.getElementById("backToHostsLink"),
  refreshModulesButton: document.getElementById("refreshModulesButton"),
  moduleList: document.getElementById("moduleList"),
  moduleRunForm: document.getElementById("moduleRunForm"),
  moduleSelect: document.getElementById("moduleSelect"),
  moduleHelpText: document.getElementById("moduleHelpText"),
  moduleArgsInput: document.getElementById("moduleArgsInput"),
  moduleTimeoutInput: document.getElementById("moduleTimeoutInput"),
  moduleOutputLimitInput: document.getElementById("moduleOutputLimitInput"),
  moduleStdinInput: document.getElementById("moduleStdinInput"),
  moduleStdinFileInput: document.getElementById("moduleStdinFileInput"),
  runModuleButton: document.getElementById("runModuleButton"),
  moduleOutput: document.getElementById("moduleOutput"),
  hostMetadataPanel: document.getElementById("hostMetadataPanel"),
  hostDisplayNameInput: document.getElementById("hostDisplayNameInput"),
  hostTagsInput: document.getElementById("hostTagsInput"),
  hostProjectSelect: document.getElementById("hostProjectSelect"),
  saveHostMetadataButton: document.getElementById("saveHostMetadataButton"),
  hostMetadataOutput: document.getElementById("hostMetadataOutput"),
  openFilesystemButton: document.getElementById("openFilesystemButton"),
  fileSystemOverlay: document.getElementById("fileSystemOverlay"),
  closeFilesystemButton: document.getElementById("closeFilesystemButton"),
  filesystemTitle: document.getElementById("filesystemTitle"),
  filesystemMeta: document.getElementById("filesystemMeta"),
  filesystemCurrentPath: document.getElementById("filesystemCurrentPath"),
  filesystemPathForm: document.getElementById("filesystemPathForm"),
  filesystemPathInput: document.getElementById("filesystemPathInput"),
  refreshFilesystemButton: document.getElementById("refreshFilesystemButton"),
  uploadFilesystemButton: document.getElementById("uploadFilesystemButton"),
  filesystemMessage: document.getElementById("filesystemMessage"),
  filesystemTree: document.getElementById("filesystemTree"),
  filesystemPreviewPanel: document.getElementById("filesystemPreviewPanel"),
  filesystemPreviewPath: document.getElementById("filesystemPreviewPath"),
  filesystemPreviewContent: document.getElementById("filesystemPreviewContent"),
  closeFilesystemPreviewButton: document.getElementById("closeFilesystemPreviewButton"),
  filesystemUploadInput: document.getElementById("filesystemUploadInput")
};

const terminalView = new TerminalView(elements.terminalViewport, elements.terminalOutput);
const MAX_FILE_TRANSFER_BYTES = 500 * 1024 * 1024;
const MAX_FILE_TRANSFER_LABEL = "500 MiB";
const MAX_MODULE_STDIN_BYTES = 8 * 1024 * 1024;
const MAX_MODULE_STDIN_LABEL = "8 MiB";
const HIDDEN_WEB_MODULES = new Set(["list", "sftp"]);
const MODULE_FORM_HELP = {
  pscan: {
    argsPlaceholder: "-h localhost -p 80,443 --json or -h host -p 53,161 --udp",
    stdinPlaceholder: "not used by pscan",
    text: "Example: pscan -h 10.0.0.0/24 -p 80,443 --json; UDP: pscan -h 10.0.0.5 -p 53,161 --udp --json"
  },
  execass: {
    argsPlaceholder: "--args \"currentluid\" --debug",
    stdinPlaceholder: "upload or paste a .NET assembly artifact",
    text: "Upload the assembly through Stdin file, then pass assembly arguments with --args."
  },
  service: {
    argsPlaceholder: "--install or --uninstall",
    stdinPlaceholder: "not used by service",
    text: "Installs or removes the client OS service. Requires elevated privileges."
  },
  setuid: {
    argsPlaceholder: "0",
    stdinPlaceholder: "not used by setuid",
    text: "Changes the Linux client process UID."
  },
  setgid: {
    argsPlaceholder: "0",
    stdinPlaceholder: "not used by setgid",
    text: "Changes the Linux client process GID."
  }
};
const filesystemAvailable = Boolean(
  elements.fileSystemOverlay &&
  elements.closeFilesystemButton &&
  elements.filesystemTitle &&
  elements.filesystemMeta &&
  elements.filesystemCurrentPath &&
  elements.filesystemPathForm &&
  elements.filesystemPathInput &&
  elements.refreshFilesystemButton &&
  elements.uploadFilesystemButton &&
  elements.filesystemMessage &&
  elements.filesystemTree &&
  elements.filesystemPreviewPanel &&
  elements.filesystemPreviewPath &&
  elements.filesystemPreviewContent &&
  elements.closeFilesystemPreviewButton &&
  elements.filesystemUploadInput
);

const pageState = {
  host: null,
  ctx: null,
  projects: [],
  systemOptions: null,
  selectedConnectionId: search.get("connectionId") || "",
  autoConnected: false,
  terminal: {
    socket: null,
    stableId: null,
    connectionId: null,
    cols: 120,
    rows: 36,
    resizeTimer: 0
  },
  modules: {
    items: [],
    loading: false,
    running: false,
    error: "",
    loadedConnectionId: ""
  },
  filesystem: {
    open: false,
    selectedDirectory: "/",
    uploadDirectory: "/",
    nodes: new Map(),
    preview: null
  }
};

elements.openTerminalButton.addEventListener("click", () => {
  const connection = currentConnection();
  if (connection) {
    openTerminal(connection.connectionId, true);
  }
});
elements.closeTerminalButton.addEventListener("click", () => {
  closeTerminal();
  terminalView.write("\n[terminal disconnected]\n");
  renderHost();
});
elements.sendCtrlCButton.addEventListener("click", () => sendTerminalInput("\u0003"));
elements.saveHostMetadataButton.addEventListener("click", saveHostMetadata);
elements.refreshModulesButton?.addEventListener("click", () => loadModules(true));
elements.moduleRunForm?.addEventListener("submit", runSelectedModule);
elements.moduleSelect?.addEventListener("change", renderModules);
elements.moduleList?.addEventListener("click", (event) => {
  const button = event.target.closest("[data-module-select]");
  if (!button || !elements.moduleSelect) {
    return;
  }
  elements.moduleSelect.value = button.dataset.moduleSelect || "";
  renderModules();
});
if (filesystemAvailable) {
  elements.openFilesystemButton?.addEventListener("click", openFilesystem);
  elements.closeFilesystemButton.addEventListener("click", closeFilesystem);
  elements.filesystemPathForm.addEventListener("submit", navigateFilesystemPath);
  elements.refreshFilesystemButton.addEventListener("click", refreshSelectedFilesystemDirectory);
  elements.uploadFilesystemButton.addEventListener("click", () => triggerFilesystemUpload(pageState.filesystem.selectedDirectory));
  elements.filesystemTree.addEventListener("click", handleFilesystemClick);
  elements.closeFilesystemPreviewButton.addEventListener("click", clearFilesystemPreview);
  elements.filesystemUploadInput.addEventListener("change", uploadSelectedFilesystemFile);
  elements.fileSystemOverlay.addEventListener("click", (event) => {
    if (event.target === elements.fileSystemOverlay) {
      closeFilesystem();
    }
  });
}
elements.terminalViewport.addEventListener("keydown", handleTerminalKeydown);
elements.terminalViewport.addEventListener("paste", handleTerminalPaste);
elements.commandTemplates.addEventListener("click", async (event) => {
  const button = event.target.closest("[data-copy-command]");
  if (!button) {
    return;
  }

  await copyFromButton(button, button.dataset.copyCommand || "");
});
window.addEventListener("resize", queueTerminalResize);
window.addEventListener("beforeunload", () => closeTerminal());

initPage({
  title: "Client Shell",
  requireProject: true,
  load: async (ctx) => {
    pageState.ctx = ctx;
    elements.backToHostsLink.href = viewHref("hosts", ctx.project);
    if (!stableId) {
      renderEmpty("No client was selected. Go back to Hosts and choose a client row.");
      markSynced(ctx, "no client selected");
      return;
    }

    const [host, systemOptions, projects] = await Promise.all([loadHost(stableId, ctx.project), loadSystemOptions(), loadProjects()]);
    pageState.host = host;
    pageState.projects = projects;
    pageState.systemOptions = systemOptions;

    if (!currentConnection() && host.activeConnections?.length) {
      pageState.selectedConnectionId = host.activeConnections[0].connectionId;
      syncQueryString();
    }

    renderHost();
    await loadModules(false);

    const connection = currentConnection();
    if (connection) {
      openTerminal(connection.connectionId, false);
      pageState.autoConnected = true;
    } else {
      if (pageState.terminal.socket) {
        closeTerminal();
      }
      terminalView.setConnected(false);
      terminalView.reset("[host is offline]\n");
      pageState.autoConnected = true;
    }

    markSynced(ctx, `${ctx.project} · ${host.hostname || host.preferredAlias || host.stableId}`);
  }
}).catch((error) => {
  console.error(error);
});

function renderEmpty(message) {
  closeTerminal();
  terminalView.setConnected(false);
  terminalView.reset("");
  elements.hostDetailEmpty.classList.remove("hidden");
  elements.hostDetailContent.classList.add("hidden");
  elements.hostDetailEmpty.innerHTML = `<p>${escapeHtml(message)}</p>`;
}

function renderHost() {
  const host = pageState.host;
  if (!host) {
    renderEmpty("Host not found.");
    return;
  }

  const connection = currentConnection();
  const row = currentRow(connection);
  const commands = currentCommands(connection);
  const displayName = row.hostname || host.preferredAlias || host.stableId || "-";
  const currentTarget = row.connectionId || "offline";

  elements.hostDetailEmpty.classList.add("hidden");
  elements.hostDetailContent.classList.remove("hidden");

  elements.detailHostname.textContent = displayName;
  elements.detailHostnameMeta.textContent = displayName;
  elements.detailSubline.textContent = `${row.project || "Unassigned"} · ${row.comment || "no comment"} · ${row.version || "unknown version"}`;
  elements.detailStatusBadge.textContent = connection ? "Online" : "Offline";
  elements.detailStatusBadge.className = `status-pill ${connection ? "online" : "offline"}`;
  elements.detailIp.textContent = row.ip || row.remoteAddr || "-";
  elements.detailPlatform.textContent = row.platform || "-";
  elements.detailAdded.textContent = formatDate(row.dateAdded || row.lastActivityAt);
  elements.detailTagsValue.textContent = row.tags?.join(", ") || "-";
  renderHostMetadataForm();

  elements.openTerminalButton.disabled = !connection;
  if (elements.openFilesystemButton) {
    elements.openFilesystemButton.disabled = !connection;
    elements.openFilesystemButton.classList.toggle("hidden", !filesystemAvailable);
  }
  elements.sendCtrlCButton.disabled = !pageState.terminal.socket;
  elements.closeTerminalButton.disabled = !pageState.terminal.socket;
  elements.openTerminalButton.textContent = pageState.terminal.connectionId === connection?.connectionId ? "Reconnect Terminal" : "Open Terminal";
  elements.terminalTitle.textContent = `${displayName} · ${currentTarget}`;

  elements.commandTemplates.innerHTML = [
    ["Full shell", commands.ssh],
    ["SCP", commands.scp],
    ["Dynamic SOCKS", commands.dynamicSocks],
    ["Remote forward", commands.remoteForward]
  ].map(([label, command]) => `
    <article class="command-card">
      <strong>${escapeHtml(label)}</strong>
      <code>${escapeHtml(command)}</code>
      <div class="inline-actions">
        <button class="ghost-button" data-copy-command="${escapeAttribute(command)}">Copy</button>
      </div>
    </article>
  `).join("");

  renderModules();
}

function currentConnection() {
  return resolveConnection(pageState.host, pageState.selectedConnectionId);
}

function currentRow(connection = currentConnection()) {
  return makeClientRow(pageState.host, connection);
}

function currentCommands(connection = currentConnection()) {
  return hostCommandTemplates(pageState.host, connection, buildJumpTarget(pageState.systemOptions, pageState.host, pageState.ctx?.user));
}

async function loadModules(force) {
  if (!elements.moduleList || !pageState.host) {
    return;
  }

  const connection = currentConnection();
  if (!connection) {
    pageState.modules.items = [];
    pageState.modules.error = "";
    pageState.modules.loadedConnectionId = "";
    renderModules();
    return;
  }
  if (!force && pageState.modules.loadedConnectionId === connection.connectionId) {
    renderModules();
    return;
  }

  pageState.modules.loading = true;
  pageState.modules.error = "";
  renderModules();
  try {
    const response = await api(moduleAPIPath("", { connectionId: connection.connectionId }));
    pageState.modules.items = Array.isArray(response.items) ? response.items : [];
    pageState.modules.loadedConnectionId = connection.connectionId;
  } catch (error) {
    pageState.modules.items = [];
    pageState.modules.error = error.message;
    pageState.modules.loadedConnectionId = "";
  } finally {
    pageState.modules.loading = false;
    renderModules();
  }
}

function renderModules() {
  if (!elements.moduleList || !elements.moduleSelect || !elements.runModuleButton) {
    return;
  }

  const connection = currentConnection();
  const row = currentRow(connection);
  const modules = visibleWebModules(pageState.modules.items).map((module) => moduleForCurrentHost(module, row));
  const selectedName = elements.moduleSelect.value;
  const selected = modules.find((module) => module.name === selectedName) || modules.find((module) => !module.disabled) || modules[0] || null;
  syncModuleFormHelp(selected);

  elements.refreshModulesButton.disabled = !connection || pageState.modules.loading;
  elements.moduleRunForm?.classList.toggle("hidden", !connection);
  elements.moduleSelect.disabled = !connection || modules.length === 0 || pageState.modules.running;
  elements.runModuleButton.disabled = !connection || !selected || selected.disabled || pageState.modules.running;

  if (!connection) {
    elements.moduleList.innerHTML = `<div class="empty-state module-empty"><p>Host is offline.</p></div>`;
    elements.moduleSelect.innerHTML = "";
    return;
  }

  if (pageState.modules.loading) {
    elements.moduleList.innerHTML = `<div class="empty-state module-empty"><p>Loading modules...</p></div>`;
    return;
  }

  if (pageState.modules.error) {
    elements.moduleList.innerHTML = `<div class="empty-state module-empty"><p>${escapeHtml(pageState.modules.error)}</p></div>`;
    elements.moduleSelect.innerHTML = "";
    return;
  }

  elements.moduleSelect.innerHTML = modules.map((module) => (
    `<option value="${escapeAttribute(module.name || "")}" ${module.disabled ? "disabled" : ""}>${escapeHtml(module.name || "unnamed")}</option>`
  )).join("");
  if (selected) {
    elements.moduleSelect.value = selected.name;
  }

  if (!modules.length) {
    elements.moduleList.innerHTML = `<div class="empty-state module-empty"><p>No runnable web modules reported by this agent.</p></div>`;
    return;
  }

  elements.moduleList.innerHTML = modules.map((module) => renderModuleCard(module, selected?.name === module.name)).join("");
}

function renderModuleCard(module, active) {
  const tags = [];
  if (module.dangerous) {
    tags.push("dangerous");
  }
  if (module.disabled) {
    tags.push("disabled");
  }
  (module.buildTags || []).forEach((tag) => tags.push(`tag:${tag}`));
  (module.platforms || []).forEach((platform) => tags.push(platform));

  const limits = module.limits || {};
  const limitParts = [];
  if (limits.timeoutSeconds) {
    limitParts.push(`${limits.timeoutSeconds}s timeout`);
  }
  if (limits.outputBytes) {
    limitParts.push(`${limits.outputBytes}B output`);
  }
  if (limits.stdinBytes) {
    limitParts.push(`${limits.stdinBytes}B stdin`);
  }

  return `
    <article class="module-card ${active ? "active" : ""} ${module.dangerous ? "dangerous" : ""} ${module.disabled ? "disabled" : ""}">
      <div class="module-card-head">
        <div>
          <strong>${escapeHtml(module.name || "unnamed")}</strong>
          <span>${escapeHtml(module.version ? `v${module.version}` : "")}</span>
        </div>
        <button class="ghost-button fs-small-button" type="button" data-module-select="${escapeAttribute(module.name || "")}" ${module.disabled ? "disabled" : ""}>Select</button>
      </div>
      <p>${escapeHtml(module.description || "No description.")}</p>
      ${module.disabledReason ? `<small class="error-text">${escapeHtml(module.disabledReason)}</small>` : ""}
      ${tags.length ? `<div class="tag-list module-tags">${tags.map((tag) => `<span>${escapeHtml(tag)}</span>`).join("")}</div>` : ""}
      ${module.usage ? `<code>${escapeHtml(module.usage)}</code>` : ""}
      ${limitParts.length ? `<small>${escapeHtml(limitParts.join(" · "))}</small>` : ""}
    </article>
  `;
}

function visibleWebModules(modules) {
  return (modules || []).filter((module) => !HIDDEN_WEB_MODULES.has(String(module?.name || "").toLowerCase()));
}

function moduleForCurrentHost(module, row) {
  const platforms = (module.platforms || []).map((platform) => String(platform || "").toLowerCase()).filter(Boolean);
  if (!platforms.length) {
    return module;
  }

  const hostOS = parsePlatform(row?.version || "").os;
  if (!hostOS || platforms.includes(hostOS)) {
    return module;
  }

  return {
    ...module,
    disabled: true,
    disabledReason: `Requires ${platforms.join(", ")} target; selected host is ${hostOS}.`
  };
}

function syncModuleFormHelp(module) {
  const name = String(module?.name || "").toLowerCase();
  const help = MODULE_FORM_HELP[name] || {};
  if (elements.moduleHelpText) {
    elements.moduleHelpText.textContent = module ? (help.text || module.usage || "") : "";
  }
  if (elements.moduleArgsInput) {
    elements.moduleArgsInput.placeholder = help.argsPlaceholder || "module arguments";
  }
  if (elements.moduleStdinInput) {
    elements.moduleStdinInput.placeholder = help.stdinPlaceholder || "optional stdin";
  }
}

async function runSelectedModule(event) {
  event.preventDefault();
  if (!pageState.host || !elements.moduleSelect || !elements.moduleOutput) {
    return;
  }

  const connection = currentConnection();
  const module = elements.moduleSelect.value.trim();
  if (!connection || !module) {
    elements.moduleOutput.textContent = "Choose an online connection and module.";
    return;
  }
  if (HIDDEN_WEB_MODULES.has(module.toLowerCase())) {
    elements.moduleOutput.textContent = "This transport helper module is hidden from the web runner.";
    return;
  }

  let args = [];
  try {
    args = parseModuleArgs(elements.moduleArgsInput?.value || "");
  } catch (error) {
    elements.moduleOutput.textContent = error.message;
    return;
  }

  let stdin = elements.moduleStdinInput?.value || "";
  let stdinBase64 = "";
  const stdinFile = elements.moduleStdinFileInput?.files?.[0] || null;
  if (stdinFile && stdin) {
    elements.moduleOutput.textContent = "Use either stdin text or stdin file, not both.";
    return;
  }
  if (stdinFile) {
    if (stdinFile.size > moduleStdinLimitBytes(module)) {
      elements.moduleOutput.textContent = `Stdin file is too large. Maximum is ${moduleStdinLimitLabel(module)}.`;
      return;
    }
    try {
      stdinBase64 = await readFileAsBase64(stdinFile);
      stdin = "";
    } catch (error) {
      elements.moduleOutput.textContent = error.message;
      return;
    }
  } else if (stdin && textByteLength(stdin) > moduleStdinLimitBytes(module)) {
    elements.moduleOutput.textContent = `Stdin is too large. Maximum is ${moduleStdinLimitLabel(module)}.`;
    return;
  }

  const payload = {
    connectionId: connection.connectionId,
    args,
    stdin,
    stdinBase64,
    timeoutSeconds: numberFieldValue(elements.moduleTimeoutInput, 60),
    outputLimitBytes: numberFieldValue(elements.moduleOutputLimitInput, 1024 * 1024)
  };

  pageState.modules.running = true;
  elements.moduleOutput.textContent = `Running ${module}...`;
  renderModules();
  try {
    const response = await api(moduleAPIPath(`/${encodeURIComponent(module)}/run`), {
      method: "POST",
      body: JSON.stringify(payload)
    });
    const lines = [];
    if (response.output) {
      lines.push(response.output);
    }
    if (response.timedOut) {
      lines.push("[timed out]");
    }
    if (response.truncated) {
      lines.push("[output truncated]");
    }
    if (response.error) {
      lines.push(`[error] ${response.error}`);
    }
    elements.moduleOutput.textContent = lines.join(lines.length > 1 ? "\n" : "") || "Module completed with no output.";
  } catch (error) {
    elements.moduleOutput.textContent = error.message;
  } finally {
    pageState.modules.running = false;
    renderModules();
  }
}

function moduleStdinLimitBytes(moduleName) {
  const module = (pageState.modules.items || []).find((item) => item.name === moduleName);
  const limit = Number(module?.limits?.stdinBytes || 0);
  if (limit > 0) {
    return Math.min(limit, MAX_MODULE_STDIN_BYTES);
  }
  return MAX_MODULE_STDIN_BYTES;
}

function moduleStdinLimitLabel(moduleName) {
  const limit = moduleStdinLimitBytes(moduleName);
  if (limit === MAX_MODULE_STDIN_BYTES) {
    return MAX_MODULE_STDIN_LABEL;
  }
  return `${limit} bytes`;
}

function textByteLength(value) {
  return new TextEncoder().encode(String(value || "")).length;
}

async function readFileAsBase64(file) {
  const bytes = new Uint8Array(await file.arrayBuffer());
  let binary = "";
  const chunkSize = 0x8000;
  for (let offset = 0; offset < bytes.length; offset += chunkSize) {
    binary += String.fromCharCode(...bytes.subarray(offset, offset + chunkSize));
  }
  return btoa(binary);
}

function parseModuleArgs(raw) {
  const args = [];
  let current = "";
  let quote = "";
  let escaping = false;

  for (const char of String(raw || "")) {
    if (escaping) {
      current += char;
      escaping = false;
      continue;
    }
    if (char === "\\") {
      escaping = true;
      continue;
    }
    if (quote) {
      if (char === quote) {
        quote = "";
      } else {
        current += char;
      }
      continue;
    }
    if (char === "\"" || char === "'") {
      quote = char;
      continue;
    }
    if (/\s/.test(char)) {
      if (current !== "") {
        args.push(current);
        current = "";
      }
      continue;
    }
    current += char;
  }

  if (escaping) {
    current += "\\";
  }
  if (quote) {
    throw new Error("Unclosed quote in module args.");
  }
  if (current !== "") {
    args.push(current);
  }
  return args;
}

function numberFieldValue(field, fallback) {
  const value = Number(field?.value || fallback);
  return Number.isFinite(value) && value > 0 ? value : fallback;
}

function moduleAPIPath(suffix = "", params = {}) {
  const base = withProjectQuery(`/api/hosts/${encodeURIComponent(pageState.host.stableId)}/modules${suffix}`, pageState.ctx?.project || "");
  const url = new URL(base, window.location.origin);
  Object.entries(params).forEach(([key, value]) => {
    if (value === undefined || value === null || value === "") {
      return;
    }
    url.searchParams.set(key, String(value));
  });
  return `${url.pathname}${url.search}`;
}

function openFilesystem() {
  const connection = currentConnection();
  if (!pageState.host || !connection) {
    return;
  }

  pageState.filesystem.open = true;
  pageState.filesystem.selectedDirectory = "/";
  pageState.filesystem.uploadDirectory = "/";
  pageState.filesystem.preview = null;
  const row = currentRow(connection);
  const root = filesystemRootNode(row);
  pageState.filesystem.nodes = new Map([[root.path, root]]);

  elements.filesystemTitle.textContent = `${row.hostname || row.stableId} File System`;
  elements.filesystemMeta.textContent = `Connection ${row.connectionId || "-"} · ${row.remoteAddr || row.ip || "-"} · max upload/download ${MAX_FILE_TRANSFER_LABEL}`;
  setFilesystemMessage("", "");
  elements.fileSystemOverlay.classList.add("visible");
  elements.fileSystemOverlay.setAttribute("aria-hidden", "false");
  renderFilesystem();
  if (root.virtual) {
    void loadFilesystemDirectory(root.path);
  }
}

function closeFilesystem() {
  pageState.filesystem.open = false;
  elements.fileSystemOverlay.classList.remove("visible");
  elements.fileSystemOverlay.setAttribute("aria-hidden", "true");
  elements.filesystemUploadInput.value = "";
}

function handleFilesystemClick(event) {
  const toggle = event.target.closest("[data-fs-toggle]");
  if (toggle) {
    toggleFilesystemDirectory(toggle.dataset.fsToggle || "/");
    return;
  }

  const select = event.target.closest("[data-fs-select]");
  if (select) {
    toggleFilesystemDirectory(select.dataset.fsSelect || "/");
    return;
  }

  const download = event.target.closest("[data-fs-download]");
  if (download) {
    downloadFilesystemFile(download.dataset.fsDownload || "");
    return;
  }

  const preview = event.target.closest("[data-fs-preview]");
  if (preview) {
    previewFilesystemFile(preview.dataset.fsPreview || "");
    return;
  }

  const upload = event.target.closest("[data-fs-upload]");
  if (upload) {
    triggerFilesystemUpload(upload.dataset.fsUpload || "/");
  }
}

async function toggleFilesystemDirectory(remotePath) {
  const node = filesystemNode(remotePath);
  if (!node || node.type !== "directory") {
    return;
  }

  selectFilesystemDirectory(node.path);
  node.expanded = !node.expanded;
  if (node.expanded && !node.loaded && !node.loading) {
    setFilesystemMessage("", "");
    clearFilesystemPreview();
    await loadFilesystemDirectory(node.path);
    return;
  }
  renderFilesystem();
}

function selectFilesystemDirectory(remotePath) {
  const node = filesystemNode(remotePath);
  if (!node || node.type !== "directory") {
    return;
  }
  pageState.filesystem.selectedDirectory = node.path;
  renderFilesystem();
}

async function navigateFilesystemPath(event) {
  event.preventDefault();

  const path = normalizeFilesystemPath(elements.filesystemPathInput.value);
  if (!path) {
    return;
  }

  let node = filesystemNode(path);
  if (!node) {
    node = ensureFilesystemDirectoryNode(path);
  }

  pageState.filesystem.selectedDirectory = node.path;
  node.expanded = true;
  node.loaded = false;
  setFilesystemMessage("", "");
  clearFilesystemPreview();
  const lineage = filesystemPathLineage(node.path);
  for (let index = 0; index < lineage.length; index++) {
    const directory = lineage[index];
    const directoryNode = ensureFilesystemDirectoryNode(directory);
    directoryNode.expanded = true;
    if (!directoryNode.loaded || directoryNode.path === node.path) {
      await loadFilesystemDirectory(directoryNode.path);
    } else {
      renderFilesystem();
    }
    const currentNode = filesystemNode(directory);
    const nextDirectory = lineage[index + 1];
    if (currentNode && nextDirectory && !currentNode.error) {
      linkFilesystemChild(currentNode, nextDirectory);
    }
  }
}

async function refreshSelectedFilesystemDirectory() {
  const selected = pageState.filesystem.selectedDirectory || "/";
  const node = filesystemNode(selected);
  if (!node) {
    return;
  }
  node.loaded = false;
  node.expanded = true;
  setFilesystemMessage("", "");
  await loadFilesystemDirectory(selected);
}

async function loadFilesystemDirectory(remotePath) {
  const node = filesystemNode(remotePath);
  if (!node) {
    return;
  }

  node.loading = true;
  node.error = "";
  renderFilesystem();

  try {
    const response = await api(filesystemAPIPath("", { path: node.path }));
    node.items = Array.isArray(response.items) ? response.items : [];
    node.loaded = true;
    node.expanded = true;
    node.error = "";
    node.items.forEach((item) => {
      if (item.type !== "directory") {
        return;
      }
      const existing = pageState.filesystem.nodes.get(item.path);
      pageState.filesystem.nodes.set(item.path, {
        ...(existing || {}),
        ...item,
        expanded: existing?.expanded || false,
        loaded: existing?.loaded || false,
        loading: false,
        error: existing?.error || "",
        items: existing?.items || []
      });
    });
  } catch (error) {
    const message = filesystemErrorMessage(error.message);
    node.error = message;
    setFilesystemMessage(message, "error");
  } finally {
    node.loading = false;
    renderFilesystem();
  }
}

async function downloadFilesystemFile(remotePath) {
  if (!remotePath) {
    return;
  }

  setFilesystemMessage(`Downloading ${remotePath}...`, "muted");
  try {
    const response = await fetch(appPath(filesystemAPIPath("/download", { path: remotePath })), {
      method: "GET",
      credentials: "include",
      cache: "no-store"
    });
    if (!response.ok) {
      const payload = await readJSONResponse(response);
      throw new Error(payload.error || `Download failed with ${response.status}`);
    }
    const length = Number(response.headers.get("Content-Length") || "0");
    if (Number.isFinite(length) && length > MAX_FILE_TRANSFER_BYTES) {
      throw new Error(maxFileTransferMessage("download"));
    }

    const blob = await response.blob();
    const anchor = document.createElement("a");
    const objectURL = URL.createObjectURL(blob);
    anchor.href = objectURL;
    anchor.download = filenameFromDisposition(response.headers.get("Content-Disposition")) || remotePath.split("/").filter(Boolean).pop() || "download";
    anchor.rel = "noopener";
    document.body.appendChild(anchor);
    anchor.click();
    document.body.removeChild(anchor);
    URL.revokeObjectURL(objectURL);
    setFilesystemMessage(`Downloaded ${remotePath}.`, "ok");
  } catch (error) {
    setFilesystemMessage(filesystemErrorMessage(error.message), "error");
  }
}

async function previewFilesystemFile(remotePath) {
  if (!remotePath) {
    return;
  }

  setFilesystemMessage(`Loading preview for ${remotePath}...`, "muted");
  try {
    const preview = await api(filesystemAPIPath("/preview", { path: remotePath }));
    pageState.filesystem.preview = preview;
    setFilesystemMessage(preview.truncated ? "Showing first 20 lines." : "Preview loaded.", "ok");
    renderFilesystemPreview();
  } catch (error) {
    clearFilesystemPreview();
    setFilesystemMessage(filesystemErrorMessage(error.message), "error");
  }
}

function triggerFilesystemUpload(directory) {
  const node = filesystemNode(directory);
  if (!node || node.type !== "directory" || node.virtual) {
    return;
  }
  pageState.filesystem.uploadDirectory = node.path;
  pageState.filesystem.selectedDirectory = node.path;
  setFilesystemMessage("", "");
  renderFilesystem();
  elements.filesystemUploadInput.click();
}

async function uploadSelectedFilesystemFile() {
  const file = elements.filesystemUploadInput.files?.[0];
  if (!file) {
    return;
  }
  if (file.size > MAX_FILE_TRANSFER_BYTES) {
    elements.filesystemUploadInput.value = "";
    setFilesystemMessage(maxFileTransferMessage("upload"), "error");
    return;
  }

  const directory = pageState.filesystem.uploadDirectory || pageState.filesystem.selectedDirectory || "/";
  const formData = new FormData();
  formData.append("file", file);

  elements.uploadFilesystemButton.disabled = true;
  setFilesystemMessage(`Uploading ${file.name} to ${directory}...`, "muted");

  try {
    const response = await fetch(appPath(filesystemAPIPath("/upload", { directory })), {
      method: "POST",
      credentials: "include",
      cache: "no-store",
      body: formData
    });
    const payload = await readJSONResponse(response);
    if (!response.ok) {
      throw new Error(payload.error || `Upload failed with ${response.status}`);
    }

    setFilesystemMessage(`Uploaded ${payload.name || file.name} to ${payload.path || directory}.`, "ok");
    const node = filesystemNode(directory);
    if (node) {
      node.loaded = false;
      node.expanded = true;
      await loadFilesystemDirectory(directory);
    }
  } catch (error) {
    setFilesystemMessage(filesystemErrorMessage(error.message), "error");
  } finally {
    elements.uploadFilesystemButton.disabled = false;
    elements.filesystemUploadInput.value = "";
    renderFilesystem();
  }
}

async function readJSONResponse(response) {
  const text = await response.text();
  if (!text) {
    return {};
  }
  try {
    return JSON.parse(text);
  } catch (error) {
    return { error: text };
  }
}

function filesystemNode(remotePath) {
  const path = normalizeFilesystemPath(remotePath);
  return pageState.filesystem.nodes.get(path);
}

function ensureFilesystemDirectoryNode(remotePath) {
  const path = normalizeFilesystemPath(remotePath);
  const existing = pageState.filesystem.nodes.get(path);
  if (existing) {
    return existing;
  }

  const node = {
    path,
    name: filesystemPathName(path),
    type: "directory",
    expanded: true,
    loaded: false,
    loading: false,
    error: "",
    items: []
  };
  pageState.filesystem.nodes.set(path, node);

  const parent = filesystemParentPath(path);
  if (parent && parent !== path) {
    const parentNode = ensureFilesystemDirectoryNode(parent);
    parentNode.expanded = true;
    linkFilesystemChild(parentNode, path);
  }

  return node;
}

function linkFilesystemChild(parentNode, childPath) {
  const path = normalizeFilesystemPath(childPath);
  if (parentNode.items.some((item) => normalizeFilesystemPath(item.path) === path)) {
    return;
  }
  const childNode = filesystemNode(path);
  parentNode.items.push({
    path,
    name: childNode?.name || filesystemPathName(path),
    type: "directory"
  });
}

function normalizeFilesystemPath(remotePath) {
  const value = String(remotePath || "").trim().replaceAll("\\", "/");
  if (!value || value === ".") {
    return "/";
  }
  if (value === "~" || value === "~/") {
    return "~";
  }
  if (value.startsWith("~/")) {
    return value;
  }
  if (value.startsWith("/") && isWindowsDrivePath(value.slice(1))) {
    return normalizeWindowsDrivePath(value.slice(1));
  }
  if (isWindowsDrivePath(value)) {
    return normalizeWindowsDrivePath(value);
  }
  return value.startsWith("/") ? value : `/${value}`;
}

function filesystemPathName(path) {
  const value = normalizeFilesystemPath(path);
  if (value === "/") {
    return "/";
  }
  if (isWindowsDrivePath(value) && value.endsWith(":/")) {
    return value;
  }
  const parts = value.split("/").filter(Boolean);
  return parts[parts.length - 1] || value;
}

function filesystemParentPath(path) {
  const value = normalizeFilesystemPath(path);
  if (value === "/") {
    return "";
  }
  if (value === "~") {
    return "/";
  }
  if (value.startsWith("~/")) {
    const rest = value.slice(2).split("/").filter(Boolean);
    return rest.length <= 1 ? "~" : `~/${rest.slice(0, -1).join("/")}`;
  }
  if (isWindowsDrivePath(value)) {
    const slash = value.lastIndexOf("/");
    return slash <= 2 ? "/" : value.slice(0, slash);
  }
  const slash = value.lastIndexOf("/");
  return slash <= 0 ? "/" : value.slice(0, slash);
}

function filesystemPathLineage(path) {
  const normalized = normalizeFilesystemPath(path);
  const lineage = [];
  let current = normalized;
  while (current && current !== "/") {
    lineage.unshift(current);
    current = filesystemParentPath(current);
  }
  return lineage;
}

function filesystemRootNode(row) {
  const os = parsePlatform(row?.version).os;
  const windows = os === "windows";
  return {
    path: "/",
    name: windows ? "Drives" : "/",
    type: "directory",
    virtual: windows,
    expanded: windows,
    loaded: false,
    loading: false,
    error: "",
    items: []
  };
}

function isWindowsDrivePath(value) {
  return /^[A-Za-z]:/.test(String(value || "").trim());
}

function normalizeWindowsDrivePath(value) {
  const raw = String(value || "").trim().replaceAll("\\", "/");
  const drive = raw.slice(0, 1).toUpperCase();
  const rest = raw.slice(2).replace(/^\/+/, "");
  if (!rest) {
    return `${drive}:/`;
  }

  const parts = rest.split("/").filter((part) => part && part !== ".");
  const clean = [];
  parts.forEach((part) => {
    if (part === "..") {
      clean.pop();
      return;
    }
    clean.push(part);
  });
  return `${drive}:/${clean.join("/")}`;
}

function filesystemAPIPath(suffix = "", params = {}) {
  const base = withProjectQuery(`/api/hosts/${encodeURIComponent(pageState.host.stableId)}/filesystem${suffix}`, pageState.ctx?.project || "");
  const url = new URL(base, window.location.origin);
  const connection = currentConnection();
  if (connection?.connectionId) {
    url.searchParams.set("connectionId", connection.connectionId);
  }
  Object.entries(params).forEach(([key, value]) => {
    if (value === undefined || value === null) {
      return;
    }
    url.searchParams.set(key, String(value));
  });
  return `${url.pathname}${url.search}`;
}

function renderFilesystem() {
  const root = filesystemNode("/");
  const selectedNode = filesystemNode(pageState.filesystem.selectedDirectory || "/");
  elements.filesystemCurrentPath.textContent = filesystemDisplayPath(pageState.filesystem.selectedDirectory || "/");
  elements.filesystemPathInput.value = normalizeFilesystemPath(pageState.filesystem.selectedDirectory || "/");
  elements.refreshFilesystemButton.disabled = !root;
  elements.uploadFilesystemButton.disabled = !selectedNode || selectedNode.virtual;
  elements.filesystemTree.innerHTML = root ? renderFilesystemNode(root, 0) : `<div class="fs-empty">File system is not initialized.</div>`;
  renderFilesystemPreview();
}

function renderFilesystemNode(node, depth) {
  const isDirectory = node.type === "directory";
  const selected = isDirectory && node.path === pageState.filesystem.selectedDirectory;
  const childMarkup = isDirectory && node.expanded
    ? renderFilesystemChildren(node, depth + 1)
    : "";
  const buttonLabel = node.expanded ? "Collapse" : "Expand";
  const rowStyle = `--fs-depth:${depth}`;
  const meta = node.virtual ? "available drives" : filesystemMeta(node);
  const canUpload = isDirectory && !node.virtual;
  const iconClass = node.virtual ? "drives" : (isDirectory ? (node.path === "/" ? "root" : "folder") : "file");
  const iconText = node.path === "/" && !node.virtual ? "/" : "";
  const expander = isDirectory
    ? `<button class="fs-expander" type="button" data-fs-toggle="${escapeAttribute(node.path)}" aria-label="${buttonLabel} ${escapeAttribute(node.name)}">${node.expanded ? "-" : "+"}</button>`
    : `<span class="fs-expander-spacer" aria-hidden="true"></span>`;
  const downloadAction = node.type === "file" && Number(node.size) > MAX_FILE_TRANSFER_BYTES
    ? `<button class="ghost-button fs-small-button" type="button" disabled title="${escapeAttribute(maxFileTransferMessage("download"))}">Max ${escapeHtml(MAX_FILE_TRANSFER_LABEL)}</button>`
    : `${node.type === "file" ? `<button class="ghost-button fs-small-button" type="button" data-fs-download="${escapeAttribute(node.path)}">Download</button>` : ""}`;

  return `
    <div class="fs-node">
      <div class="fs-row ${selected ? "selected" : ""}" style="${rowStyle}">
        ${expander}
        <button class="fs-main" type="button" ${isDirectory ? `data-fs-select="${escapeAttribute(node.path)}"` : "disabled"}>
          <span class="fs-icon ${iconClass}">${iconText}</span>
          <span class="fs-name">${escapeHtml(node.name || node.path)}</span>
          <span class="fs-meta">${escapeHtml(meta)}</span>
        </button>
        <div class="fs-actions">
          ${canUpload ? `<button class="ghost-button fs-small-button" type="button" data-fs-upload="${escapeAttribute(node.path)}">Upload</button>` : ""}
          ${node.type === "file" ? `<button class="ghost-button fs-small-button" type="button" data-fs-preview="${escapeAttribute(node.path)}">Preview</button>` : ""}
          ${downloadAction}
        </div>
      </div>
      ${node.loading ? `<div class="fs-state" style="${rowStyle}">Loading...</div>` : ""}
      ${node.error ? `<div class="fs-state fs-state-error" style="${rowStyle}">${escapeHtml(node.error)}</div>` : ""}
      ${childMarkup}
    </div>
  `;
}

function renderFilesystemChildren(node, depth) {
  if (node.loading) {
    return "";
  }
  if (!node.loaded) {
    return `<div class="fs-state" style="--fs-depth:${depth}">Open this directory to load its contents.</div>`;
  }
  if (!node.items.length) {
    return `<div class="fs-state" style="--fs-depth:${depth}">Empty directory.</div>`;
  }

  return node.items.map((item) => {
    if (item.type === "directory") {
      return renderFilesystemNode(filesystemNode(item.path) || {
        ...item,
        expanded: false,
        loaded: false,
        loading: false,
        error: "",
        items: []
      }, depth);
    }
    return renderFilesystemNode(item, depth);
  }).join("");
}

function filesystemMeta(node) {
  const parts = [];
  if (node.type && node.type !== "directory") {
    parts.push(node.type);
  }
  if (node.type === "file") {
    parts.push(formatFileSize(node.size));
  }
  if (node.mode) {
    parts.push(node.mode);
  }
  if (node.modifiedAt) {
    parts.push(formatDate(node.modifiedAt));
  }
  return parts.join(" · ");
}

function filesystemDisplayPath(remotePath) {
  const path = normalizeFilesystemPath(remotePath);
  const node = filesystemNode(path);
  if (node?.virtual) {
    return "Drives";
  }
  return path;
}

function formatFileSize(size) {
  const value = Number(size);
  if (!Number.isFinite(value) || value < 0) {
    return "-";
  }
  if (value < 1024) {
    return `${value} B`;
  }
  const units = ["KB", "MB", "GB", "TB"];
  let current = value / 1024;
  for (const unit of units) {
    if (current < 1024) {
      return `${current.toFixed(current >= 10 ? 1 : 2)} ${unit}`;
    }
    current /= 1024;
  }
  return `${current.toFixed(1)} PB`;
}

function setFilesystemMessage(message, type) {
  elements.filesystemMessage.textContent = message;
  elements.filesystemMessage.className = `filesystem-message ${type ? `is-${type}` : ""}`;
}

function renderFilesystemPreview() {
  const preview = pageState.filesystem.preview;
  elements.filesystemPreviewPanel.classList.toggle("hidden", !preview);
  if (!preview) {
    elements.filesystemPreviewPath.textContent = "-";
    elements.filesystemPreviewContent.textContent = "";
    return;
  }

  elements.filesystemPreviewPath.textContent = preview.path || preview.name || "-";
  elements.filesystemPreviewContent.textContent = preview.content || "";
}

function clearFilesystemPreview() {
  pageState.filesystem.preview = null;
  renderFilesystemPreview();
}

function filesystemErrorMessage(message) {
  const value = String(message || "request failed").trim();
  if (value.toLowerCase().includes("permission denied")) {
    return "permission denied";
  }
  if (value.toLowerCase().includes("exceeds maximum transfer size") || value.toLowerCase().includes("request entity too large")) {
    return maxFileTransferMessage("transfer");
  }
  return value;
}

function maxFileTransferMessage(action) {
  return `Cannot ${action} files larger than ${MAX_FILE_TRANSFER_LABEL}.`;
}

function filenameFromDisposition(header) {
  const value = String(header || "");
  const match = value.match(/filename="?([^";]+)"?/i);
  return match?.[1] || "";
}

function renderHostMetadataForm() {
  elements.hostMetadataPanel.classList.toggle("hidden", !pageState.host);
  if (!pageState.host) {
    return;
  }

  const options = [
    ...pageState.projects.map((project) => ({
      value: project.name,
      label: project.name
    }))
  ];

  elements.hostProjectSelect.innerHTML = options
    .map((option) => `<option value="${escapeAttribute(option.value)}">${escapeHtml(option.label)}</option>`)
    .join("");
  elements.hostProjectSelect.value = pageState.host.project || "";
  elements.hostDisplayNameInput.value = pageState.host.displayName || pageState.host.hostname || "";
  elements.hostTagsInput.value = (pageState.host.tags || []).join(", ");
}

function syncQueryString() {
  const href = hostPageHref(stableId, pageState.selectedConnectionId, pageState.ctx?.project || "");
  window.history.replaceState({}, "", href);
}

async function saveHostMetadata() {
  if (!pageState.host || !pageState.ctx) {
    return;
  }

  const targetProject = elements.hostProjectSelect.value;
  const tags = parseTags(elements.hostTagsInput.value);
  const payload = {
    project: targetProject,
    tags,
    hostname: elements.hostDisplayNameInput.value.trim()
  };

  elements.saveHostMetadataButton.disabled = true;
  elements.hostMetadataOutput.textContent = "Saving client metadata...";

  try {
    const updated = await api(withProjectQuery(`/api/hosts/${encodeURIComponent(pageState.host.stableId)}`, pageState.ctx.project), {
      method: "PATCH",
      body: JSON.stringify(payload)
    });

    pageState.host = updated;
    const targetViewProject = targetProject;
    elements.hostMetadataOutput.textContent = `Client metadata saved.`;

    if (targetViewProject !== pageState.ctx.project) {
      window.location.href = hostPageHref(pageState.host.stableId, pageState.selectedConnectionId, targetViewProject);
      return;
    }

    renderHost();
  } catch (error) {
    elements.hostMetadataOutput.textContent = error.message;
  } finally {
    elements.saveHostMetadataButton.disabled = false;
  }
}

function parseTags(raw) {
  return raw.split(",").map((value) => value.trim()).filter(Boolean);
}

function openTerminal(connectionId, reconnect) {
  const connection = resolveConnection(pageState.host, connectionId);
  if (!pageState.host || !connection) {
    terminalView.setConnected(false);
    terminalView.reset("[no active connection selected]\n");
    renderHost();
    return;
  }

  const alreadyAttached =
    pageState.terminal.socket &&
    pageState.terminal.stableId === pageState.host.stableId &&
    pageState.terminal.connectionId === connection.connectionId;

  if (!reconnect && alreadyAttached) {
    queueTerminalResize();
    elements.terminalViewport.focus();
    return;
  }

  pageState.selectedConnectionId = connection.connectionId;
  syncQueryString();
  closeTerminal();

  const size = measureTerminalSize();
  terminalView.setSize(size.cols, size.rows);
  terminalView.setConnected(false);
  terminalView.reset("");
  terminalView.write(`[connecting to ${connection.connectionId}]\n`);
  elements.terminalViewport.focus();

  const url = websocketURL(
    `/ws/terminal/${encodeURIComponent(pageState.host.stableId)}?cols=${size.cols}&rows=${size.rows}&connectionId=${encodeURIComponent(connection.connectionId)}`
  );
  const socket = new WebSocket(url);

  pageState.terminal.socket = socket;
  pageState.terminal.stableId = pageState.host.stableId;
  pageState.terminal.connectionId = connection.connectionId;
  pageState.terminal.cols = size.cols;
  pageState.terminal.rows = size.rows;

  socket.addEventListener("open", () => {
    if (pageState.terminal.socket !== socket) {
      return;
    }

    terminalView.setConnected(true);
    terminalView.write("[terminal connected]\n");
    sendTerminalResize(size);
    renderHost();
  });

  socket.addEventListener("message", (event) => {
    if (pageState.terminal.socket !== socket) {
      return;
    }

    let message;
    try {
      message = JSON.parse(event.data);
    } catch (error) {
      terminalView.write(String(event.data || ""));
      return;
    }

    if (message.type === "output") {
      terminalView.write(message.data || "");
      return;
    }

    if (message.type === "error") {
      terminalView.write(`\n[error] ${message.data || "unknown error"}\n`);
      return;
    }

    if (message.type === "status") {
      terminalView.write(`\n[${message.data || "status"}]\n`);
    }
  });

  socket.addEventListener("close", () => {
    if (pageState.terminal.socket !== socket) {
      return;
    }

    pageState.terminal.socket = null;
    pageState.terminal.stableId = null;
    pageState.terminal.connectionId = null;
    pageState.terminal.cols = 120;
    pageState.terminal.rows = 36;
    terminalView.setConnected(false);
    terminalView.write("\n[terminal closed]\n");
    renderHost();
  });
}

function closeTerminal() {
  if (pageState.terminal.resizeTimer) {
    window.clearTimeout(pageState.terminal.resizeTimer);
    pageState.terminal.resizeTimer = 0;
  }

  const socket = pageState.terminal.socket;

  pageState.terminal.socket = null;
  pageState.terminal.stableId = null;
  pageState.terminal.connectionId = null;
  pageState.terminal.cols = 120;
  pageState.terminal.rows = 36;
  terminalView.setConnected(false);

  if (!socket) {
    return;
  }

  try {
    socket.send(JSON.stringify({ type: "close" }));
  } catch (error) {
    console.error(error);
  }

  try {
    socket.close();
  } catch (error) {
    console.error(error);
  }
}

function handleTerminalKeydown(event) {
  if (!pageState.terminal.socket || pageState.terminal.socket.readyState !== WebSocket.OPEN) {
    return;
  }

  const controlMap = {
    a: "\u0001",
    c: "\u0003",
    d: "\u0004",
    e: "\u0005",
    k: "\u000b",
    l: "\u000c",
    u: "\u0015",
    w: "\u0017"
  };

  if (event.ctrlKey && !event.metaKey) {
    const control = controlMap[event.key.toLowerCase()];
    if (control) {
      sendTerminalInput(control);
      event.preventDefault();
      return;
    }
  }

  const keyMap = {
    Enter: "\r",
    Backspace: "\u007f",
    Delete: "\u001b[3~",
    Tab: "\t",
    ArrowUp: "\u001b[A",
    ArrowDown: "\u001b[B",
    ArrowRight: "\u001b[C",
    ArrowLeft: "\u001b[D",
    Home: "\u001b[H",
    End: "\u001b[F",
    Escape: "\u001b"
  };

  if (keyMap[event.key]) {
    sendTerminalInput(keyMap[event.key]);
    event.preventDefault();
    return;
  }

  if (event.key.length === 1 && !event.metaKey && !event.ctrlKey) {
    sendTerminalInput(event.key);
    event.preventDefault();
  }
}

function handleTerminalPaste(event) {
  if (!pageState.terminal.socket || pageState.terminal.socket.readyState !== WebSocket.OPEN) {
    return;
  }

  sendTerminalInput(event.clipboardData.getData("text"));
  event.preventDefault();
}

function sendTerminalInput(data) {
  if (!pageState.terminal.socket || pageState.terminal.socket.readyState !== WebSocket.OPEN) {
    return;
  }

  pageState.terminal.socket.send(JSON.stringify({ type: "input", data }));
}

function queueTerminalResize() {
  if (!pageState.terminal.socket || pageState.terminal.socket.readyState !== WebSocket.OPEN) {
    return;
  }

  if (pageState.terminal.resizeTimer) {
    window.clearTimeout(pageState.terminal.resizeTimer);
  }

  pageState.terminal.resizeTimer = window.setTimeout(() => {
    pageState.terminal.resizeTimer = 0;
    sendTerminalResize(measureTerminalSize());
  }, 120);
}

function sendTerminalResize(size) {
  if (!pageState.terminal.socket || pageState.terminal.socket.readyState !== WebSocket.OPEN) {
    return;
  }

  terminalView.setSize(size.cols, size.rows);
  pageState.terminal.cols = size.cols;
  pageState.terminal.rows = size.rows;
  pageState.terminal.socket.send(JSON.stringify({
    type: "resize",
    cols: size.cols,
    rows: size.rows
  }));
}

function measureTerminalSize() {
  const viewportWidth = Math.max(elements.terminalViewport.clientWidth - 40, 320);
  const viewportHeight = Math.max(elements.terminalViewport.clientHeight - 40, 200);
  const computed = window.getComputedStyle(elements.terminalOutput);
  const probe = document.createElement("span");
  probe.textContent = "MMMMMMMMMM";
  probe.style.position = "absolute";
  probe.style.visibility = "hidden";
  probe.style.whiteSpace = "pre";
  probe.style.font = computed.font;
  probe.style.lineHeight = computed.lineHeight;
  document.body.appendChild(probe);
  const rect = probe.getBoundingClientRect();
  document.body.removeChild(probe);

  const charWidth = rect.width > 0 ? rect.width / 10 : 9;
  const lineHeight = rect.height > 0 ? rect.height : parseFloat(computed.lineHeight) || 22;

  return {
    cols: Math.max(60, Math.floor(viewportWidth / charWidth)),
    rows: Math.max(18, Math.floor(viewportHeight / lineHeight))
  };
}
