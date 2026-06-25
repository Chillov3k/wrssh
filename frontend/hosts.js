import {
  api,
  appPath,
  buildJumpTarget,
  copyFromButton,
  copyText,
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
const bulkModulePanel = document.getElementById("bulkModulePanel");
const bulkModuleSelect = document.getElementById("bulkModuleSelect");
const bulkModuleHelpText = document.getElementById("bulkModuleHelpText");
const bulkModuleArgsInput = document.getElementById("bulkModuleArgsInput");
const bulkModuleTimeoutInput = document.getElementById("bulkModuleTimeoutInput");
const bulkModuleOutputLimitInput = document.getElementById("bulkModuleOutputLimitInput");
const bulkModuleStdinInput = document.getElementById("bulkModuleStdinInput");
const bulkModuleStdinFileInput = document.getElementById("bulkModuleStdinFileInput");
const refreshBulkModulesButton = document.getElementById("refreshBulkModulesButton");
const runSelectedModuleButton = document.getElementById("runSelectedModuleButton");
const bulkModuleOutput = document.getElementById("bulkModuleOutput");
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
const filesystemPathForm = document.getElementById("filesystemPathForm");
const filesystemPathInput = document.getElementById("filesystemPathInput");
const refreshFilesystemButton = document.getElementById("refreshFilesystemButton");
const uploadFilesystemButton = document.getElementById("uploadFilesystemButton");
const filesystemMessage = document.getElementById("filesystemMessage");
const filesystemTree = document.getElementById("filesystemTree");
const filesystemPreviewPanel = document.getElementById("filesystemPreviewPanel");
const filesystemPreviewPath = document.getElementById("filesystemPreviewPath");
const filesystemPreviewContent = document.getElementById("filesystemPreviewContent");
const closeFilesystemPreviewButton = document.getElementById("closeFilesystemPreviewButton");
const filesystemUploadInput = document.getElementById("filesystemUploadInput");
const hostContextMenu = document.getElementById("hostContextMenu");

const OS_FILTERS = [
  { key: "all", label: "All" },
  { key: "linux", label: "Linux" },
  { key: "windows", label: "Windows" },
  { key: "darwin", label: "macOS" }
];

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

const pageState = {
  hosts: [],
  systemOptions: null,
  ctx: null,
  osFilter: "all",
  sort: {
    key: "status",
    direction: "desc"
  },
  selectedRows: new Set(),
  runningRows: new Set(),
  commandResults: new Map(),
  bulkModules: {
    items: [],
    loading: false,
    running: false,
    error: "",
    loadedRowKey: ""
  },
  pendingOfflineDelete: null,
  pendingOnlineDelete: null,
  contextRowKey: "",
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
bulkModulePanel?.addEventListener("toggle", () => {
  if (bulkModulePanel.open) {
    void loadBulkModules(false);
  }
});
refreshBulkModulesButton?.addEventListener("click", () => loadBulkModules(true));
bulkModuleSelect?.addEventListener("change", syncBulkModuleHelp);
runSelectedModuleButton?.addEventListener("click", runSelectedModuleOnHosts);
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
filesystemPathForm.addEventListener("submit", navigateFilesystemPath);
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
  if (event.key === "Escape" && !hostContextMenu.classList.contains("hidden")) {
    closeHostContextMenu();
  }
});

hostTable.addEventListener("click", async (event) => {
  const sortButton = event.target.closest("[data-host-sort]");
  if (sortButton) {
    setHostSort(sortButton.dataset.hostSort || "");
    return;
  }

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

  const copyResultButton = event.target.closest("[data-copy-result]");
  if (copyResultButton) {
    await copyExecutionResult(copyResultButton);
    return;
  }

  const row = event.target.closest("[data-open-shell]");
  if (!row || event.target.closest("a,button,input,label,.client-command-result")) {
    return;
  }

  window.location.href = row.dataset.openShell;
});

hostTable.addEventListener("contextmenu", (event) => {
  const row = event.target.closest("[data-host-row-key]");
  if (!row) {
    return;
  }
  event.preventDefault();
  openHostContextMenu(row.dataset.hostRowKey || "", event.clientX, event.clientY);
});

