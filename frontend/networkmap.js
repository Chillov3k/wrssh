import {
  api,
  byId,
  copyText,
  escapeAttribute,
  escapeHtml,
  hostPageHref,
  initPage,
  loadHosts,
  markSynced,
  osIconPath,
  withProjectQuery
} from "./shared.js";

const mapCanvas = byId("mapCanvas");
const mapViewport = byId("mapViewport");
const mapGroups = byId("mapGroups");
const mapEdges = byId("mapEdges");
const mapNodes = byId("mapNodes");
const mapNotes = byId("mapNotes");
const mapEmptyState = byId("mapEmptyState");
const fitMapButton = byId("fitMapButton");
const groupFilter = byId("groupFilter");
const addNoteButton = byId("addNoteButton");
const zoomInButton = byId("zoomInButton");
const zoomOutButton = byId("zoomOutButton");
const mapNodePopover = byId("mapNodePopover");
const popoverTitle = byId("popoverTitle");
const popoverMeta = byId("popoverMeta");
const popoverPorts = byId("popoverPorts");
const popoverCopyButton = byId("popoverCopyButton");
const scanHostSelect = byId("scanHostSelect");
const scanTargetsInput = byId("scanTargetsInput");
const scanPortsInput = byId("scanPortsInput");
const scanTimeoutInput = byId("scanTimeoutInput");
const runScanButton = byId("runScanButton");
const clearMapButton = byId("clearMapButton");
const scanOutput = byId("scanOutput");
const scanStateChip = byId("scanStateChip");
const clearMapOverlay = byId("clearMapOverlay");
const confirmClearMapButton = byId("confirmClearMapButton");
const cancelClearMapButton = byId("cancelClearMapButton");
const clearMapError = byId("clearMapError");

// Monitor icon geometry. The node group origin (0,0) sits between the monitor
// base and the text block; the icon spans y -52..4.
const NODE_ICON_WIDTH = 66;
const NODE_ICON_HEIGHT = 56;
const NODE_ICON_CENTER_Y = -32;
const NODE_HIT_WIDTH = 140;
const NODE_HIT_HEIGHT = 128;
const EDGE_ENDPOINT_MARGIN = 42;
const OS_ICON_SIZE = 26;

const GROUP_PAD_X = 18;
const GROUP_PAD_TOP = 46;
const GROUP_PAD_BOTTOM = 14;
// Visual extents of one node around its position point (monitor plus labels).
const NODE_EXTENT_HALF_W = 72;
const NODE_EXTENT_TOP = -58;
const NODE_EXTENT_BOTTOM = 58;
const GROUP_CELL_WIDTH = 170;
const GROUP_CELL_HEIGHT = 150;
const GROUP_COLUMNS = 4;
const GROUP_WRAP_WIDTH = 2000;
const GROUP_GAP = 90;

const NOTE_DEFAULT_W = 170;
const NOTE_DEFAULT_H = 70;
const NOTE_DEFAULT_FONT = 13;
const NOTE_MIN_W = 90;
const NOTE_MIN_H = 54;
const NOTE_SAVE_DEBOUNCE_MS = 600;
const POPOVER_HIDE_DELAY_MS = 180;

const AUTO_REFRESH_MS = 10000;
const ZOOM_MIN = 0.25;
const ZOOM_MAX = 2.6;

const GROUP_STORAGE_KEY = "wrssh.mapGrouping";
const GROUP_MODES = new Set(["subnet", "domain", "off"]);

const pageState = {
  ctx: null,
  hosts: [],
  map: { nodes: [], edges: [], notes: [], summary: { captured: 0, online: 0, discovered: 0 } },
  positions: new Map(),
  grouping: readStoredGrouping(),
  transform: { x: 0, y: 0, k: 1 },
  fitApplied: false,
  hoveredIP: "",
  drag: null,
  pan: null,
  noteDrag: null,
  editingNoteID: "",
  popoverNodeIP: "",
  popoverHideTimer: null,
  scanning: false,
  clearPending: false,
  noteSaveTimers: new Map(),
  autoRefreshTimer: null
};

mapCanvas.addEventListener("wheel", (event) => {
  event.preventDefault();
  hideNodePopover();
  const factor = event.deltaY < 0 ? 1.12 : 1 / 1.12;
  zoomAt(event.offsetX, event.offsetY, factor);
}, { passive: false });

mapCanvas.addEventListener("pointerdown", (event) => {
  const nodeGroup = event.target.closest("[data-map-node]");
  if (nodeGroup && !event.button) {
    hideNodePopover();
    startNodeDrag(nodeGroup, event);
    return;
  }
  if (!event.button) {
    hideNodePopover();
    startPan(event);
  }
});

window.addEventListener("pointermove", (event) => {
  if (pageState.noteDrag) {
    moveNoteDrag(event);
    return;
  }
  if (pageState.drag) {
    moveNodeDrag(event);
    return;
  }
  if (pageState.pan) {
    movePan(event);
  }
});

window.addEventListener("pointerup", () => {
  endNodeDrag();
  endPan();
  endNoteDrag();
});

mapNodes.addEventListener("dblclick", (event) => {
  const nodeGroup = event.target.closest("[data-map-node]");
  if (!nodeGroup) {
    return;
  }
  const node = findNode(nodeGroup.dataset.mapNode || "");
  if (node?.captured && node.stableId) {
    window.location.href = hostPageHref(node.stableId, "", pageState.ctx?.project || "");
  }
});

mapNodes.addEventListener("pointerover", (event) => {
  const nodeGroup = event.target.closest("[data-map-node]");
  if (!nodeGroup) {
    return;
  }
  pageState.hoveredIP = nodeGroup.dataset.mapNode || "";
  syncEdgeHighlight();
  showNodePopover(pageState.hoveredIP);
});

mapNodes.addEventListener("pointerout", (event) => {
  const nodeGroup = event.target.closest("[data-map-node]");
  if (!nodeGroup) {
    return;
  }
  const related = event.relatedTarget;
  if (related && nodeGroup.contains(related)) {
    return;
  }
  if (related && mapNodePopover.contains(related)) {
    return;
  }
  pageState.hoveredIP = "";
  syncEdgeHighlight();
  scheduleHideNodePopover();
});

mapNodePopover.addEventListener("pointerenter", () => {
  if (pageState.popoverHideTimer) {
    window.clearTimeout(pageState.popoverHideTimer);
    pageState.popoverHideTimer = null;
  }
});

mapNodePopover.addEventListener("pointerleave", () => {
  scheduleHideNodePopover();
});

