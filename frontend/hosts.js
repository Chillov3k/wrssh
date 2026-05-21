import {
  api,
  appPath,
  buildJumpTarget,
  copyFromButton,
  escapeAttribute,
  escapeHtml,
  formatDate,
  getClientRows,
  hostMetrics,
  hostCommandTemplates,
  hostPageHref,
  initPage,
  loadHosts,
  loadSystemOptions,
  markSynced,
  parsePlatform,
  rowMatchesQuery,
  withProjectQuery
} from "./shared.js";

const hostSearch = document.getElementById("hostSearch");
const hostTable = document.getElementById("hostTable");
const osFilter = document.getElementById("osFilter");
const bulkCommandInput = document.getElementById("bulkCommandInput");
const bulkSelectionSummary = document.getElementById("bulkSelectionSummary");
const selectVisibleHostsButton = document.getElementById("selectVisibleHostsButton");
const clearSelectedHostsButton = document.getElementById("clearSelectedHostsButton");
const runSelectedHostsButton = document.getElementById("runSelectedHostsButton");
const bulkExecOutput = document.getElementById("bulkExecOutput");
const offlineDeleteOverlay = document.getElementById("offlineDeleteOverlay");
const offlineDeleteMessage = document.getElementById("offlineDeleteMessage");
const offlineDeleteError = document.getElementById("offlineDeleteError");
const deleteOnlyOfflineButton = document.getElementById("deleteOnlyOfflineButton");
const deleteAllOfflineButton = document.getElementById("deleteAllOfflineButton");
const cancelOfflineDeleteButton = document.getElementById("cancelOfflineDeleteButton");
const onlineDeleteOverlay = document.getElementById("onlineDeleteOverlay");
const onlineDeleteMessage = document.getElementById("onlineDeleteMessage");
const onlineDeleteError = document.getElementById("onlineDeleteError");
const confirmOnlineDeleteButton = document.getElementById("confirmOnlineDeleteButton");
const cancelOnlineDeleteButton = document.getElementById("cancelOnlineDeleteButton");
const fileSystemOverlay = document.getElementById("fileSystemOverlay");
const closeFilesystemButton = document.getElementById("closeFilesystemButton");
const filesystemTitle = document.getElementById("filesystemTitle");
const filesystemMetaText = document.getElementById("filesystemMeta");
const filesystemCurrentPath = document.getElementById("filesystemCurrentPath");
const refreshFilesystemButton = document.getElementById("refreshFilesystemButton");
const uploadFilesystemButton = document.getElementById("uploadFilesystemButton");
const filesystemMessage = document.getElementById("filesystemMessage");
const filesystemTree = document.getElementById("filesystemTree");
const filesystemPreviewPanel = document.getElementById("filesystemPreviewPanel");
const filesystemPreviewPath = document.getElementById("filesystemPreviewPath");
const filesystemPreviewContent = document.getElementById("filesystemPreviewContent");
const closeFilesystemPreviewButton = document.getElementById("closeFilesystemPreviewButton");
const filesystemUploadInput = document.getElementById("filesystemUploadInput");

const OS_FILTERS = [
  { key: "all", label: "All" },
  { key: "linux", label: "Linux" },
  { key: "windows", label: "Windows" },
  { key: "darwin", label: "macOS" }
];

const MAX_FILE_TRANSFER_BYTES = 500 * 1024 * 1024;
const MAX_FILE_TRANSFER_LABEL = "500 MiB";

const pageState = {
  hosts: [],
  systemOptions: null,
  ctx: null,
  osFilter: "all",
  selectedRows: new Set(),
  runningRows: new Set(),
  commandResults: new Map(),
  pendingOfflineDelete: null,
  pendingOnlineDelete: null,
  filesystem: {
    target: null,
    selectedDirectory: "/",
    uploadDirectory: "/",
    nodes: new Map(),
    preview: null
  }
};

