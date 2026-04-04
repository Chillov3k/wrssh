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
  rowMatchesQuery
} from "./shared.js";

const hostSearch = document.getElementById("hostSearch");
const hostTable = document.getElementById("hostTable");

const pageState = {
  hosts: [],
  systemOptions: null,
  ctx: null
};

hostSearch.addEventListener("input", renderHosts);

hostTable.addEventListener("click", async (event) => {
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
  if (!row || event.target.closest("a,button")) {
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
  const query = hostSearch.value.trim().toLowerCase();
  const rows = getClientRows(pageState.hosts).filter((row) => rowMatchesQuery(row, query));

  if (!rows.length) {
    hostTable.innerHTML = `<div class="empty-state"><p>No clients matched the current filter.</p></div>`;
    return;
  }

  const jumpTarget = buildJumpTarget(pageState.systemOptions, rows[0]?.host || null, pageState.ctx?.user);

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
          <span>ID</span>
          <span>Host</span>
          <span>Network</span>
          <span>Platform</span>
          <span>Timestamps</span>
          <span>Status</span>
          <span>Actions</span>
        </div>
        <div class="client-table-body">
          ${rows.map((row) => renderRow(row, jumpTarget)).join("")}
        </div>
      </div>
    </div>
  `;
}

function renderRow(row, jumpTarget) {
  const command = hostCommandTemplates(row.host, row.connection, jumpTarget).ssh;
  const href = hostPageHref(row.stableId, row.connectionId, pageState.ctx?.project || "");
  const deleteLabel = row.connected ? "Delete Client" : "Delete Offline Client";

  return `
    <article class="client-row client-row-hosts" data-open-shell="${escapeAttribute(href)}">
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
      <div class="client-cell client-actions">
        <a class="action-button action-link" href="${escapeAttribute(href)}">Open Shell</a>
        <button class="ghost-button" data-copy-command="${escapeAttribute(command)}">Copy Connect Command</button>
        <button class="danger-button" data-delete-host="${escapeAttribute(row.stableId)}" data-hostname="${escapeAttribute(row.hostname)}" data-connected="${row.connected ? "true" : "false"}">${escapeHtml(deleteLabel)}</button>
      </div>
    </article>
  `;
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