popoverCopyButton.addEventListener("click", async () => {
  const node = findNode(pageState.popoverNodeIP);
  if (!node) {
    return;
  }
  try {
    await copyText(nodePortsText(node));
    popoverCopyButton.textContent = "Copied";
    window.setTimeout(() => {
      popoverCopyButton.textContent = "Copy";
    }, 900);
  } catch (error) {
    popoverCopyButton.textContent = "Copy failed";
    window.setTimeout(() => {
      popoverCopyButton.textContent = "Copy";
    }, 900);
  }
});

mapNotes.addEventListener("pointerdown", (event) => {
  const noteEl = event.target.closest("[data-map-note]");
  if (!noteEl || event.button) {
    return;
  }
  if (event.target.closest("button")) {
    return;
  }
  startNoteDrag(noteEl, event);
});

mapNotes.addEventListener("contextmenu", (event) => {
  const noteEl = event.target.closest("[data-map-note]");
  if (!noteEl) {
    return;
  }
  event.preventDefault();
  enterNoteEdit(noteEl);
});

mapNotes.addEventListener("click", (event) => {
  const fontButton = event.target.closest("[data-note-font]");
  if (fontButton) {
    adjustNoteFont(fontButton.closest("[data-map-note]"), Number(fontButton.dataset.noteFont));
    return;
  }
  const deleteButton = event.target.closest("[data-note-delete]");
  if (deleteButton) {
    void deleteNote(deleteButton.closest("[data-map-note]"));
  }
});

document.addEventListener("pointerdown", (event) => {
  if (!pageState.editingNoteID) {
    return;
  }
  const noteEl = event.target.closest("[data-map-note]");
  if (noteEl && noteEl.dataset.mapNote === pageState.editingNoteID) {
    return;
  }
  exitNoteEdit();
});

mapNotes.addEventListener("input", (event) => {
  const textEl = event.target.closest(".map-note-text");
  if (!textEl) {
    return;
  }
  scheduleNoteSave(textEl.closest("[data-map-note]"));
});

fitMapButton.addEventListener("click", () => {
  fitMap(true);
});

groupFilter.addEventListener("click", (event) => {
  const button = event.target.closest("[data-group-mode]");
  if (!button) {
    return;
  }
  setGrouping(button.dataset.groupMode || "subnet");
});

addNoteButton.addEventListener("click", () => {
  void createNote();
});

zoomInButton.addEventListener("click", () => {
  zoomAroundCenter(1.25);
});
zoomOutButton.addEventListener("click", () => {
  zoomAroundCenter(0.8);
});

runScanButton.addEventListener("click", () => {
  void runScan();
});
scanTargetsInput.addEventListener("keydown", (event) => {
  if (event.key === "Enter") {
    event.preventDefault();
    void runScan();
  }
});
clearMapButton.addEventListener("click", openClearMapModal);
cancelClearMapButton.addEventListener("click", closeClearMapModal);
clearMapOverlay.addEventListener("click", (event) => {
  if (event.target === clearMapOverlay) {
    closeClearMapModal();
  }
});
confirmClearMapButton.addEventListener("click", () => {
  void clearMapData();
});

document.addEventListener("keydown", (event) => {
  if (event.key === "Escape" && clearMapOverlay.classList.contains("visible")) {
    closeClearMapModal();
  }
  if (event.key === "Escape" && pageState.editingNoteID) {
    exitNoteEdit();
  }
});

initPage({
  title: "Network Map",
  requireProject: true,
  load: async (ctx) => {
    pageState.ctx = ctx;
    syncGroupFilterButtons();
    await refreshAll(false);
    startAutoRefresh();
  }
}).catch((error) => {
  console.error(error);
});

// ---------------------------------------------------------------------------
// Data loading
// ---------------------------------------------------------------------------

async function refreshAll(keepView = true) {
  const [hosts, map] = await Promise.all([
    loadHosts(),
    api(withProjectQuery("/api/network-map", pageState.ctx?.project || ""))
  ]);
  pageState.hosts = hosts;
  pageState.map = normalizeMapResponse(map);
  renderScanHostOptions();
  renderMap(keepView);
  renderNotes();
  markSynced(pageState.ctx, mapSummaryText());
}

function normalizeMapResponse(payload) {
  const map = {
    nodes: Array.isArray(payload?.nodes) ? payload.nodes : [],
    edges: Array.isArray(payload?.edges) ? payload.edges : [],
    notes: Array.isArray(payload?.notes) ? payload.notes : [],
    summary: payload?.summary || { captured: 0, online: 0, discovered: 0 }
  };
  map.nodes.sort((left, right) => (left.ip || "").localeCompare(right.ip || ""));
  return map;
}

function mapSummaryText() {
  const summary = pageState.map.summary || {};
  return `${summary.captured || 0} captured · ${summary.online || 0} online`;
}

function findNode(ip) {
  return pageState.map.nodes.find((node) => node.ip === ip) || null;
}

function renderScanHostOptions() {
  const onlineHosts = clientRows().filter((row) => row.connected);
  const previous = scanHostSelect.value;
  scanHostSelect.innerHTML = onlineHosts.length
    ? onlineHosts.map((row) => (
      `<option value="${escapeAttribute(`${row.stableId}|${row.connectionId || ""}`)}">${escapeHtml(row.hostname || row.stableId)}</option>`
    )).join("")
    : `<option value="">No online hosts</option>`;

  if (previous && [...scanHostSelect.options].some((option) => option.value === previous)) {
    scanHostSelect.value = previous;
  }
}

function clientRows() {
  const rows = [];
  (pageState.hosts || []).forEach((host) => {
    const active = host.activeConnections || [];
    if (!active.length) {
      rows.push({
        stableId: host.stableId,
        connectionId: "",
        hostname: host.hostname || host.stableId,
        connected: false
      });
      return;
    }
    active.forEach((connection) => {
      rows.push({
        stableId: host.stableId,
        connectionId: connection.connectionId || "",
        hostname: connection.hostname || host.hostname || host.stableId,
        connected: true
      });
    });
  });
  return rows;
}

// ---------------------------------------------------------------------------
// Graph rendering
// ---------------------------------------------------------------------------

function renderMap(keepView = true) {
  const nodes = pageState.map.nodes;
  mapEmptyState.classList.toggle("hidden", nodes.length > 0 || pageState.map.notes.length > 0);
  hideNodePopover();

  computeLayout(keepView);

  renderGroups();
  renderEdges();
  renderNodes();
  syncEdgeHighlight();
  applyTransform();
  if (!pageState.fitApplied && nodes.length) {
    fitMap(false);
    pageState.fitApplied = true;
  }
}

function renderNodes() {
  const fragment = document.createDocumentFragment();
  pageState.map.nodes.forEach((node) => {
    fragment.appendChild(renderNode(node));
  });
  mapNodes.replaceChildren(fragment);
}