hostSearch.addEventListener("input", renderHosts);
osFilter.addEventListener("click", (event) => {
  const button = event.target.closest("[data-os-filter]");
  if (!button) {
    return;
  }

  pageState.osFilter = button.dataset.osFilter || "all";
  renderHosts();
});
selectVisibleHostsButton.addEventListener("click", () => {
  filteredRows().forEach((row) => pageState.selectedRows.add(row.key));
  renderHosts();
});
clearSelectedHostsButton.addEventListener("click", () => {
  pageState.selectedRows.clear();
  renderHosts();
});
runSelectedHostsButton.addEventListener("click", runSelectedHosts);
bulkCommandInput.addEventListener("keydown", (event) => {
  if (event.key !== "Enter") {
    return;
  }
  event.preventDefault();
  runSelectedHosts();
});
cancelOfflineDeleteButton.addEventListener("click", closeOfflineDeleteModal);
offlineDeleteOverlay.addEventListener("click", (event) => {
  if (event.target === offlineDeleteOverlay) {
    closeOfflineDeleteModal();
  }
});
deleteOnlyOfflineButton.addEventListener("click", () => deleteOfflineFromModal("single"));
deleteAllOfflineButton.addEventListener("click", () => deleteOfflineFromModal("all"));
cancelOnlineDeleteButton.addEventListener("click", closeOnlineDeleteModal);
onlineDeleteOverlay.addEventListener("click", (event) => {
  if (event.target === onlineDeleteOverlay) {
    closeOnlineDeleteModal();
  }
});
confirmOnlineDeleteButton.addEventListener("click", deleteOnlineFromModal);
closeFilesystemButton.addEventListener("click", closeFilesystem);
refreshFilesystemButton.addEventListener("click", refreshSelectedFilesystemDirectory);
uploadFilesystemButton.addEventListener("click", () => triggerFilesystemUpload(pageState.filesystem.selectedDirectory));
filesystemTree.addEventListener("click", handleFilesystemClick);
closeFilesystemPreviewButton.addEventListener("click", clearFilesystemPreview);
filesystemUploadInput.addEventListener("change", uploadSelectedFilesystemFile);
fileSystemOverlay.addEventListener("click", (event) => {
  if (event.target === fileSystemOverlay) {
    closeFilesystem();
  }
});
document.addEventListener("keydown", (event) => {
  if (event.key === "Escape" && offlineDeleteOverlay.classList.contains("visible")) {
    closeOfflineDeleteModal();
  }
  if (event.key === "Escape" && onlineDeleteOverlay.classList.contains("visible")) {
    closeOnlineDeleteModal();
  }
  if (event.key === "Escape" && fileSystemOverlay.classList.contains("visible")) {
    closeFilesystem();
  }
});

hostTable.addEventListener("click", async (event) => {
  const selectVisibleToggle = event.target.closest("[data-select-visible-toggle]");
  if (selectVisibleToggle) {
    toggleVisibleSelection(selectVisibleToggle.checked);
    renderHosts();
    return;
  }

  const rowSelector = event.target.closest("[data-select-row]");
  if (rowSelector) {
    toggleRowSelection(rowSelector.dataset.selectRow || "", rowSelector.checked);
    syncBulkControls(filteredRows());
    syncVisibleSelectionToggle(filteredRows());
    return;
  }

  const deleteButton = event.target.closest("[data-delete-host]");
  if (deleteButton) {
    await deleteHost(deleteButton);
    return;
  }

  const filesystemButton = event.target.closest("[data-open-filesystem]");
  if (filesystemButton) {
    openFilesystemForRow(filesystemButton.dataset.openFilesystem || "");
    return;
  }

  const copyButton = event.target.closest("[data-copy-command]");
  if (copyButton) {
    await copyFromButton(copyButton, copyButton.dataset.copyCommand, "Copied");
    return;
  }

  const row = event.target.closest("[data-open-shell]");
  if (!row || event.target.closest("a,button,input,label,.client-command-result")) {
    return;
  }

  window.location.href = row.dataset.openShell;
});

initPage({
  title: "Hosts",
  requireProject: true,
  load: async (ctx) => {
    pageState.ctx = ctx;
    const [hosts, systemOptions] = await Promise.all([loadHosts(), loadSystemOptions()]);
    pageState.hosts = hosts;
    pageState.systemOptions = systemOptions;
    renderHosts();

    const metrics = hostMetrics(hosts);
    markSynced(ctx, `${ctx.project} · ${metrics.activeConnections} online / ${metrics.uniqueHosts} unique hosts`);
  }
}).catch((error) => {
  console.error(error);
});

