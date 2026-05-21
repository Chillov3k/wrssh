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
const filesystemAvailable = Boolean(
  elements.fileSystemOverlay &&
  elements.closeFilesystemButton &&
  elements.filesystemTitle &&
  elements.filesystemMeta &&
  elements.filesystemCurrentPath &&
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
if (filesystemAvailable) {
  elements.openFilesystemButton?.addEventListener("click", openFilesystem);
  elements.closeFilesystemButton.addEventListener("click", closeFilesystem);
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
    ["Full Shell", commands.ssh],
    ["SCP", commands.scp],
    ["Dynamic SOCKS", commands.dynamicSocks],
    ["Remote Forward", commands.remoteForward]
  ].map(([label, command]) => `
    <article class="command-card">
      <strong>${escapeHtml(label)}</strong>
      <code>${escapeHtml(command)}</code>
      <div class="inline-actions">
        <button class="ghost-button" data-copy-command="${escapeAttribute(command)}">Copy</button>
      </div>
    </article>
  `).join("");
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

function openFilesystem() {
  const connection = currentConnection();
  if (!pageState.host || !connection) {
    return;
  }

  pageState.filesystem.open = true;
  pageState.filesystem.selectedDirectory = "/";
  pageState.filesystem.uploadDirectory = "/";
  pageState.filesystem.preview = null;
  pageState.filesystem.nodes = new Map([
    ["/", {
      path: "/",
      name: "/",
      type: "directory",
      expanded: false,
      loaded: false,
      loading: false,
      error: "",
      items: []
    }]
  ]);

  const row = currentRow(connection);
  elements.filesystemTitle.textContent = `${row.hostname || row.stableId} File System`;
  elements.filesystemMeta.textContent = `Connection ${row.connectionId || "-"} · ${row.remoteAddr || row.ip || "-"} · max upload/download ${MAX_FILE_TRANSFER_LABEL}`;
  setFilesystemMessage("", "");
  elements.fileSystemOverlay.classList.add("visible");
  elements.fileSystemOverlay.setAttribute("aria-hidden", "false");
  renderFilesystem();
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
  if (!node || node.type !== "directory") {
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

function normalizeFilesystemPath(remotePath) {
  const value = String(remotePath || "").trim();
  if (!value || value === ".") {
    return "/";
  }
  return value.startsWith("/") ? value : `/${value}`;
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
  elements.filesystemCurrentPath.textContent = pageState.filesystem.selectedDirectory || "/";
  elements.refreshFilesystemButton.disabled = !root;
  elements.uploadFilesystemButton.disabled = !root;
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
  const meta = filesystemMeta(node);
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
          <span class="fs-icon ${isDirectory ? (node.path === "/" ? "root" : "folder") : "file"}">${node.path === "/" ? "/" : ""}</span>
          <span class="fs-name">${escapeHtml(node.name || node.path)}</span>
          <span class="fs-meta">${escapeHtml(meta)}</span>
        </button>
        <div class="fs-actions">
          ${isDirectory ? `<button class="ghost-button fs-small-button" type="button" data-fs-upload="${escapeAttribute(node.path)}">Upload</button>` : ""}
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
