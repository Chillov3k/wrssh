import {
  api,
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
  markSynced,
  platformLabel,
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
  detailHostId: document.getElementById("detailHostId"),
  detailStableId: document.getElementById("detailStableId"),
  detailIp: document.getElementById("detailIp"),
  detailPlatform: document.getElementById("detailPlatform"),
  detailAdded: document.getElementById("detailAdded"),
  detailActivity: document.getElementById("detailActivity"),
  detailConnection: document.getElementById("detailConnection"),
  detailProjectValue: document.getElementById("detailProjectValue"),
  detailTagsValue: document.getElementById("detailTagsValue"),
  selectedConnectionSummary: document.getElementById("selectedConnectionSummary"),
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
  hostMetadataOutput: document.getElementById("hostMetadataOutput")
};

const terminalView = new TerminalView(elements.terminalViewport, elements.terminalOutput);

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
  const jumpTarget = buildJumpTarget(pageState.systemOptions, host, pageState.ctx?.user);
  const commands = currentCommands(connection);
  const platform = platformLabel(connection?.version || host.version);
  const currentTarget = connection?.connectionId || "offline";

  elements.hostDetailEmpty.classList.add("hidden");
  elements.hostDetailContent.classList.remove("hidden");

  elements.detailHostname.textContent = host.hostname || connection?.hostname || host.preferredAlias || host.stableId;
  elements.detailHostnameMeta.textContent = host.hostname || connection?.hostname || host.preferredAlias || "-";
  elements.detailSubline.textContent = `${host.project || "Unassigned"} · ${host.comment || "no comment"} · ${connection?.version || host.version || "unknown version"}`;
  elements.detailStatusBadge.textContent = connection ? "Online" : "Offline";
  elements.detailStatusBadge.className = `status-pill ${connection ? "online" : "offline"}`;
  elements.detailHostId.textContent = connection?.connectionId || host.hostId || "-";
  elements.detailStableId.textContent = host.stableId;
  elements.detailIp.textContent = connection?.remoteIp || host.ip || host.remoteAddr || "-";
  elements.detailPlatform.textContent = platform;
  elements.detailAdded.textContent = formatDate(host.dateAdded);
  elements.detailActivity.textContent = formatDate(host.lastActivityAt);
  elements.detailConnection.textContent = formatDate(host.lastConnectionAt || host.lastDisconnectAt);
  elements.detailProjectValue.textContent = host.project || "Unassigned";
  elements.detailTagsValue.textContent = host.tags?.join(", ") || "-";
  renderHostMetadataForm();

  elements.openTerminalButton.disabled = !connection;
  elements.sendCtrlCButton.disabled = !pageState.terminal.socket;
  elements.closeTerminalButton.disabled = !pageState.terminal.socket;
  elements.openTerminalButton.textContent = pageState.terminal.connectionId === connection?.connectionId ? "Reconnect Terminal" : "Open Terminal";
  elements.terminalTitle.textContent = `${host.hostname || host.preferredAlias || host.stableId} · ${currentTarget}`;
  elements.selectedConnectionSummary.textContent = connection
    ? `Bound to ${connection.connectionId} · ${connection.remoteAddr || connection.remoteIp || "-"} · ssh -J ${jumpTarget} ${connection.connectionId}`
    : "The selected client is currently offline. Open a live connection from Hosts to attach the shell.";

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

function currentCommands(connection = currentConnection()) {
  return hostCommandTemplates(pageState.host, connection, buildJumpTarget(pageState.systemOptions, pageState.host, pageState.ctx?.user));
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