function renderHosts() {
  const allRows = getClientRows(pageState.hosts);
  pruneRowState(allRows);

  const counts = countRowsByOS(allRows);
  syncOSFilterButtons(counts);

  const rows = filteredRows(allRows);
  syncBulkControls(rows);

  if (!rows.length) {
    hostTable.innerHTML = `<div class="empty-state"><p>No clients matched the current search or OS filter.</p></div>`;
    syncVisibleSelectionToggle([]);
    return;
  }

  const jumpTarget = buildJumpTarget(pageState.systemOptions, rows[0]?.host || null, pageState.ctx?.user);
  const selectedVisibleCount = rows.filter((row) => pageState.selectedRows.has(row.key)).length;

  hostTable.innerHTML = `
    <div class="table-toolbar">
      <div>
        <p class="eyebrow">Client Inventory</p>
        <h3>${rows.length} client${rows.length === 1 ? "" : "s"}</h3>
      </div>
      <div class="inline-actions">
        <span class="chip">${rows.filter((row) => row.connected).length} online</span>
        <span class="chip">${rows.filter((row) => !row.connected).length} offline</span>
      </div>
    </div>
    <div class="table-scroll">
      <div class="client-table client-table-hosts">
        <div class="client-table-head client-table-head-hosts">
          <span class="client-select-head">
            <input type="checkbox" class="row-selector" aria-label="Select all visible hosts" data-select-visible-toggle ${selectedVisibleCount > 0 && selectedVisibleCount === rows.length ? "checked" : ""}>
          </span>
          <span>ID</span>
          <span>Host</span>
          <span>Network</span>
          <span>Platform</span>
          <span>Timestamps</span>
          <span>Status</span>
        </div>
        <div class="client-table-body">
          ${rows.map((row) => renderRow(row, jumpTarget)).join("")}
        </div>
      </div>
    </div>
  `;

  syncVisibleSelectionToggle(rows);
}

function filteredRows(existingRows = null) {
  const query = hostSearch.value.trim().toLowerCase();
  const rows = existingRows || getClientRows(pageState.hosts);
  return rows.filter((row) => rowMatchesQuery(row, query) && rowMatchesOS(row, pageState.osFilter));
}

function countRowsByOS(rows) {
  const counts = {
    all: rows.length,
    linux: 0,
    windows: 0,
    darwin: 0
  };

  rows.forEach((row) => {
    const os = parsePlatform(row.version).os;
    if (os === "linux" || os === "windows" || os === "darwin") {
      counts[os] += 1;
    }
  });

  return counts;
}

function syncOSFilterButtons(counts) {
  osFilter.querySelectorAll("[data-os-filter]").forEach((button) => {
    const key = button.dataset.osFilter || "all";
    const definition = OS_FILTERS.find((item) => item.key === key);
    const count = counts[key] || 0;
    button.classList.toggle("is-active", key === pageState.osFilter);
    button.textContent = `${definition?.label || key} ${count}`;
  });
}

function rowMatchesOS(row, filter) {
  if (!filter || filter === "all") {
    return true;
  }

  return parsePlatform(row.version).os === filter;
}