function renderEdges() {
  const fragment = document.createDocumentFragment();
  pageState.map.edges.forEach((edge) => {
    const line = document.createElementNS("http://www.w3.org/2000/svg", "line");
    const from = pageState.positions.get(edge.from);
    const to = pageState.positions.get(edge.to);
    if (!from || !to) {
      return;
    }
    const endpoints = edgeEndpoints(from, to);
    line.setAttribute("x1", String(endpoints.x1));
    line.setAttribute("y1", String(endpoints.y1));
    line.setAttribute("x2", String(endpoints.x2));
    line.setAttribute("y2", String(endpoints.y2));
    line.setAttribute("class", "map-edge");
    line.dataset.from = edge.from;
    line.dataset.to = edge.to;
    const targetNode = findNode(edge.to);
    if (targetNode?.online) {
      line.setAttribute("marker-end", "url(#mapArrowHeadActive)");
    } else {
      line.setAttribute("marker-end", "url(#mapArrowHead)");
    }
    fragment.appendChild(line);
  });
  mapEdges.replaceChildren(fragment);
}

function renderNode(node) {
  const group = document.createElementNS("http://www.w3.org/2000/svg", "g");
  const position = pageState.positions.get(node.ip) || { x: 0, y: 0 };
  group.setAttribute("transform", `translate(${position.x},${position.y})`);
  group.setAttribute("class", nodeClass(node));
  group.dataset.mapNode = node.ip;

  // Monitor: screen sits on a stand neck and a base, all connected.
  // Origin (0,0) is just below the base; screen spans y -52..-12.
  group.innerHTML = `
    <rect class="map-node-hit" x="${-NODE_HIT_WIDTH / 2}" y="${-NODE_HIT_HEIGHT + 24}" width="${NODE_HIT_WIDTH}" height="${NODE_HIT_HEIGHT}"></rect>
    <g class="map-node-device">
      <rect class="map-node-screen" x="${-NODE_ICON_WIDTH / 2}" y="-52" width="${NODE_ICON_WIDTH}" height="40" rx="6"></rect>
      <rect class="map-node-screen-inner" x="${-NODE_ICON_WIDTH / 2 + 4}" y="-48" width="${NODE_ICON_WIDTH - 8}" height="32" rx="3"></rect>
      <rect class="map-node-stand" x="-4" y="-12" width="8" height="10" rx="1"></rect>
      <rect class="map-node-base" x="-16" y="-2" width="32" height="6" rx="2"></rect>
    </g>
    <g class="map-node-os" transform="translate(${-OS_ICON_SIZE / 2},${NODE_ICON_CENTER_Y - OS_ICON_SIZE / 2}) scale(${OS_ICON_SIZE / 24})"><path class="map-node-os-path" d="${osIconPath(node.os)}"></path></g>
    <text class="map-node-name" y="22" text-anchor="middle">${escapeHtml(nodeLabel(node))}</text>
    <text class="map-node-user" y="36" text-anchor="middle">${escapeHtml(userLabel(node))}</text>
    <text class="map-node-ip" y="50" text-anchor="middle">${escapeHtml(node.ip)}</text>
  `;

  if (node.online) {
    const pulse = document.createElementNS("http://www.w3.org/2000/svg", "circle");
    pulse.setAttribute("class", "map-node-pulse");
    pulse.setAttribute("cx", String(NODE_ICON_WIDTH / 2 - 8));
    pulse.setAttribute("cy", "-46");
    pulse.setAttribute("r", "4");
    group.appendChild(pulse);
  }

  return group;
}

function nodeClass(node) {
  if (node.captured) {
    return `map-node ${node.online ? "is-online" : "is-offline"}`;
  }
  return "map-node is-discovered";
}

function nodeLabel(node) {
  return node.hostname || node.ip;
}

function userLabel(node) {
  if (!node.captured) {
    return "not captured";
  }
  return node.user || "unknown user";
}

function edgeEndpoints(from, to) {
  const dx = to.x - from.x;
  const dy = to.y - from.y;
  const distance = Math.hypot(dx, dy) || 1;
  const offsetX = (dx / distance) * EDGE_ENDPOINT_MARGIN;
  const offsetY = (dy / distance) * EDGE_ENDPOINT_MARGIN;
  return {
    x1: from.x + offsetX,
    y1: from.y + NODE_ICON_CENTER_Y + offsetY,
    x2: to.x - offsetX,
    y2: to.y + NODE_ICON_CENTER_Y - offsetY
  };
}

function syncEdgeHighlight() {
  const hovered = pageState.hoveredIP;
  mapEdges.querySelectorAll(".map-edge").forEach((edge) => {
    const active = hovered && (edge.dataset.from === hovered || edge.dataset.to === hovered);
    edge.classList.toggle("is-highlighted", Boolean(active));
  });
  mapNodes.querySelectorAll("[data-map-node]").forEach((node) => {
    node.classList.toggle("is-hovered", Boolean(hovered && node.dataset.mapNode === hovered));
  });
}

// nodePortsText renders every known open port of a node, one per line, for
// the hover popover and the copy button.
function nodePortsText(node) {
  if (!node.ports || !node.ports.length) {
    return "";
  }
  return [...node.ports]
    .sort((left, right) => left.port - right.port)
    .map((port) => `${port.port}/${port.protocol || "tcp"}`)
    .join("\n");
}

// ---------------------------------------------------------------------------
// Node hover popover
// ---------------------------------------------------------------------------

function showNodePopover(ip) {
  const node = findNode(ip);
  if (!node) {
    hideNodePopover();
    return;
  }
  if (pageState.popoverHideTimer) {
    window.clearTimeout(pageState.popoverHideTimer);
    pageState.popoverHideTimer = null;
  }

  const nodeGroup = mapNodes.querySelector(`[data-map-node="${cssEscape(ip)}"]`);
  if (!nodeGroup) {
    hideNodePopover();
    return;
  }

  if (pageState.popoverNodeIP !== ip) {
    pageState.popoverNodeIP = ip;
    popoverTitle.textContent = node.hostname || node.ip;
    const metaParts = [];
    if (node.captured) {
      metaParts.push(node.user || "unknown user");
    } else {
      metaParts.push("not captured");
    }
    metaParts.push(node.os ? node.os : "unknown os");
    metaParts.push(node.ip);
    popoverMeta.textContent = metaParts.join(" · ");
    const ports = nodePortsText(node);
    popoverPorts.textContent = ports || "no open ports recorded";
    popoverCopyButton.disabled = !ports;
  }

  positionNodePopover(nodeGroup);
  mapNodePopover.classList.remove("hidden");
}