hostContextMenu.addEventListener("click", async (event) => {
  const actionButton = event.target.closest("[data-context-action]");
  if (!actionButton) {
    return;
  }
  await handleHostContextAction(actionButton.dataset.contextAction || "");
});

document.addEventListener("click", (event) => {
  if (!hostContextMenu.classList.contains("hidden") && !event.target.closest("#hostContextMenu")) {
    closeHostContextMenu();
  }
});

window.addEventListener("resize", closeHostContextMenu);
window.addEventListener("scroll", closeHostContextMenu, true);

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

  const rows = sortedRows(filteredRows(allRows));
  syncBulkControls(rows);

  if (!rows.length) {
    hostTable.innerHTML = `<div class="empty-state"><p>No clients matched the current search or OS filter.</p></div>`;
    syncVisibleSelectionToggle([]);
    return;
  }

  const selectedVisibleCount = rows.filter((row) => pageState.selectedRows.has(row.key)).length;

  hostTable.innerHTML = `
    <div class="hosts-strip-toolbar">
      <span>${rows.length} client${rows.length === 1 ? "" : "s"}</span>
      <span>${rows.filter((row) => row.connected).length} online</span>
      <span>${rows.filter((row) => !row.connected).length} offline</span>
    </div>
    <div class="hosts-list-shell">
      <div class="hosts-list">
        <div class="hosts-list-head">
          <span class="client-select-head">
            <input type="checkbox" class="row-selector hosts-select-all" title="Select all visible" aria-label="Select all visible hosts" data-select-visible-toggle ${selectedVisibleCount > 0 && selectedVisibleCount === rows.length ? "checked" : ""}>
          </span>
          ${renderSortHeader("os", "OS")}
          ${renderSortHeader("ip", "IP")}
          ${renderSortHeader("user", "User")}
          ${renderSortHeader("host", "Host")}
          ${renderSortHeader("sessionTime", "Session time")}
          ${renderSortHeader("status", "Status")}
        </div>
        <div class="hosts-list-body">
          ${rows.map((row) => renderRow(row)).join("")}
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

function sortedRows(rows) {
  const direction = pageState.sort.direction === "asc" ? 1 : -1;
  return [...rows].sort((left, right) => {
    const result = compareRows(left, right, pageState.sort.key);
    if (result !== 0) {
      return result * direction;
    }

    if (left.connected !== right.connected) {
      return left.connected ? -1 : 1;
    }

    return compareDateRows(right.dateAdded, left.dateAdded) || compareStrings(left.hostname, right.hostname);
  });
}

function compareRows(left, right, key) {
  switch (key) {
  case "os":
    return compareStrings(parsePlatform(left.version).label, parsePlatform(right.version).label);
  case "ip":
    return compareIPLabels(left.ip, right.ip);
  case "user":
    return compareStrings(splitHostIdentity(left.hostname).user, splitHostIdentity(right.hostname).user);
  case "host":
    return compareStrings(splitHostIdentity(left.hostname).host, splitHostIdentity(right.hostname).host);
  case "sessionTime":
    return compareDateRows(left.dateAdded, right.dateAdded);
  case "status":
    return Number(left.connected) - Number(right.connected);
  default:
    return 0;
  }
}

function compareStrings(left, right) {
  return String(left || "").localeCompare(String(right || ""), undefined, { numeric: true, sensitivity: "base" });
}

function compareDateRows(left, right) {
  return dateValue(left) - dateValue(right);
}

function compareIPLabels(left, right) {
  const leftParts = parseIPv4(left);
  const rightParts = parseIPv4(right);
  if (leftParts && rightParts) {
    for (let index = 0; index < leftParts.length; index += 1) {
      if (leftParts[index] !== rightParts[index]) {
        return leftParts[index] - rightParts[index];
      }
    }
    return 0;
  }

  return compareStrings(left, right);
}

function parseIPv4(value) {
  const primary = String(value || "").split("/")[0].trim();
  const parts = primary.split(".");
  if (parts.length !== 4) {
    return null;
  }

  const numbers = parts.map((part) => Number(part));
  if (numbers.some((part) => !Number.isInteger(part) || part < 0 || part > 255)) {
    return null;
  }

  return numbers;
}

function dateValue(value) {
  if (!value) {
    return 0;
  }
  const time = new Date(value).getTime();
  return Number.isNaN(time) ? 0 : time;
}

function setHostSort(key) {
  if (!key) {
    return;
  }

  if (pageState.sort.key === key) {
    pageState.sort.direction = pageState.sort.direction === "asc" ? "desc" : "asc";
  } else {
    pageState.sort.key = key;
    pageState.sort.direction = key === "sessionTime" || key === "status" ? "desc" : "asc";
  }

  renderHosts();
}

function renderSortHeader(key, label) {
  const active = pageState.sort.key === key;
  const direction = active ? pageState.sort.direction : "";
  return `
    <button class="hosts-sort-button ${active ? "is-active" : ""}" type="button" data-host-sort="${escapeAttribute(key)}" aria-sort="${active ? (direction === "asc" ? "ascending" : "descending") : "none"}">
      <span>${escapeHtml(label)}</span>
      <span class="sort-arrows" aria-hidden="true">
        <span class="sort-arrow-up ${active && direction === "asc" ? "active" : ""}"></span>
        <span class="sort-arrow-down ${active && direction === "desc" ? "active" : ""}"></span>
      </span>
    </button>
  `;
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

function osIconMarkup(os) {
  switch (os) {
  case "windows":
    return `
      <svg class="os-svg" viewBox="0 0 24 24" aria-hidden="true">
        <path d="M3 4.4 10.8 3v8.1H3V4.4Zm9.2-1.7L21 1.2v9.9h-8.8V2.7ZM3 12.9h7.8V21L3 19.6v-6.7Zm9.2 0H21v9.9l-8.8-1.5v-8.4Z"></path>
      </svg>
    `;
  case "linux":
    return `
      <svg class="os-svg" viewBox="0 0 24 24" aria-hidden="true">
        <path d="M12 2.6c-2.5 0-4.2 2.1-4.2 5.1 0 1.2-.4 2.3-1.1 3.5-.8 1.2-1.6 2.6-1.6 4.8 0 3.2 2.5 5.4 6.9 5.4s6.9-2.2 6.9-5.4c0-2.1-.8-3.6-1.6-4.8-.7-1.1-1.1-2.2-1.1-3.5 0-3-1.7-5.1-4.2-5.1Zm-1.7 4.7c.6 0 1 .5 1 1.1s-.4 1.1-1 1.1-1-.5-1-1.1.4-1.1 1-1.1Zm3.4 0c.6 0 1 .5 1 1.1s-.4 1.1-1 1.1-1-.5-1-1.1.4-1.1 1-1.1Zm-1.7 5 3.3 1.4-3.3 1.4-3.3-1.4 3.3-1.4Z"></path>
      </svg>
    `;
  case "darwin":
    return `
      <svg class="os-svg" viewBox="0 0 24 24" aria-hidden="true">
        <path d="M16.7 2.4c.1 1.2-.4 2.3-1.2 3.2-.8.9-1.9 1.5-3 1.4-.1-1.1.4-2.2 1.2-3 .8-.9 2-1.5 3-1.6ZM20.2 17.4c-.5 1.2-.8 1.7-1.5 2.8-.9 1.3-2.2 2.9-3.8 2.9-1.4 0-1.8-.9-3.7-.9s-2.3.9-3.7.9c-1.6 0-2.8-1.5-3.7-2.8-2.6-3.8-2.9-8.3-1.3-10.7 1.1-1.7 2.9-2.7 4.6-2.7 1.7 0 2.8.9 4.2.9 1.4 0 2.2-.9 4.2-.9 1.5 0 3 .8 4.1 2.1-3.6 2-3 7.1.6 8.4Z"></path>
      </svg>
    `;
  default:
    return `
      <svg class="os-svg" viewBox="0 0 24 24" aria-hidden="true">
        <path d="M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18Zm0 14.5a1.2 1.2 0 1 1 0-2.4 1.2 1.2 0 0 1 0 2.4Zm1.1-4.3h-2c0-2.8 3-2.8 3-4.6 0-1-.8-1.7-2-1.7-1.1 0-2 .6-2.7 1.5L8 7.1c1-1.4 2.4-2.2 4.2-2.2 2.4 0 4.1 1.4 4.1 3.5 0 2.8-3.2 3-3.2 4.8Z"></path>
      </svg>
    `;
  }
}

function splitHostIdentity(hostname) {
  const value = String(hostname || "-").trim() || "-";
  const dot = value.indexOf(".");
  if (dot > 0 && dot < value.length - 1) {
    return {
      user: value.slice(0, dot),
      host: value.slice(dot + 1)
    };
  }

  return {
    user: "-",
    host: value
  };
}

function compactDate(value) {
  if (!value) {
    return "-";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "-";
  }
  return date.toLocaleString(undefined, {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit"
  });
}

function renderRow(row) {
  const href = hostPageHref(row.stableId, row.connectionId, pageState.ctx?.project || "");
  const selected = pageState.selectedRows.has(row.key);
  const running = pageState.runningRows.has(row.key);
  const execution = pageState.commandResults.get(row.key);
  const platform = parsePlatform(row.version);
  const identity = splitHostIdentity(row.hostname);
  const sessionTime = row.dateAdded;
  const ipTitle = [row.remoteAddr || row.ip, row.internalIp ? `internal ${row.internalIp}` : ""].filter(Boolean).join(" / ");

  return `
    <article class="hosts-list-row ${row.connected ? "is-online" : "is-offline"} ${selected ? "is-selected" : ""}" data-host-row-key="${escapeAttribute(row.key)}" data-open-shell="${escapeAttribute(href)}" title="Right-click for host actions">
      <div class="hosts-list-cell hosts-select-cell">
        <input type="checkbox" class="row-selector" aria-label="Select host ${escapeAttribute(row.hostname)}" data-select-row="${escapeAttribute(row.key)}" ${selected ? "checked" : ""}>
      </div>
      <div class="hosts-list-cell hosts-os-cell">
        <span class="os-icon os-${escapeAttribute(platform.os || "unknown")}" title="${escapeAttribute(platform.label)}">${osIconMarkup(platform.os)}</span>
      </div>
      <div class="hosts-list-cell table-code compact-value" title="${escapeAttribute(ipTitle)}">${escapeHtml(row.ip)}</div>
      <div class="hosts-list-cell compact-value" title="${escapeAttribute(row.hostname)}">${escapeHtml(identity.user)}</div>
      <div class="hosts-list-cell hosts-name-cell">
        <strong title="${escapeAttribute(row.hostname)}">${escapeHtml(identity.host)}</strong>
        <span title="${escapeAttribute(row.connectionId || row.stableId)}">${escapeHtml(row.connectionId || row.stableId)}</span>
      </div>
      <div class="hosts-list-cell compact-value" title="${escapeAttribute(formatDate(sessionTime))}">${escapeHtml(compactDate(sessionTime))}</div>
      <div class="hosts-list-cell hosts-status-cell">
        <span class="hosts-status-text ${row.connected ? "online" : "offline"}">
          <span class="status-dot ${row.connected ? "online" : "offline"}"></span>${row.connected ? "Online" : "Offline"}
        </span>
      </div>
      ${running ? `<div class="hosts-row-running"><span class="chip">Running command...</span></div>` : ""}
      ${renderCommandResult(execution, row.key)}
    </article>
  `;
}

function renderCommandResult(execution, rowKey) {
  if (!execution) {
    return "";
  }

  const status = execution.timedOut
    ? "Timed out"
    : (execution.error ? "Failed" : "Completed");
  const statusClass = execution.error || execution.timedOut ? "offline" : "online";
  const resultLabel = execution.kind === "module" ? "Module result" : "Command result";

  const body = executionOutputText(execution);

  return `
    <div class="client-command-result shell-panel">
      <div class="client-command-result-head">
        <div>
          <p class="eyebrow">${escapeHtml(resultLabel)}</p>
          <h4>${escapeHtml(execution.command)}</h4>
        </div>
        <div class="client-command-result-actions">
          <button class="ghost-button fs-small-button" type="button" data-copy-result="${escapeAttribute(rowKey)}">Copy output</button>
          <span class="status-pill ${statusClass}">${escapeHtml(status)}</span>
        </div>
      </div>
      <pre class="output-block small-output">${escapeHtml(body)}</pre>
    </div>
  `;
}

function executionOutputText(execution) {
  if (!execution) {
    return "";
  }

  const bodyParts = [];
  if (execution.output) {
    bodyParts.push(execution.output);
  }
  if (execution.timedOut) {
    bodyParts.push("[timed out]");
  }
  if (execution.truncated) {
    bodyParts.push("[output truncated]");
  }
  if (execution.error) {
    bodyParts.push(`[error] ${execution.error}`);
  }
  return bodyParts.join(bodyParts.length > 1 ? "\n\n" : "") || "[no output]";
}

async function copyExecutionResult(button) {
  const rowKey = button.dataset.copyResult || "";
  const execution = pageState.commandResults.get(rowKey);
  if (!execution) {
    return;
  }

  const original = button.textContent;
  button.disabled = true;
  try {
    await copyText(executionOutputText(execution));
    button.textContent = "Copied";
  } catch (error) {
    button.textContent = "Copy failed";
  } finally {
    window.setTimeout(() => {
      button.textContent = original;
      button.disabled = false;
    }, 1000);
  }
}

function openHostContextMenu(rowKey, x, y) {
  const row = findRowByKey(rowKey);
  if (!row) {
    closeHostContextMenu();
    return;
  }

  pageState.contextRowKey = row.key;
  hostContextMenu.querySelectorAll("[data-context-action]").forEach((button) => {
    const action = button.dataset.contextAction || "";
    if (action === "filesystem") {
      button.disabled = !row.connected;
    } else {
      button.disabled = false;
    }
  });

  hostContextMenu.classList.remove("hidden");
  hostContextMenu.setAttribute("aria-hidden", "false");
  hostContextMenu.style.left = "0px";
  hostContextMenu.style.top = "0px";

  const rect = hostContextMenu.getBoundingClientRect();
  const left = Math.min(x, window.innerWidth - rect.width - 12);
  const top = Math.min(y, window.innerHeight - rect.height - 12);
  hostContextMenu.style.left = `${Math.max(12, left)}px`;
  hostContextMenu.style.top = `${Math.max(12, top)}px`;
}

function closeHostContextMenu() {
  pageState.contextRowKey = "";
  hostContextMenu.classList.add("hidden");
  hostContextMenu.setAttribute("aria-hidden", "true");
}

async function handleHostContextAction(action) {
  const row = findRowByKey(pageState.contextRowKey);
  closeHostContextMenu();
  if (!row) {
    return;
  }

  switch (action) {
  case "open":
    window.location.href = hostPageHref(row.stableId, row.connectionId, pageState.ctx?.project || "");
    return;
  case "filesystem":
    openFilesystemForRow(row.key);
    return;
  case "copy": {
    const jumpTarget = buildJumpTarget(pageState.systemOptions, row.host, pageState.ctx?.user);
    await copyText(hostCommandTemplates(row.host, row.connection, jumpTarget).ssh);
    return;
  }
  case "select":
    toggleRowSelection(row.key, !pageState.selectedRows.has(row.key));
    renderHosts();
    return;
  case "delete":
    requestDeleteHost(row);
    return;
  default:
    return;
  }
}

function findRowByKey(rowKey) {
  return getClientRows(pageState.hosts).find((row) => row.key === rowKey) || null;
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
  sortedRows(filteredRows()).forEach((row) => toggleRowSelection(row.key, checked));
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
  runSelectedHostsButton.textContent = pageState.runningRows.size > 0 ? "Running..." : "Run selected";
  syncBulkModuleControls();
}

function selectedHostRows() {
  const rowMap = new Map(getClientRows(pageState.hosts).map((row) => [row.key, row]));
  return [...pageState.selectedRows]
    .map((key) => rowMap.get(key))
    .filter(Boolean);
}

function firstSelectedOnlineRow() {
  return selectedHostRows().find((row) => row.connected) || null;
}

function syncBulkModuleControls() {
  if (!bulkModuleSelect || !runSelectedModuleButton || !refreshBulkModulesButton) {
    return;
  }

  const selectedRows = selectedHostRows();
  const onlineRows = selectedRows.filter((row) => row.connected);
  const sourceRow = onlineRows[0] || null;
  if (bulkModulePanel?.open && sourceRow && sourceRow.key !== pageState.bulkModules.loadedRowKey && !pageState.bulkModules.loading && !pageState.bulkModules.running) {
    void loadBulkModules(true);
  }
  if (!onlineRows.length && pageState.bulkModules.items.length) {
    pageState.bulkModules.items = [];
    pageState.bulkModules.loadedRowKey = "";
    bulkModuleSelect.innerHTML = "";
    syncBulkModuleHelp();
  }

  const selectedModule = bulkModuleSelect.value.trim();
  const selectedManifest = pageState.bulkModules.items.find((module) => module.name === selectedModule);

  refreshBulkModulesButton.disabled = onlineRows.length === 0 || pageState.bulkModules.loading || pageState.bulkModules.running;
  bulkModuleSelect.disabled = pageState.bulkModules.loading || pageState.bulkModules.running || pageState.bulkModules.items.length === 0;
  runSelectedModuleButton.disabled = (
    selectedRows.length === 0 ||
    !selectedModule ||
    Boolean(selectedManifest?.disabled) ||
    pageState.bulkModules.loading ||
    pageState.bulkModules.running ||
    pageState.runningRows.size > 0
  );
  refreshBulkModulesButton.textContent = pageState.bulkModules.loading ? "Loading..." : "Refresh modules";
  runSelectedModuleButton.textContent = pageState.bulkModules.running ? "Running..." : "Run module";
}

async function loadBulkModules(force) {
  if (!bulkModuleSelect || !bulkModuleOutput) {
    return;
  }

  const row = firstSelectedOnlineRow();
  if (!row) {
    pageState.bulkModules.items = [];
    pageState.bulkModules.loadedRowKey = "";
    bulkModuleSelect.innerHTML = "";
    setBulkModuleOutput("Select at least one online host to load modules.");
    syncBulkModuleHelp();
    syncBulkModuleControls();
    return;
  }

  if (!force && pageState.bulkModules.loadedRowKey === row.key && pageState.bulkModules.items.length) {
    syncBulkModuleControls();
    return;
  }

  pageState.bulkModules.loading = true;
  pageState.bulkModules.error = "";
  setBulkModuleOutput(`Loading modules from ${row.hostname || row.stableId}...`);
  syncBulkModuleControls();

  try {
    const response = await api(withProjectQuery(`/api/hosts/${encodeURIComponent(row.stableId)}/modules?connectionId=${encodeURIComponent(row.connectionId || "")}`, pageState.ctx?.project || ""));
    pageState.bulkModules.items = visibleWebModules(response.items || []).map((module) => moduleForRow(module, row));
    pageState.bulkModules.loadedRowKey = row.key;
    renderBulkModuleOptions(row);
  } catch (error) {
    pageState.bulkModules.items = [];
    pageState.bulkModules.loadedRowKey = "";
    bulkModuleSelect.innerHTML = "";
    setBulkModuleOutput(error.message, "error");
  } finally {
    pageState.bulkModules.loading = false;
    syncBulkModuleHelp();
    syncBulkModuleControls();
  }
}

function renderBulkModuleOptions(sourceRow) {
  if (!bulkModuleSelect || !bulkModuleOutput) {
    return;
  }

  const modules = pageState.bulkModules.items;
  const previous = bulkModuleSelect.value;
  bulkModuleSelect.innerHTML = modules.map((module) => (
    `<option value="${escapeAttribute(module.name || "")}" ${module.disabled ? "disabled" : ""}>${escapeHtml(module.name || "unnamed")}</option>`
  )).join("");

  const selected = modules.find((module) => module.name === previous && !module.disabled) || modules.find((module) => !module.disabled) || modules[0] || null;
  if (selected) {
    bulkModuleSelect.value = selected.name;
  }

  setBulkModuleOutput(modules.length
    ? `Modules loaded from ${sourceRow.hostname || sourceRow.stableId}.`
    : "No runnable web modules reported by the selected host.");
}

function syncBulkModuleHelp() {
  if (!bulkModuleSelect || !bulkModuleHelpText) {
    return;
  }

  const module = pageState.bulkModules.items.find((item) => item.name === bulkModuleSelect.value) || null;
  const name = String(module?.name || "").toLowerCase();
  const help = MODULE_FORM_HELP[name] || {};
  bulkModuleHelpText.textContent = module ? (help.text || module.usage || "") : "";

  if (bulkModuleArgsInput) {
    bulkModuleArgsInput.placeholder = help.argsPlaceholder || "module arguments";
  }
  if (bulkModuleStdinInput) {
    bulkModuleStdinInput.placeholder = help.stdinPlaceholder || "optional stdin";
  }
}

function setBulkModuleOutput(message, type = "muted") {
  if (!bulkModuleOutput) {
    return;
  }
  bulkModuleOutput.textContent = message || "";
  bulkModuleOutput.classList.toggle("error-text", type === "error");
  bulkModuleOutput.classList.toggle("muted", type !== "error");
}

function visibleWebModules(modules) {
  return (modules || []).filter((module) => !HIDDEN_WEB_MODULES.has(String(module?.name || "").toLowerCase()));
}

function moduleForRow(module, row) {
  const platforms = (module?.platforms || []).map((platform) => String(platform || "").toLowerCase()).filter(Boolean);
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

async function runSelectedModuleOnHosts() {
  if (!bulkModuleSelect || !bulkModuleOutput) {
    return;
  }

  const selectedRows = selectedHostRows();
  const moduleName = bulkModuleSelect.value.trim();
  if (!selectedRows.length) {
    setBulkModuleOutput("Select at least one host first.", "error");
    return;
  }
  if (!moduleName) {
    setBulkModuleOutput("Choose a module to run.", "error");
    return;
  }
  if (HIDDEN_WEB_MODULES.has(moduleName.toLowerCase())) {
    setBulkModuleOutput("This transport helper module is hidden from the web runner.", "error");
    return;
  }

  const manifest = pageState.bulkModules.items.find((module) => module.name === moduleName) || null;
  if (!manifest) {
    setBulkModuleOutput("Refresh modules before running this module.", "error");
    return;
  }
  if (manifest.disabled) {
    setBulkModuleOutput(manifest.disabledReason || "This module is disabled by the selected host.", "error");
    return;
  }

  let args = [];
  try {
    args = parseModuleArgs(bulkModuleArgsInput?.value || "");
  } catch (error) {
    setBulkModuleOutput(error.message, "error");
    return;
  }

  let stdin = bulkModuleStdinInput?.value || "";
  let stdinBase64 = "";
  const stdinFile = bulkModuleStdinFileInput?.files?.[0] || null;
  if (stdinFile && stdin) {
    setBulkModuleOutput("Use either stdin text or stdin file, not both.", "error");
    return;
  }
  if (stdinFile) {
    if (stdinFile.size > moduleStdinLimitBytes(moduleName)) {
      setBulkModuleOutput(`Stdin file is too large. Maximum is ${moduleStdinLimitLabel(moduleName)}.`, "error");
      return;
    }
    try {
      stdinBase64 = await readFileAsBase64(stdinFile);
      stdin = "";
    } catch (error) {
      setBulkModuleOutput(error.message, "error");
      return;
    }
  } else if (stdin && textByteLength(stdin) > moduleStdinLimitBytes(moduleName)) {
    setBulkModuleOutput(`Stdin is too large. Maximum is ${moduleStdinLimitLabel(moduleName)}.`, "error");
    return;
  }

  const command = moduleCommandLabel(moduleName, args);
  const targetRows = [];
  selectedRows.forEach((row) => {
    const rowManifest = moduleForRow(manifest, row);
    pageState.commandResults.delete(row.key);
    if (rowManifest.disabled) {
      pageState.commandResults.set(row.key, {
        kind: "module",
        command,
        output: "",
        error: rowManifest.disabledReason || "This module is not available for this host.",
        timedOut: false,
        truncated: false
      });
      return;
    }
    targetRows.push(row);
  });

  if (!targetRows.length) {
    setBulkModuleOutput("No selected hosts can run this module.", "error");
    renderHosts();
    return;
  }

  pageState.bulkModules.running = true;
  targetRows.forEach((row) => pageState.runningRows.add(row.key));
  setBulkModuleOutput(`Running ${moduleName} on ${targetRows.length} host${targetRows.length === 1 ? "" : "s"}...`);
  renderHosts();

  try {
    const response = await api(withProjectQuery(`/api/hosts/modules/${encodeURIComponent(moduleName)}/run`, pageState.ctx?.project || ""), {
      method: "POST",
      body: JSON.stringify({
        targets: targetRows.map((row) => ({
          stableId: row.stableId,
          connectionId: row.connectionId || ""
        })),
        args,
        stdin,
        stdinBase64,
        timeoutSeconds: numberFieldValue(bulkModuleTimeoutInput, 60),
        outputLimitBytes: numberFieldValue(bulkModuleOutputLimitInput, 1024 * 1024)
      })
    });

    targetRows.forEach((row) => pageState.runningRows.delete(row.key));
    for (const item of response.items || []) {
      const key = item.key || `${item.stableId}:${item.connectionId || "offline"}`;
      pageState.commandResults.set(key, {
        kind: "module",
        command,
        output: item.output || "",
        error: item.error || "",
        timedOut: Boolean(item.timedOut),
        truncated: Boolean(item.truncated)
      });
    }
    setBulkModuleOutput(`Module ${moduleName} completed on ${targetRows.length} host${targetRows.length === 1 ? "" : "s"}.`);
  } catch (error) {
    targetRows.forEach((row) => pageState.runningRows.delete(row.key));
    setBulkModuleOutput(error.message, "error");
  } finally {
    pageState.bulkModules.running = false;
    renderHosts();
  }
}

function moduleCommandLabel(moduleName, args) {
  const suffix = args.length ? ` ${args.map((arg) => quoteModuleArg(arg)).join(" ")}` : "";
  return `${moduleName}${suffix}`;
}

function quoteModuleArg(value) {
  const arg = String(value || "");
  if (!arg || /\s/.test(arg)) {
    return `"${arg.replaceAll("\\", "\\\\").replaceAll("\"", "\\\"")}"`;
  }
  return arg;
}