function renderRow(row, jumpTarget) {
  const command = hostCommandTemplates(row.host, row.connection, jumpTarget).ssh;
  const href = hostPageHref(row.stableId, row.connectionId, pageState.ctx?.project || "");
  const deleteLabel = row.connected ? "Delete Client" : "Delete Offline Client";
  const selected = pageState.selectedRows.has(row.key);
  const running = pageState.runningRows.has(row.key);
  const execution = pageState.commandResults.get(row.key);

  return `
    <article class="client-row client-row-hosts" data-open-shell="${escapeAttribute(href)}">
      <div class="client-cell client-cell-select">
        <input type="checkbox" class="row-selector" aria-label="Select host ${escapeAttribute(row.hostname)}" data-select-row="${escapeAttribute(row.key)}" ${selected ? "checked" : ""}>
      </div>
      <div class="client-cell client-cell-code">
        <strong class="table-code-primary">${escapeHtml(row.connectionId || row.hostId)}</strong>
        <span class="muted table-code-secondary">${escapeHtml(row.stableId)}</span>
      </div>
      <div class="client-cell">
        <strong>${escapeHtml(row.hostname)}</strong>
        <span class="muted">${escapeHtml(row.project || "Unassigned")}</span>
        <span class="muted wrap-anywhere">${escapeHtml(row.tags.join(", ") || row.comment || "-")}</span>
      </div>
      <div class="client-cell">
        <strong>${escapeHtml(row.ip)}</strong>
        <span class="muted wrap-anywhere">${escapeHtml(row.remoteAddr || "-")}</span>
      </div>
      <div class="client-cell">
        <strong>${escapeHtml(row.platform)}</strong>
        <span class="muted wrap-anywhere">${escapeHtml(row.version || "-")}</span>
      </div>
      <div class="client-cell">
        <span>Added ${escapeHtml(formatDate(row.dateAdded))}</span>
        <span class="muted">Last ${escapeHtml(formatDate(row.lastActivityAt))}</span>
      </div>
      <div class="client-cell client-cell-status">
        <span class="status-pill ${row.connected ? "online" : "offline"}">
          <span class="status-dot ${row.connected ? "online" : "offline"}"></span>${row.connected ? "Online" : "Offline"}
        </span>
        <button class="ghost-button host-filesystem-button" type="button" data-open-filesystem="${escapeAttribute(row.key)}" ${row.connected ? "" : "disabled"}>Open file system</button>
      </div>
      <div class="client-toolbar-slot client-toolbar-slot-select">
        <a class="action-button action-link host-row-action" href="${escapeAttribute(href)}">Open Shell</a>
      </div>
      <div class="client-toolbar-slot client-toolbar-slot-id">
        <button class="ghost-button host-row-action" data-copy-command="${escapeAttribute(command)}">Copy Connect Command</button>
      </div>
      <div class="client-toolbar-slot client-toolbar-slot-host">
        <button class="danger-button host-row-action" data-delete-host="${escapeAttribute(row.stableId)}" data-hostname="${escapeAttribute(row.hostname)}" data-connected="${row.connected ? "true" : "false"}">${escapeHtml(deleteLabel)}</button>
      </div>
      ${running ? `<div class="client-toolbar-status"><span class="chip">Running command...</span></div>` : ""}
      ${renderCommandResult(execution)}
    </article>
  `;
}

function renderCommandResult(execution) {
  if (!execution) {
    return "";
  }

  const status = execution.error
    ? (execution.timedOut ? "Timed Out" : "Failed")
    : "Completed";
  const statusClass = execution.error ? "offline" : "online";

  let body = execution.output || "";
  if (execution.error) {
    body = body ? `${body}\n\n[error] ${execution.error}` : `[error] ${execution.error}`;
  }
  if (!body) {
    body = "[no output]";
  }

  return `
    <div class="client-command-result shell-panel">
      <div class="client-command-result-head">
        <div>
          <p class="eyebrow">Command Result</p>
          <h4>${escapeHtml(execution.command)}</h4>
        </div>
        <span class="status-pill ${statusClass}">${escapeHtml(status)}</span>
      </div>
      <pre class="output-block small-output">${escapeHtml(body)}</pre>
    </div>
  `;
}

function toggleRowSelection(key, checked) {
  if (!key) {
    return;
  }

  if (checked) {
    pageState.selectedRows.add(key);
    return;
  }

  pageState.selectedRows.delete(key);
}

function toggleVisibleSelection(checked) {
  filteredRows().forEach((row) => toggleRowSelection(row.key, checked));
}

function syncVisibleSelectionToggle(rows) {
  const toggle = hostTable.querySelector("[data-select-visible-toggle]");
  if (!toggle) {
    return;
  }

  const selectedVisibleCount = rows.filter((row) => pageState.selectedRows.has(row.key)).length;
  toggle.checked = rows.length > 0 && selectedVisibleCount === rows.length;
  toggle.indeterminate = selectedVisibleCount > 0 && selectedVisibleCount < rows.length;
}

function syncBulkControls(rows) {
  const selectedCount = pageState.selectedRows.size;
  bulkSelectionSummary.textContent = `${selectedCount} selected`;
  bulkSelectionSummary.classList.toggle("hidden", selectedCount === 0);
  selectVisibleHostsButton.disabled = rows.length === 0 || pageState.runningRows.size > 0;
  clearSelectedHostsButton.disabled = selectedCount === 0 || pageState.runningRows.size > 0;
  runSelectedHostsButton.disabled = selectedCount === 0 || pageState.runningRows.size > 0;
  runSelectedHostsButton.textContent = pageState.runningRows.size > 0 ? "Running..." : "Run On Selected";
}