function positionNodePopover(nodeGroup) {
  const shell = mapNodePopover.parentElement.getBoundingClientRect();
  const nodeRect = nodeGroup.getBoundingClientRect();

  mapNodePopover.classList.remove("hidden");
  const popoverWidth = mapNodePopover.offsetWidth;
  const popoverHeight = mapNodePopover.offsetHeight;

  let left = nodeRect.right - shell.left + 12;
  let top = nodeRect.top - shell.top + nodeRect.height / 2 - popoverHeight / 2;
  if (left + popoverWidth > shell.width - 8) {
    left = Math.max(8, nodeRect.left - shell.left - popoverWidth - 12);
  }
  left = Math.min(Math.max(left, 8), Math.max(8, shell.width - popoverWidth - 8));
  top = Math.min(Math.max(top, 8), Math.max(8, shell.height - popoverHeight - 8));

  mapNodePopover.style.left = `${left}px`;
  mapNodePopover.style.top = `${top}px`;
}

function scheduleHideNodePopover() {
  if (pageState.popoverHideTimer) {
    window.clearTimeout(pageState.popoverHideTimer);
  }
  pageState.popoverHideTimer = window.setTimeout(() => {
    pageState.popoverHideTimer = null;
    hideNodePopover();
  }, POPOVER_HIDE_DELAY_MS);
}

function hideNodePopover() {
  if (pageState.popoverHideTimer) {
    window.clearTimeout(pageState.popoverHideTimer);
    pageState.popoverHideTimer = null;
  }
  pageState.popoverNodeIP = "";
  mapNodePopover.classList.add("hidden");
}

// ---------------------------------------------------------------------------
// Grouping
// ---------------------------------------------------------------------------

function readStoredGrouping() {
  try {
    const stored = window.localStorage.getItem(GROUP_STORAGE_KEY);
    if (GROUP_MODES.has(stored)) {
      return stored;
    }
  } catch (error) {
    console.warn("grouping storage unavailable", error);
  }
  return "subnet";
}

function setGrouping(mode) {
  if (!GROUP_MODES.has(mode) || mode === pageState.grouping) {
    return;
  }
  pageState.grouping = mode;
  try {
    window.localStorage.setItem(GROUP_STORAGE_KEY, mode);
  } catch (error) {
    console.warn("grouping storage unavailable", error);
  }
  syncGroupFilterButtons();
  hideNodePopover();
  computeLayout(false);
  renderGroups();
  renderEdges();
  renderNodes();
  syncEdgeHighlight();
  fitMap(true);
}

function syncGroupFilterButtons() {
  groupFilter.querySelectorAll("[data-group-mode]").forEach((button) => {
    button.classList.toggle("is-active", button.dataset.groupMode === pageState.grouping);
  });
}

// subnetKey groups IPv4 nodes by their /24 network.
function subnetKey(ip) {
  const parts = String(ip || "").split(".");
  if (parts.length === 4 && parts.every((part) => /^\d+$/.test(part))) {
    return `${parts[0]}.${parts[1]}.${parts[2]}.0/24`;
  }
  return "IPv6";
}

// domainKey groups domain-joined hosts (windows domains, freeipa) by their
// DNS zone: the last three labels of the full hostname, which drops the
// rssh username and the computer label, e.g.
// "john.pc1.office.corp.local" -> "office.corp.local".
function domainKey(node) {
  const labels = String(node.hostname || "").split(".").filter(Boolean);
  if (labels.length < 3) {
    return "";
  }
  return labels.slice(-3).join(".");
}

function nodeGroupKey(node) {
  if (pageState.grouping === "subnet") {
    return subnetKey(node.ip);
  }
  if (pageState.grouping === "domain") {
    return domainKey(node);
  }
  return "";
}

function renderGroups() {
  const groups = collectGroups();
  const fragment = document.createDocumentFragment();

  groups.forEach((group) => {
    const box = groupBoundingBox(group.nodes);
    if (!box) {
      return;
    }

    const rect = document.createElementNS("http://www.w3.org/2000/svg", "rect");
    rect.setAttribute("class", "map-group-rect");
    rect.setAttribute("x", String(box.minX));
    rect.setAttribute("y", String(box.minY));
    rect.setAttribute("width", String(box.maxX - box.minX));
    rect.setAttribute("height", String(box.maxY - box.minY));
    rect.setAttribute("rx", "14");
    fragment.appendChild(rect);

    const label = document.createElementNS("http://www.w3.org/2000/svg", "text");
    label.setAttribute("class", "map-group-label");
    label.setAttribute("x", String(box.minX + 14));
    label.setAttribute("y", String(box.minY + 20));
    label.textContent = `${group.key} · ${group.nodes.length}`;
    fragment.appendChild(label);
  });

  mapGroups.replaceChildren(fragment);
}

function collectGroups() {
  const groups = [];
  const byKey = new Map();
  pageState.map.nodes.forEach((node) => {
    const key = nodeGroupKey(node);
    if (!key) {
      return;
    }
    let group = byKey.get(key);
    if (!group) {
      group = { key, nodes: [] };
      byKey.set(key, group);
      groups.push(group);
    }
    group.nodes.push(node);
  });
  groups.sort((left, right) => left.key.localeCompare(right.key));
  return groups;
}

function groupBoundingBox(nodes) {
  let minX = Infinity;
  let minY = Infinity;
  let maxX = -Infinity;
  let maxY = -Infinity;
  nodes.forEach((node) => {
    const position = pageState.positions.get(node.ip);
    if (!position) {
      return;
    }
    minX = Math.min(minX, position.x - NODE_EXTENT_HALF_W);
    maxX = Math.max(maxX, position.x + NODE_EXTENT_HALF_W);
    minY = Math.min(minY, position.y + NODE_EXTENT_TOP);
    maxY = Math.max(maxY, position.y + NODE_EXTENT_BOTTOM);
  });
  if (minX === Infinity) {
    return null;
  }
  return {
    minX: minX - GROUP_PAD_X,
    maxX: maxX + GROUP_PAD_X,
    minY: minY - GROUP_PAD_TOP,
    maxY: maxY + GROUP_PAD_BOTTOM
  };
}

// ---------------------------------------------------------------------------
// Layout
// ---------------------------------------------------------------------------

