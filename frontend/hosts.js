import {
  api,
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

const OS_FILTERS = [
  { key: "all", label: "All" },
  { key: "linux", label: "Linux" },
  { key: "windows", label: "Windows" },
  { key: "darwin", label: "macOS" }
];

const pageState = {
  hosts: [],
  systemOptions: null,
  ctx: null,
  osFilter: "all",
  selectedRows: new Set(),
  runningRows: new Set(),
  commandResults: new Map()
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
  const confirmText = connected
    ? `Delete ${hostname}? Active client connections will receive a kill request and the host record will be removed from the web inventory.`
    : `Delete offline host ${hostname} from the web inventory?`;

  if (!window.confirm(confirmText)) {
    return;
  }

  const original = button.textContent;
  button.disabled = true;
  button.textContent = "Deleting...";

  try {
    await api(`/api/hosts/${encodeURIComponent(stableId)}`, {
      method: "DELETE"
    });

    const [hosts, systemOptions] = await Promise.all([loadHosts(), loadSystemOptions()]);
    pageState.hosts = hosts;
    pageState.systemOptions = systemOptions;
    renderHosts();

    if (pageState.ctx) {
      const metrics = hostMetrics(hosts);
      markSynced(pageState.ctx, `${pageState.ctx.project} · ${metrics.activeConnections} online / ${metrics.uniqueHosts} unique hosts`);
    }
  } catch (error) {
    button.disabled = false;
    button.textContent = original;
    window.alert(error.message);
  }
}