function pruneRowState(rows) {
  const keys = new Set(rows.map((row) => row.key));

  pageState.selectedRows.forEach((key) => {
    if (!keys.has(key)) {
      pageState.selectedRows.delete(key);
    }
  });
  pageState.runningRows.forEach((key) => {
    if (!keys.has(key)) {
      pageState.runningRows.delete(key);
    }
  });
  pageState.commandResults.forEach((_, key) => {
    if (!keys.has(key)) {
      pageState.commandResults.delete(key);
    }
  });
}

async function runSelectedHosts() {
  const command = bulkCommandInput.value.trim();
  if (!command) {
    bulkExecOutput.textContent = "Enter a command to run on the selected hosts.";
    return;
  }

  const rowMap = new Map(getClientRows(pageState.hosts).map((row) => [row.key, row]));
  const selectedRows = [...pageState.selectedRows]
    .map((key) => rowMap.get(key))
    .filter(Boolean);

  if (!selectedRows.length) {
    bulkExecOutput.textContent = "Select at least one host first.";
    return;
  }

  bulkExecOutput.textContent = "";

  selectedRows.forEach((row) => {
    pageState.commandResults.delete(row.key);
    pageState.runningRows.add(row.key);
  });
  renderHosts();

  try {
    const response = await api(withProjectQuery("/api/hosts/exec", pageState.ctx?.project || ""), {
      method: "POST",
      body: JSON.stringify({
        command,
        targets: selectedRows.map((row) => ({
          stableId: row.stableId,
          connectionId: row.connectionId || ""
        }))
      })
    });

    selectedRows.forEach((row) => pageState.runningRows.delete(row.key));
    for (const item of response.items || []) {
      const key = item.key || `${item.stableId}:${item.connectionId || "offline"}`;
      pageState.commandResults.set(key, {
        command,
        output: item.output || "",
        error: item.error || "",
        timedOut: Boolean(item.timedOut)
      });
    }
  } catch (error) {
    selectedRows.forEach((row) => pageState.runningRows.delete(row.key));
    bulkExecOutput.textContent = error.message;
  } finally {
    renderHosts();
  }
}

async function deleteHost(button) {
  const stableId = button.dataset.deleteHost || "";
  const hostname = button.dataset.hostname || stableId;
  const connected = button.dataset.connected === "true";
  if (!connected) {
    openOfflineDeleteModal(stableId, hostname);
    return;
  }

  openOnlineDeleteModal(stableId, hostname);
}

function openOfflineDeleteModal(stableId, hostname) {
  const offlineRows = offlineHostRows();
  const totalOffline = offlineRows.length;

  pageState.pendingOfflineDelete = {
    stableId,
    hostname,
    totalOffline
  };

  offlineDeleteError.textContent = "";
  offlineDeleteMessage.textContent = totalOffline > 1
    ? `${hostname} is offline. Delete only this client or remove all ${totalOffline} offline clients from this project.`
    : `${hostname} is offline. Delete this client from the project inventory.`;
  deleteAllOfflineButton.disabled = totalOffline <= 1;
  deleteOnlyOfflineButton.disabled = false;
  cancelOfflineDeleteButton.disabled = false;
  offlineDeleteOverlay.classList.add("visible");
  deleteOnlyOfflineButton.focus();
}

function closeOfflineDeleteModal(force = false) {
  if (!offlineDeleteOverlay.classList.contains("visible")) {
    return;
  }
  if (!force && cancelOfflineDeleteButton.disabled) {
    return;
  }

  pageState.pendingOfflineDelete = null;
  offlineDeleteOverlay.classList.remove("visible");
  offlineDeleteError.textContent = "";
  deleteOnlyOfflineButton.textContent = "Delete Only This Client";
  deleteAllOfflineButton.textContent = "Delete All Offline Clients";
  deleteOnlyOfflineButton.disabled = false;
  deleteAllOfflineButton.disabled = false;
  cancelOfflineDeleteButton.disabled = false;
}