// computeLayout arranges nodes in a structured grid per group so the map
// reads as tidy rectangles instead of a random scatter. Positions of known
// nodes are preserved across refreshes; a full re-layout (mode switch or
// first render) rebuilds the grid.
function computeLayout(keepView) {
  const nodes = pageState.map.nodes;
  if (!nodes.length) {
    pageState.positions = new Map();
    return;
  }

  if (!keepView) {
    pageState.positions = structuredLayout(nodes);
    return;
  }

  const positions = new Map(pageState.positions);
  const groups = collectGroups();

  // New nodes join their group next to existing members.
  nodes.forEach((node) => {
    if (positions.has(node.ip)) {
      return;
    }
    const group = groups.find((item) => item.nodes.some((candidate) => candidate.ip === node.ip));
    if (!group) {
      positions.set(node.ip, { x: (positions.size % 5) * GROUP_CELL_WIDTH, y: Math.floor(positions.size / 5) * GROUP_CELL_HEIGHT });
      return;
    }
    const anchor = groupAnchor(group, positions);
    const index = group.nodes.findIndex((candidate) => candidate.ip === node.ip);
    const ring = 1 + Math.floor(index / 6);
    const angle = (Math.PI / 3) * (index % 6);
    positions.set(node.ip, {
      x: anchor.x + Math.cos(angle) * GROUP_CELL_WIDTH * 0.7 * ring,
      y: anchor.y + Math.sin(angle) * GROUP_CELL_HEIGHT * 0.7 * ring
    });
  });

  // Drop positions of nodes that no longer exist.
  const live = new Set(nodes.map((node) => node.ip));
  positions.forEach((_, ip) => {
    if (!live.has(ip)) {
      positions.delete(ip);
    }
  });

  pageState.positions = positions;
}

function groupAnchor(group, positions) {
  let sumX = 0;
  let sumY = 0;
  let count = 0;
  group.nodes.forEach((node) => {
    const position = positions.get(node.ip);
    if (position) {
      sumX += position.x;
      sumY += position.y;
      count += 1;
    }
  });
  if (count) {
    return { x: sumX / count, y: sumY / count };
  }
  return { x: 0, y: 0 };
}

// structuredLayout builds a deterministic tidy grid: every group becomes a
// wrapped block of node cells, and blocks are laid out left to right.
function structuredLayout(nodes) {
  const positions = new Map();

  const grouped = new Map();
  const ungrouped = [];
  nodes.forEach((node) => {
    const key = nodeGroupKey(node);
    if (!key) {
      ungrouped.push(node);
      return;
    }
    if (!grouped.has(key)) {
      grouped.set(key, []);
    }
    grouped.get(key).push(node);
  });

  const keys = [...grouped.keys()].sort();
  let cursorX = 0;
  let rowHeight = 0;
  let nextRowY = 0;

  const placeBlock = (members, blockColumns) => {
    const columns = Math.max(1, Math.min(blockColumns, Math.ceil(Math.sqrt(members.length) + 1)));
    const rows = Math.ceil(members.length / columns);
    const width = columns * GROUP_CELL_WIDTH;
    const height = rows * GROUP_CELL_HEIGHT;

    let originX = cursorX;
    let originY = nextRowY;
    if (originX + width > GROUP_WRAP_WIDTH && originX > 0) {
      originX = 0;
      nextRowY += rowHeight + GROUP_GAP;
      originY = nextRowY;
      rowHeight = 0;
    }

    members.forEach((node, index) => {
      const column = index % columns;
      const row = Math.floor(index / columns);
      positions.set(node.ip, {
        x: originX + column * GROUP_CELL_WIDTH + GROUP_CELL_WIDTH / 2,
        y: originY + row * GROUP_CELL_HEIGHT + GROUP_CELL_HEIGHT / 2
      });
    });

    cursorX = originX + width + GROUP_GAP;
    rowHeight = Math.max(rowHeight, height);
    return { width, height };
  };

  keys.forEach((key) => {
    placeBlock(grouped.get(key), GROUP_COLUMNS);
  });
  if (ungrouped.length) {
    placeBlock(ungrouped, 6);
  }

  return positions;
}

// ---------------------------------------------------------------------------
// Pan / zoom
// ---------------------------------------------------------------------------

function applyTransform() {
  const { x, y, k } = pageState.transform;
  mapViewport.setAttribute("transform", `translate(${x},${y}) scale(${k})`);
  mapNotes.style.transform = `translate(${x}px,${y}px) scale(${k})`;
}

function zoomAt(cx, cy, factor) {
  const { x, y, k } = pageState.transform;
  const nextK = Math.min(ZOOM_MAX, Math.max(ZOOM_MIN, k * factor));
  const applied = nextK / k;
  pageState.transform = {
    x: cx - (cx - x) * applied,
    y: cy - (cy - y) * applied,
    k: nextK
  };
  applyTransform();
}

function zoomAroundCenter(factor) {
  const rect = mapCanvas.getBoundingClientRect();
  zoomAt(rect.width / 2, rect.height / 2, factor);
}

function fitMap(force = false) {
  const nodes = pageState.map.nodes;
  const rect = mapCanvas.getBoundingClientRect();
  if ((!nodes.length && !pageState.map.notes.length) || rect.width < 40 || rect.height < 40) {
    return;
  }

  let minX = Infinity;
  let minY = Infinity;
  let maxX = -Infinity;
  let maxY = -Infinity;
  pageState.positions.forEach((point) => {
    minX = Math.min(minX, point.x - NODE_EXTENT_HALF_W - GROUP_PAD_X);
    maxX = Math.max(maxX, point.x + NODE_EXTENT_HALF_W + GROUP_PAD_X);
    minY = Math.min(minY, point.y + NODE_EXTENT_TOP - GROUP_PAD_TOP);
    maxY = Math.max(maxY, point.y + NODE_EXTENT_BOTTOM + GROUP_PAD_BOTTOM);
  });
  pageState.map.notes.forEach((note) => {
    minX = Math.min(minX, note.x);
    maxX = Math.max(maxX, note.x + note.w);
    minY = Math.min(minY, note.y);
    maxY = Math.max(maxY, note.y + note.h);
  });

  const padding = 48;
  const contentWidth = Math.max(maxX - minX, 1) + padding * 2;
  const contentHeight = Math.max(maxY - minY, 1) + padding * 2;
  const k = Math.min(ZOOM_MAX, Math.min(rect.width / contentWidth, rect.height / contentHeight));
  pageState.transform = {
    k,
    x: (rect.width - (maxX + minX) * k) / 2,
    y: (rect.height - (maxY + minY) * k) / 2
  };
  if (force) {
    pageState.fitApplied = true;
  }
  applyTransform();
}

function startPan(event) {
  pageState.pan = {
    pointerId: event.pointerId,
    startX: event.clientX,
    startY: event.clientY,
    originX: pageState.transform.x,
    originY: pageState.transform.y
  };
  mapCanvas.classList.add("is-panning");
}

function movePan(event) {
  if (!pageState.pan || event.pointerId !== pageState.pan.pointerId) {
    return;
  }
  pageState.transform.x = pageState.pan.originX + (event.clientX - pageState.pan.startX);
  pageState.transform.y = pageState.pan.originY + (event.clientY - pageState.pan.startY);
  applyTransform();
}