function moduleStdinLimitBytes(moduleName) {
  const module = (pageState.bulkModules.items || []).find((item) => item.name === moduleName);
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
  requestDeleteHost({ stableId, hostname, connected });
}

function requestDeleteHost(row) {
  if (!row?.connected) {
    openOfflineDeleteModal(row.stableId, row.hostname || row.stableId);
    return;
  }

  openOnlineDeleteModal(row.stableId, row.hostname || row.stableId);
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
  deleteOnlyOfflineButton.textContent = "Delete only this client";
  deleteAllOfflineButton.textContent = "Delete all offline clients";
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
  actionButton.textContent = mode === "all" ? "Deleting offline clients..." : "Deleting client...";

  try {
    if (mode === "all") {
      await deleteAllOfflineHostRecords();
    } else {
      await deleteHostRecords(stableIds);
    }
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

async function deleteAllOfflineHostRecords() {
  return api(withProjectQuery("/api/hosts/offline", pageState.ctx?.project || ""), {
    method: "DELETE"
  });
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
  confirmOnlineDeleteButton.textContent = "Delete client";
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
  confirmOnlineDeleteButton.textContent = "Deleting client...";

  try {
    await deleteHostRecords([pending.stableId]);
    closeOnlineDeleteModal(true);
    await refreshHostsAfterMutation();
  } catch (error) {
    onlineDeleteError.textContent = error.message;
    confirmOnlineDeleteButton.textContent = "Delete client";
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

async function navigateFilesystemPath(event) {
  event.preventDefault();

  const path = normalizeFilesystemPath(filesystemPathInput.value);
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
  filesystemPathInput.value = normalizeFilesystemPath(pageState.filesystem.selectedDirectory || "/");
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