async function deleteOfflineFromModal(mode) {
  const pending = pageState.pendingOfflineDelete;
  if (!pending) {
    return;
  }

  const targets = mode === "all"
    ? offlineHostRows().map((row) => row.stableId)
    : [pending.stableId];
  const stableIds = [...new Set(targets.filter(Boolean))];

  if (!stableIds.length) {
    closeOfflineDeleteModal();
    return;
  }

  const actionButton = mode === "all" ? deleteAllOfflineButton : deleteOnlyOfflineButton;
  const original = actionButton.textContent;
  offlineDeleteError.textContent = "";
  deleteOnlyOfflineButton.disabled = true;
  deleteAllOfflineButton.disabled = true;
  cancelOfflineDeleteButton.disabled = true;
  actionButton.textContent = mode === "all" ? "Deleting Offline Clients..." : "Deleting Client...";

  try {
    await deleteHostRecords(stableIds);
    closeOfflineDeleteModal(true);
    await refreshHostsAfterMutation();
  } catch (error) {
    offlineDeleteError.textContent = error.message;
    actionButton.textContent = original;
    deleteOnlyOfflineButton.disabled = false;
    deleteAllOfflineButton.disabled = pageState.pendingOfflineDelete?.totalOffline <= 1;
    cancelOfflineDeleteButton.disabled = false;
  }
}

function offlineHostRows() {
  return getClientRows(pageState.hosts).filter((row) => !row.connected);
}

async function deleteHostRecords(stableIds) {
  for (const stableId of stableIds) {
    await api(withProjectQuery(`/api/hosts/${encodeURIComponent(stableId)}`, pageState.ctx?.project || ""), {
      method: "DELETE"
    });
  }
}

async function refreshHostsAfterMutation() {
  const [hosts, systemOptions] = await Promise.all([loadHosts(), loadSystemOptions()]);
  pageState.hosts = hosts;
  pageState.systemOptions = systemOptions;
  renderHosts();

  if (pageState.ctx) {
    const metrics = hostMetrics(hosts);
    markSynced(pageState.ctx, `${pageState.ctx.project} · ${metrics.activeConnections} online / ${metrics.uniqueHosts} unique hosts`);
  }
}

function openOnlineDeleteModal(stableId, hostname) {
  pageState.pendingOnlineDelete = {
    stableId,
    hostname
  };

  onlineDeleteError.textContent = "";
  onlineDeleteMessage.textContent = `${hostname} is online. Deleting it will kill active connections and remove the host record from this project inventory.`;
  confirmOnlineDeleteButton.disabled = false;
  cancelOnlineDeleteButton.disabled = false;
  onlineDeleteOverlay.classList.add("visible");
  confirmOnlineDeleteButton.focus();
}

function closeOnlineDeleteModal(force = false) {
  if (!onlineDeleteOverlay.classList.contains("visible")) {
    return;
  }
  if (!force && cancelOnlineDeleteButton.disabled) {
    return;
  }

  pageState.pendingOnlineDelete = null;
  onlineDeleteOverlay.classList.remove("visible");
  onlineDeleteError.textContent = "";
  confirmOnlineDeleteButton.textContent = "Delete Client";
  confirmOnlineDeleteButton.disabled = false;
  cancelOnlineDeleteButton.disabled = false;
}

async function deleteOnlineFromModal() {
  const pending = pageState.pendingOnlineDelete;
  if (!pending?.stableId) {
    return;
  }

  onlineDeleteError.textContent = "";
  confirmOnlineDeleteButton.disabled = true;
  cancelOnlineDeleteButton.disabled = true;
  confirmOnlineDeleteButton.textContent = "Deleting Client...";

  try {
    await deleteHostRecords([pending.stableId]);
    closeOnlineDeleteModal(true);
    await refreshHostsAfterMutation();
  } catch (error) {
    onlineDeleteError.textContent = error.message;
    confirmOnlineDeleteButton.textContent = "Delete Client";
    confirmOnlineDeleteButton.disabled = false;
    cancelOnlineDeleteButton.disabled = false;
  }
}