function endPan() {
  pageState.pan = null;
  mapCanvas.classList.remove("is-panning");
}

function startNodeDrag(nodeGroup, event) {
  const ip = nodeGroup.dataset.mapNode || "";
  const position = pageState.positions.get(ip);
  if (!position) {
    return;
  }
  const rect = mapCanvas.getBoundingClientRect();
  pageState.drag = {
    pointerId: event.pointerId,
    ip,
    offsetX: (event.clientX - rect.left - pageState.transform.x) / pageState.transform.k - position.x,
    offsetY: (event.clientY - rect.top - pageState.transform.y) / pageState.transform.k - position.y,
    moved: false
  };
  nodeGroup.classList.add("is-dragging");
}

function moveNodeDrag(event) {
  const drag = pageState.drag;
  if (!drag || event.pointerId !== drag.pointerId) {
    return;
  }
  const rect = mapCanvas.getBoundingClientRect();
  const x = (event.clientX - rect.left - pageState.transform.x) / pageState.transform.k - drag.offsetX;
  const y = (event.clientY - rect.top - pageState.transform.y) / pageState.transform.k - drag.offsetY;
  pageState.positions.set(drag.ip, { x, y });
  drag.moved = true;

  const group = mapNodes.querySelector(`[data-map-node="${cssEscape(drag.ip)}"]`);
  if (group) {
    group.setAttribute("transform", `translate(${x},${y})`);
  }
  syncEdgesForNode(drag.ip);
  renderGroups();
}

function endNodeDrag() {
  if (pageState.drag) {
    const group = mapNodes.querySelector(`[data-map-node="${cssEscape(pageState.drag.ip)}"]`);
    group?.classList.remove("is-dragging");
    pageState.drag = null;
  }
}

function syncEdgesForNode(ip) {
  const position = pageState.positions.get(ip);
  if (!position) {
    return;
  }
  mapEdges.querySelectorAll(".map-edge").forEach((edge) => {
    let otherKey = "";
    let isSource = false;
    if (edge.dataset.from === ip) {
      otherKey = edge.dataset.to;
      isSource = true;
    } else if (edge.dataset.to === ip) {
      otherKey = edge.dataset.from;
      isSource = false;
    } else {
      return;
    }

    const other = pageState.positions.get(otherKey);
    if (!other) {
      return;
    }
    const from = isSource ? position : other;
    const to = isSource ? other : position;
    const endpoints = edgeEndpoints(from, to);
    edge.setAttribute("x1", String(endpoints.x1));
    edge.setAttribute("y1", String(endpoints.y1));
    edge.setAttribute("x2", String(endpoints.x2));
    edge.setAttribute("y2", String(endpoints.y2));
  });
}

function cssEscape(value) {
  if (window.CSS?.escape) {
    return window.CSS.escape(value);
  }
  return String(value).replace(/([^a-zA-Z0-9_-])/g, "\\$1");
}

// ---------------------------------------------------------------------------
// Notes
// ---------------------------------------------------------------------------

function renderNotes() {
  const focused = document.activeElement?.closest?.("[data-map-note]");
  const keepID = focused ? focused.dataset.mapNote : pageState.editingNoteID;

  const existing = new Map([...mapNotes.children].map((el) => [el.dataset.mapNote, el]));
  const live = new Set(pageState.map.notes.map((note) => String(note.id)));

  pageState.map.notes.forEach((note) => {
    const id = String(note.id);
    let element = existing.get(id);
    if (id === keepID && element) {
      // Never clobber the note being edited or dragged right now.
      element.style.left = `${note.x}px`;
      element.style.top = `${note.y}px`;
      if (!element.classList.contains("is-dragging") && !element.classList.contains("is-resizing")) {
        element.style.width = `${note.w}px`;
        element.style.height = `${note.h}px`;
        element.style.setProperty("--map-note-font", `${note.font}px`);
      }
      return;
    }
    if (element) {
      element.replaceWith(makeNoteElement(note));
      return;
    }
    mapNotes.appendChild(makeNoteElement(note));
  });

  existing.forEach((element, id) => {
    if (!live.has(id)) {
      element.remove();
    }
  });
}

function makeNoteElement(note) {
  const element = document.createElement("div");
  element.className = "map-note";
  element.dataset.mapNote = String(note.id);
  element.style.left = `${note.x}px`;
  element.style.top = `${note.y}px`;
  element.style.width = `${note.w}px`;
  element.style.height = `${note.h}px`;
  element.style.setProperty("--map-note-font", `${note.font}px`);
  element.innerHTML = `
    <div class="map-note-head">
      <button type="button" data-note-font="-1" title="Smaller text">A−</button>
      <button type="button" data-note-font="1" title="Larger text">A+</button>
      <button type="button" class="map-note-delete" data-note-delete title="Delete note">×</button>
    </div>
    <div class="map-note-text" contenteditable="false" spellcheck="false" role="textbox" aria-label="Map note text"></div>
    <span class="map-note-resize" aria-hidden="true"></span>
  `;
  element.querySelector(".map-note-text").textContent = note.text || "";
  if (String(note.id) === pageState.editingNoteID) {
    applyNoteEditing(element, true);
  }
  return element;
}

// Word-style notes: plain text at rest, right-click to enter edit mode with
// the frame, font controls, and the resize handle.
function enterNoteEdit(element) {
  if (pageState.editingNoteID && pageState.editingNoteID !== element.dataset.mapNote) {
    const previous = mapNotes.querySelector(`[data-map-note="${cssEscape(pageState.editingNoteID)}"]`);
    if (previous) {
      applyNoteEditing(previous, false);
      void saveNote(previous);
    }
  }
  pageState.editingNoteID = element.dataset.mapNote;
  applyNoteEditing(element, true);
  element.querySelector(".map-note-text").focus();
}

function exitNoteEdit() {
  if (!pageState.editingNoteID) {
    return;
  }
  const element = mapNotes.querySelector(`[data-map-note="${cssEscape(pageState.editingNoteID)}"]`);
  pageState.editingNoteID = "";
  if (element) {
    applyNoteEditing(element, false);
    void saveNote(element);
  }
}

function applyNoteEditing(element, editing) {
  element.classList.toggle("is-editing", editing);
  element.querySelector(".map-note-text").setAttribute("contenteditable", editing ? "true" : "false");
}

function findNoteState(id) {
  return pageState.map.notes.find((note) => String(note.id) === String(id)) || null;
}

function currentNoteFromElement(element) {
  const note = findNoteState(element.dataset.mapNote);
  if (!note) {
    return null;
  }
  return {
    ...note,
    x: parseFloat(element.style.left) || note.x,
    y: parseFloat(element.style.top) || note.y,
    w: parseFloat(element.style.width) || note.w,
    h: parseFloat(element.style.height) || note.h,
    font: parseInt(getComputedStyle(element.querySelector(".map-note-text")).fontSize, 10) || note.font,
    text: element.querySelector(".map-note-text").textContent || ""
  };
}

async function createNote() {
  const rect = mapCanvas.getBoundingClientRect();
  const { x, y, k } = pageState.transform;
  const worldX = (rect.width / 2 - x) / k - NOTE_DEFAULT_W / 2;
  const worldY = (rect.height / 2 - y) / k - NOTE_DEFAULT_H / 2;

  try {
    const note = await api(withProjectQuery("/api/network-map/notes", pageState.ctx?.project || ""), {
      method: "PUT",
      body: JSON.stringify({
        id: 0,
        x: Math.round(worldX),
        y: Math.round(worldY),
        w: NOTE_DEFAULT_W,
        h: NOTE_DEFAULT_H,
        font: NOTE_DEFAULT_FONT,
        text: ""
      })
    });
    pageState.map.notes.push(note);
    mapNotes.appendChild(makeNoteElement(note));
    const element = mapNotes.querySelector(`[data-map-note="${note.id}"]`);
    if (element) {
      enterNoteEdit(element);
    }
  } catch (error) {
    setScanOutput(`Could not create note: ${error.message}`, "error");
  }
}

function startNoteDrag(element, event) {
  const isResize = Boolean(event.target.closest(".map-note-resize"));
  pageState.noteDrag = {
    pointerId: event.pointerId,
    id: element.dataset.mapNote,
    element,
    mode: isResize ? "resize" : "move",
    startX: event.clientX,
    startY: event.clientY,
    originX: parseFloat(element.style.left) || 0,
    originY: parseFloat(element.style.top) || 0,
    originW: parseFloat(element.style.width) || NOTE_DEFAULT_W,
    originH: parseFloat(element.style.height) || NOTE_DEFAULT_H,
    moved: false
  };
}

function moveNoteDrag(event) {
  const drag = pageState.noteDrag;
  if (!drag || event.pointerId !== drag.pointerId) {
    return;
  }
  const dx = (event.clientX - drag.startX) / pageState.transform.k;
  const dy = (event.clientY - drag.startY) / pageState.transform.k;

  if (!drag.moved && Math.hypot(event.clientX - drag.startX, event.clientY - drag.startY) < 4) {
    return;
  }

  drag.moved = true;
  if (drag.mode === "move") {
    drag.element.classList.add("is-dragging");
    const position = clampNotePosition(drag.originX + dx, drag.originY + dy, drag.element);
    drag.element.style.left = `${position.x}px`;
    drag.element.style.top = `${position.y}px`;
    return;
  }

  drag.element.classList.add("is-resizing");
  drag.element.style.width = `${Math.max(NOTE_MIN_W, drag.originW + dx)}px`;
  drag.element.style.height = `${Math.max(NOTE_MIN_H, drag.originH + dy)}px`;
}

// clampNotePosition keeps a note inside the currently visible part of the map
// so it can never be dropped outside the canvas and lost.
function clampNotePosition(x, y, element) {
  const canvas = mapCanvas.getBoundingClientRect();
  const { tx, ty, k } = { tx: pageState.transform.x, ty: pageState.transform.y, k: pageState.transform.k };
  const worldLeft = -tx / k;
  const worldTop = -ty / k;
  const worldRight = worldLeft + canvas.width / k;
  const worldBottom = worldTop + canvas.height / k;
  const width = parseFloat(element.style.width) || NOTE_DEFAULT_W;
  const height = parseFloat(element.style.height) || NOTE_DEFAULT_H;

  const clamp = (value, min, max) => Math.min(Math.max(value, min), Math.max(min, max));
  return {
    x: clamp(x, worldLeft, worldRight - width),
    y: clamp(y, worldTop, worldBottom - height)
  };
}

function endNoteDrag() {
  const drag = pageState.noteDrag;
  if (!drag) {
    return;
  }
  pageState.noteDrag = null;
  drag.element.classList.remove("is-dragging", "is-resizing");
  if (drag.moved) {
    void saveNote(drag.element);
  }
}

function adjustNoteFont(element, delta) {
  const note = findNoteState(element.dataset.mapNote);
  if (!note) {
    return;
  }
  const next = Math.min(40, Math.max(10, note.font + delta));
  note.font = next;
  element.style.setProperty("--map-note-font", `${next}px`);
  void saveNote(element);
}

async function saveNote(element) {
  const note = currentNoteFromElement(element);
  if (!note) {
    return;
  }
  const index = pageState.map.notes.findIndex((item) => String(item.id) === String(note.id));
  if (index >= 0) {
    pageState.map.notes[index] = note;
  }

  try {
    const stored = await api(withProjectQuery("/api/network-map/notes", pageState.ctx?.project || ""), {
      method: "PUT",
      body: JSON.stringify({
        id: note.id,
        x: Math.round(note.x),
        y: Math.round(note.y),
        w: Math.round(note.w),
        h: Math.round(note.h),
        font: note.font,
        text: note.text
      })
    });
    if (index >= 0) {
      pageState.map.notes[index] = { ...pageState.map.notes[index], ...stored };
    }
  } catch (error) {
    setScanOutput(`Could not save note: ${error.message}`, "error");
  }
}

function scheduleNoteSave(element) {
  const id = element.dataset.mapNote;
  const timer = pageState.noteSaveTimers.get(id);
  if (timer) {
    window.clearTimeout(timer);
  }
  pageState.noteSaveTimers.set(id, window.setTimeout(() => {
    pageState.noteSaveTimers.delete(id);
    void saveNote(element);
  }, NOTE_SAVE_DEBOUNCE_MS));
}

async function deleteNote(element) {
  const id = element.dataset.mapNote;
  try {
    await api(withProjectQuery(`/api/network-map/notes/${encodeURIComponent(id)}`, pageState.ctx?.project || ""), {
      method: "DELETE"
    });
  } catch (error) {
    setScanOutput(`Could not delete note: ${error.message}`, "error");
    return;
  }
  if (pageState.editingNoteID === id) {
    pageState.editingNoteID = "";
  }
  const timer = pageState.noteSaveTimers.get(id);
  if (timer) {
    window.clearTimeout(timer);
    pageState.noteSaveTimers.delete(id);
  }
  pageState.map.notes = pageState.map.notes.filter((note) => String(note.id) !== id);
  element.remove();
  mapEmptyState.classList.toggle("hidden", pageState.map.nodes.length > 0 || pageState.map.notes.length > 0);
}