function openFilesystemForRow(rowKey) {
  const row = getClientRows(pageState.hosts).find((item) => item.key === rowKey);
  if (!row || !row.connected) {
    return;
  }

  pageState.filesystem.target = {
    key: row.key,
    stableId: row.stableId,
    hostname: row.hostname,
    connectionId: row.connectionId,
    remoteAddr: row.remoteAddr || row.ip || "-",
    version: row.version
  };
  pageState.filesystem.selectedDirectory = "/";
  pageState.filesystem.uploadDirectory = "/";
  pageState.filesystem.preview = null;
  const root = filesystemRootNode(row);
  pageState.filesystem.nodes = new Map([[root.path, root]]);

  filesystemTitle.textContent = `${row.hostname || row.stableId} File System`;
  filesystemMetaText.textContent = `Connection ${row.connectionId || "-"} · ${row.remoteAddr || row.ip || "-"} · max upload/download ${MAX_FILE_TRANSFER_LABEL}`;
  setFilesystemMessage("", "");
  fileSystemOverlay.classList.add("visible");
  fileSystemOverlay.setAttribute("aria-hidden", "false");
  renderFilesystem();
  if (root.virtual) {
    void loadFilesystemDirectory(root.path);
  }
}

function closeFilesystem() {
  pageState.filesystem.target = null;
  fileSystemOverlay.classList.remove("visible");
  fileSystemOverlay.setAttribute("aria-hidden", "true");
  filesystemUploadInput.value = "";
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
  if (!node || node.type !== "directory" || node.virtual) {
    return;
  }
  pageState.filesystem.uploadDirectory = node.path;
  pageState.filesystem.selectedDirectory = node.path;
  setFilesystemMessage("", "");
  renderFilesystem();
  filesystemUploadInput.click();
}

async function uploadSelectedFilesystemFile() {
  const file = filesystemUploadInput.files?.[0];
  if (!file) {
    return;
  }
  if (file.size > MAX_FILE_TRANSFER_BYTES) {
    filesystemUploadInput.value = "";
    setFilesystemMessage(maxFileTransferMessage("upload"), "error");
    return;
  }

  const directory = pageState.filesystem.uploadDirectory || pageState.filesystem.selectedDirectory || "/";
  const formData = new FormData();
  formData.append("file", file);

  uploadFilesystemButton.disabled = true;
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
    uploadFilesystemButton.disabled = false;
    filesystemUploadInput.value = "";
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
  const value = String(remotePath || "").trim().replaceAll("\\", "/");
  if (!value || value === ".") {
    return "/";
  }
  if (value.startsWith("/") && isWindowsDrivePath(value.slice(1))) {
    return normalizeWindowsDrivePath(value.slice(1));
  }
  if (isWindowsDrivePath(value)) {
    return normalizeWindowsDrivePath(value);
  }
  return value.startsWith("/") ? value : `/${value}`;
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
  const target = pageState.filesystem.target;
  const stableId = target?.stableId || "";
  const base = withProjectQuery(`/api/hosts/${encodeURIComponent(stableId)}/filesystem${suffix}`, pageState.ctx?.project || "");
  const url = new URL(base, window.location.origin);
  if (target?.connectionId) {
    url.searchParams.set("connectionId", target.connectionId);
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
  filesystemCurrentPath.textContent = filesystemDisplayPath(pageState.filesystem.selectedDirectory || "/");
  refreshFilesystemButton.disabled = !root;
  uploadFilesystemButton.disabled = !selectedNode || selectedNode.virtual;
  filesystemTree.innerHTML = root ? renderFilesystemNode(root, 0) : `<div class="fs-empty">File system is not initialized.</div>`;
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
  filesystemMessage.textContent = message;
  filesystemMessage.className = `filesystem-message ${type ? `is-${type}` : ""}`;
}

function renderFilesystemPreview() {
  const preview = pageState.filesystem.preview;
  filesystemPreviewPanel.classList.toggle("hidden", !preview);
  if (!preview) {
    filesystemPreviewPath.textContent = "-";
    filesystemPreviewContent.textContent = "";
    return;
  }

  filesystemPreviewPath.textContent = preview.path || preview.name || "-";
  filesystemPreviewContent.textContent = preview.content || "";
}

function clearFilesystemPreview() {
  pageState.filesystem.preview = null;
  renderFilesystemPreview();
}

function filesystemErrorMessage(message) {
  const value = String(message || "request failed").trim();
  const lower = value.toLowerCase();
  if (lower.includes("permission denied")) {
    return "permission denied";
  }
  if (lower.includes("exceeds maximum transfer size") || lower.includes("request entity too large")) {
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