// ---------------------------------------------------------------------------
// Auto refresh
// ---------------------------------------------------------------------------

function startAutoRefresh() {
  if (pageState.autoRefreshTimer) {
    window.clearInterval(pageState.autoRefreshTimer);
  }
  pageState.autoRefreshTimer = window.setInterval(async () => {
    if (document.hidden || pageState.drag || pageState.pan || pageState.noteDrag || pageState.scanning) {
      return;
    }
    try {
      await refreshAll(true);
    } catch (error) {
      console.warn("network map refresh failed", error);
    }
  }, AUTO_REFRESH_MS);
}

// ---------------------------------------------------------------------------
// pscan console
// ---------------------------------------------------------------------------

function setScanOutput(message, type = "muted") {
  scanOutput.textContent = message || "";
  scanOutput.classList.toggle("error-text", type === "error");
  scanOutput.classList.toggle("muted", type !== "error");
}

function setScanState(text) {
  if (!text) {
    scanStateChip.classList.add("hidden");
    scanStateChip.textContent = "";
    return;
  }
  scanStateChip.classList.remove("hidden");
  scanStateChip.textContent = text;
}

async function runScan() {
  if (pageState.scanning) {
    return;
  }

  const selected = scanHostSelect.value || "";
  const [stableId, connectionId] = selected.split("|");
  const targets = scanTargetsInput.value.trim();
  const ports = scanPortsInput.value.trim();
  // 0 means no execution deadline at all; positive values are capped at 600s.
  const timeoutSeconds = Math.min(600, Math.max(0, Number(scanTimeoutInput.value) || 0));

  if (!stableId) {
    setScanOutput("Select an online host to scan from.", "error");
    return;
  }
  if (!targets) {
    setScanOutput("Enter scan targets, for example 192.168.1.0/24.", "error");
    return;
  }

  // Text mode prints only open ports, so big scans stay far below the
  // module output limit (JSON mode streams closed-port results too).
  const args = ["-h", targets];
  if (ports) {
    args.push("-p", ports);
  }

  pageState.scanning = true;
  runScanButton.disabled = true;
  runScanButton.textContent = "Scanning...";
  setScanState("pscan running");
  setScanOutput(`Running pscan -h ${targets}${ports ? ` -p ${ports}` : ""} from ${scanHostSelect.options[scanHostSelect.selectedIndex]?.textContent || stableId}. Large targets can take a while.`);

  try {
    const response = await api(withProjectQuery(`/api/hosts/modules/pscan/run`, pageState.ctx?.project || ""), {
      method: "POST",
      body: JSON.stringify({
        targets: [{ stableId, connectionId: connectionId || "" }],
        args,
        stdin: "",
        stdinBase64: "",
        timeoutSeconds,
        outputLimitBytes: 1024 * 1024
      })
    });

    const result = (response.items || [])[0] || {};
    const output = result.output || "";
    const discovered = countDiscoveredIPs(output);
    const outputError = extractOutputError(output);

    if (result.error && !output) {
      setScanOutput(`pscan failed: ${result.error}`, "error");
    } else if (!discovered && outputError) {
      setScanOutput(`pscan failed: ${outputError}`, "error");
    } else {
      const notes = [];
      if (result.error) {
        notes.push(result.error);
      }
      if (result.timedOut) {
        notes.push("timed out");
      }
      if (result.truncated) {
        notes.push("output truncated");
      }
      setScanOutput(`Scan finished: ${discovered} host${discovered === 1 ? "" : "s"} discovered.${notes.length ? ` (${notes.join(", ")})` : ""}`);
    }

    await refreshAll(true);
  } catch (error) {
    setScanOutput(error.message, "error");
  } finally {
    pageState.scanning = false;
    runScanButton.disabled = false;
    runScanButton.textContent = "Run scan";
    setScanState("");
  }
}

function countDiscoveredIPs(output) {
  const ips = new Set();
  String(output || "").split("\n").forEach((line) => {
    line = line.trim();
    if (!line.startsWith("{")) {
      return;
    }
    try {
      const payload = JSON.parse(line);
      if (payload.open && payload.ip) {
        ips.add(payload.ip);
      }
    } catch (error) {
      const match = line.match(/^(\S+?):(\d+)(?:\/\w+)?\s+open\b/);
      if (match) {
        ips.add(match[1]);
      }
    }
  });
  return ips.size;
}

// pscan reports argument and target problems as plain output text instead of a
// failed request, so surface the last error-looking line to the operator.
function extractOutputError(output) {
  const lines = String(output || "").split("\n").map((line) => line.trim()).filter(Boolean);
  for (let index = lines.length - 1; index >= 0; index -= 1) {
    const line = lines[index];
    if (line.startsWith("{")) {
      continue;
    }
    const subsystemMatch = line.match(/subsystem error:\s*(.*)$/i);
    if (subsystemMatch) {
      return unquoteSubsystemError(subsystemMatch[1]);
    }
    if (/error/i.test(line)) {
      return unquoteSubsystemError(line);
    }
  }
  return "";
}

function unquoteSubsystemError(value) {
  let message = String(value || "").trim();
  if (message.length > 1 && message.startsWith('"') && message.endsWith('"')) {
    message = message.slice(1, -1);
  }
  return message.replaceAll('\\"', '"').trim();
}

// ---------------------------------------------------------------------------
// Clear scan data
// ---------------------------------------------------------------------------

function openClearMapModal() {
  clearMapError.textContent = "";
  confirmClearMapButton.disabled = false;
  cancelClearMapButton.disabled = false;
  clearMapOverlay.classList.add("visible");
  confirmClearMapButton.focus();
}

function closeClearMapModal() {
  if (!clearMapOverlay.classList.contains("visible")) {
    return;
  }
  if (pageState.clearPending) {
    return;
  }
  clearMapOverlay.classList.remove("visible");
  clearMapError.textContent = "";
}

async function clearMapData() {
  confirmClearMapButton.disabled = true;
  cancelClearMapButton.disabled = true;
  confirmClearMapButton.textContent = "Clearing...";
  pageState.clearPending = true;

  try {
    await api(withProjectQuery("/api/network-map", pageState.ctx?.project || ""), { method: "DELETE" });
    clearMapOverlay.classList.remove("visible");
    setScanOutput("Scan data cleared. Discovered hosts were removed; captured agent hosts remain on the map.");
    await refreshAll(false);
  } catch (error) {
    clearMapError.textContent = error.message;
  } finally {
    pageState.clearPending = false;
    confirmClearMapButton.disabled = false;
    cancelClearMapButton.disabled = false;
    confirmClearMapButton.textContent = "Clear scan data";
  }
}
