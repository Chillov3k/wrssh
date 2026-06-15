export function byId(id) {
  return document.getElementById(id);
}

const VIEW_PATHS = {
  profile: "/profile",
  projects: "/projects",
  projectCreate: "/project-create",
  projectEdit: "/project-edit",
  userEdit: "/user-edit",
  overview: "/overview",
  hosts: "/hosts",
  host: "/host",
  builds: "/builds",
  downloads: "/downloads",
  download: "/download",
  users: "/users"
};

const PROJECT_SCOPED_VIEWS = new Set([
  "overview",
  "hosts",
  "host",
  "builds",
  "downloads",
  "download"
]);

const THEME_STORAGE_KEY = "wrssh.theme";
const SIDEBAR_STORAGE_KEY = "wrssh.sidebarCollapsed";
const LIGHT_THEME = "light";
const DARK_THEME = "dark";
const KNOWN_VIEW_PATHS = Object.values(VIEW_PATHS).sort((left, right) => right.length - left.length);
const APP_BASE_PATH = detectBasePath();

export async function initPage({ title, load, requireProject = false }) {
  setAuthPending(true);

  const shell = {
    loginOverlay: byId("loginOverlay"),
    loginForm: byId("loginForm"),
    loginError: byId("loginError"),
    editProfileButton: byId("editProfileButton"),
    logoutButton: byId("logoutButton"),
    currentUserName: byId("currentUserName"),
    projectContext: byId("projectContext"),
    sidebarProjectName: byId("sidebarProjectName"),
    sidebarProjectTags: byId("sidebarProjectTags"),
    changeProjectLink: byId("changeProjectLink"),
    viewTitle: byId("viewTitle"),
    topbarMeta: byId("topbarMeta"),
    refreshButton: byId("refreshButton")
  };

  const ctx = {
    shell,
    title,
    user: null,
    project: currentProjectName(),
    projectInfo: null,
    requireProject,
    lastSyncAt: null,
    state: {}
  };

  if (shell.viewTitle) {
    shell.viewTitle.textContent = title;
  }

  initThemeUI();
  initSidebarUI();
  syncNavigation(ctx.project);
  bindAuth(ctx, load, requireProject);

  if (shell.refreshButton) {
    shell.refreshButton.addEventListener("click", async () => {
      if (requireProject && !ctx.project) {
        window.location.href = viewHref("projects");
        return;
      }
      await loadWithAuth(ctx, load);
    });
  }

  try {
    ctx.user = await api("/api/auth/me");
    renderUser(ctx);
    hideLogin(ctx);
    if (redirectToProfileIfRequired(ctx)) {
      return ctx;
    }
    if (requireProject && !ctx.project) {
      window.location.href = viewHref("projects");
      return ctx;
    }
    await loadWithAuth(ctx, load);
  } catch (error) {
    if (isAuthError(error)) {
      showLogin(ctx);
      return ctx;
    }
    if (isPasswordChangeRequired(error)) {
      window.location.href = viewHref("profile");
      return ctx;
    }
    hideLogin(ctx);
    if (shell.topbarMeta) {
      shell.topbarMeta.textContent = error.message || "Page load failed";
    }
    throw error;
  }

  return ctx;
}

function bindAuth(ctx, load, requireProject) {
  const { shell } = ctx;

  if (shell.loginForm) {
    shell.loginForm.addEventListener("submit", async (event) => {
      event.preventDefault();
      shell.loginError.textContent = "";

      const payload = {
        username: byId("loginUsername").value.trim(),
        password: byId("loginPassword").value
      };

      try {
        ctx.user = await api("/api/auth/login", {
          method: "POST",
          body: JSON.stringify(payload)
        });
        renderUser(ctx);
        hideLogin(ctx);
        if (redirectToProfileIfRequired(ctx)) {
          return;
        }
        if (requireProject && !ctx.project) {
          window.location.href = viewHref("projects");
          return;
        }
        await loadWithAuth(ctx, load);
      } catch (error) {
        shell.loginError.textContent = error.message;
      }
    });
  }

  if (shell.editProfileButton) {
    shell.editProfileButton.addEventListener("click", () => {
      if (currentViewPath() === VIEW_PATHS.profile) {
        return;
      }
      window.location.href = viewHref("profile");
    });
  }

  if (shell.logoutButton) {
    shell.logoutButton.addEventListener("click", async () => {
      try {
        await api("/api/auth/logout", { method: "POST" });
      } finally {
        ctx.user = null;
        showLogin(ctx);
        window.location.href = viewHref("projects");
      }
    });
  }
}

function initThemeUI() {
  applyTheme(readStoredTheme());

  const brandBlock = document.querySelector(".brand-block");
  if (!brandBlock) {
    return;
  }

  let themeToggle = brandBlock.querySelector(".theme-toggle");
  if (!themeToggle) {
    const title = brandBlock.querySelector("h1");
    if (!title) {
      return;
    }

    const head = document.createElement("div");
    head.className = "brand-head";
    title.replaceWith(head);
    head.appendChild(title);

    themeToggle = document.createElement("button");
    themeToggle.className = "theme-toggle";
    themeToggle.type = "button";
    themeToggle.setAttribute("role", "switch");
    themeToggle.setAttribute("aria-label", "Toggle theme");
    themeToggle.innerHTML = `
      <img class="theme-toggle-icon theme-toggle-icon-light" src="${appPath("/1.png")}" alt="" aria-hidden="true">
      <span class="theme-toggle-switch" aria-hidden="true">
        <span class="theme-toggle-thumb"></span>
      </span>
      <img class="theme-toggle-icon theme-toggle-icon-dark" src="${appPath("/2.png")}" alt="" aria-hidden="true">
    `;
    head.appendChild(themeToggle);
  }

  themeToggle.addEventListener("click", () => {
    applyTheme(currentTheme() === DARK_THEME ? LIGHT_THEME : DARK_THEME);
  });
  syncThemeToggle(themeToggle);
}

function initSidebarUI() {
  const appShell = document.querySelector(".app-shell");
  const sidebar = document.querySelector(".sidebar");
  const brandHead = document.querySelector(".brand-head");
  if (!appShell || !sidebar || !brandHead) {
    return;
  }

  document.querySelectorAll(".nav-item").forEach((item) => {
    const label = item.textContent.trim();
    item.dataset.short = sidebarShortLabel(label);
    item.setAttribute("title", label);
  });

  const editProfileButton = byId("editProfileButton");
  const logoutButton = byId("logoutButton");
  if (editProfileButton) {
    editProfileButton.dataset.short = "E";
    editProfileButton.setAttribute("title", "Edit profile");
  }
  if (logoutButton) {
    logoutButton.dataset.short = "L";
    logoutButton.setAttribute("title", "Logout");
  }

  let sidebarToggle = brandHead.querySelector(".sidebar-toggle");
  if (!sidebarToggle) {
    sidebarToggle = document.createElement("button");
    sidebarToggle.className = "sidebar-toggle";
    sidebarToggle.type = "button";
    brandHead.appendChild(sidebarToggle);
  }

  const applyCollapsed = (collapsed) => {
    appShell.classList.toggle("sidebar-collapsed", collapsed);
    sidebarToggle.textContent = collapsed ? "›" : "‹";
    sidebarToggle.setAttribute("aria-label", collapsed ? "Show menu" : "Hide menu");
    sidebarToggle.setAttribute("title", collapsed ? "Show menu" : "Hide menu");
    sidebarToggle.setAttribute("aria-expanded", String(!collapsed));
    try {
      window.localStorage.setItem(SIDEBAR_STORAGE_KEY, collapsed ? "1" : "0");
    } catch (error) {
      console.warn("sidebar storage unavailable", error);
    }
  };

  applyCollapsed(readStoredSidebarCollapsed());
  sidebarToggle.addEventListener("click", () => {
    applyCollapsed(!appShell.classList.contains("sidebar-collapsed"));
  });
}

function readStoredSidebarCollapsed() {
  try {
    return window.localStorage.getItem(SIDEBAR_STORAGE_KEY) === "1";
  } catch (error) {
    console.warn("sidebar storage unavailable", error);
    return false;
  }
}

function sidebarShortLabel(label) {
  switch (label.toLowerCase()) {
  case "overview":
    return "O";
  case "hosts":
    return "H";
  case "builds":
    return "B";
  case "downloads":
    return "D";
  case "manage users":
    return "U";
  default:
    return label.slice(0, 1).toUpperCase() || "?";
  }
}

async function loadWithAuth(ctx, load) {
  try {
    if (ctx.requireProject) {
      if (!ctx.project) {
        window.location.href = viewHref("projects");
        return;
      }
      const ok = await syncProjectContext(ctx);
      if (!ok) {
        return;
      }
    }
    await load(ctx);
  } catch (error) {
    if (isAuthError(error)) {
      showLogin(ctx);
      return;
    }
    if (isPasswordChangeRequired(error)) {
      window.location.href = viewHref("profile");
      return;
    }
    throw error;
  }
}

function isAuthError(error) {
  return Number(error?.status) === 401;
}

function isPasswordChangeRequired(error) {
  const message = String(error?.message || "").toLowerCase();
  return message.includes("password change required");
}

function renderUser(ctx) {
  if (!ctx.user) {
    return;
  }

  ctx.shell.currentUserName.textContent = ctx.user.username;
  if (ctx.shell.editProfileButton) {
    const onProfilePage = currentViewPath() === VIEW_PATHS.profile;
    ctx.shell.editProfileButton.disabled = onProfilePage;
    ctx.shell.editProfileButton.title = onProfilePage ? "Already on profile page" : "";
  }
  applyCapabilities(ctx.user);
}

function readStoredTheme() {
  try {
    const stored = window.localStorage.getItem(THEME_STORAGE_KEY);
    if (stored === DARK_THEME || stored === LIGHT_THEME) {
      return stored;
    }
  } catch (error) {
    console.warn("theme storage unavailable", error);
  }

  return document.documentElement.dataset.theme === DARK_THEME ? DARK_THEME : LIGHT_THEME;
}

function currentTheme() {
  return document.documentElement.dataset.theme === DARK_THEME ? DARK_THEME : LIGHT_THEME;
}

function applyTheme(theme) {
  const nextTheme = theme === DARK_THEME ? DARK_THEME : LIGHT_THEME;
  document.documentElement.dataset.theme = nextTheme;
  document.documentElement.style.colorScheme = nextTheme;

  try {
    window.localStorage.setItem(THEME_STORAGE_KEY, nextTheme);
  } catch (error) {
    console.warn("theme storage unavailable", error);
  }

  document.querySelectorAll(".theme-toggle").forEach((toggle) => syncThemeToggle(toggle));
}

function syncThemeToggle(toggle) {
  const isDark = currentTheme() === DARK_THEME;
  toggle.classList.toggle("is-dark", isDark);
  toggle.setAttribute("aria-checked", String(isDark));
  toggle.setAttribute("title", isDark ? "Switch to light theme" : "Switch to dark theme");
}

function redirectToProfileIfRequired(ctx) {
  if (!ctx?.user?.mustChangePassword) {
    return false;
  }
  if (currentViewPath() === VIEW_PATHS.profile) {
    return false;
  }
  window.location.href = viewHref("profile");
  return true;
}

async function syncProjectContext(ctx) {
  try {
    ctx.projectInfo = await loadProject(ctx.project);
    renderProjectContext(ctx);
    return true;
  } catch (error) {
    window.location.href = viewHref("projects");
    return false;
  }
}

function renderProjectContext(ctx) {
  const { shell, projectInfo } = ctx;
  if (!shell.projectContext || !projectInfo) {
    return;
  }

  shell.projectContext.classList.remove("hidden");
  if (shell.sidebarProjectName) {
    shell.sidebarProjectName.textContent = projectInfo.name || ctx.project;
  }
  if (shell.sidebarProjectTags) {
    const tags = Array.isArray(projectInfo.tags) ? projectInfo.tags : [];
    shell.sidebarProjectTags.innerHTML = tags.length
      ? tags.map((tag) => `<span class="tag">${escapeHtml(tag)}</span>`).join("")
      : `<span class="tag">no tags</span>`;
  }
  if (shell.changeProjectLink) {
    shell.changeProjectLink.setAttribute("href", viewHref("projects"));
  }
}

export function markSynced(ctx, extra = "") {
  ctx.lastSyncAt = new Date().toISOString();
  const syncText = ctx.lastSyncAt ? `Last sync ${formatDate(ctx.lastSyncAt)}` : "Waiting for initial sync";
  ctx.shell.topbarMeta.textContent = extra ? `${syncText} · ${extra}` : syncText;
}

export function showLogin(ctx) {
  setAuthPending(false);
  ctx.shell.loginOverlay?.classList.add("visible");
}

export function hideLogin(ctx) {
  setAuthPending(false);
  ctx.shell.loginOverlay?.classList.remove("visible");
}

function setAuthPending(active) {
  document.body?.classList.toggle("auth-pending", Boolean(active));
}

export async function api(path, options = {}) {
  const method = String(options.method || "GET").toUpperCase();
  const headers = {
    ...(options.headers || {})
  };

  if (options.body && !headers["Content-Type"]) {
    headers["Content-Type"] = "application/json";
  }

  const targetPath = appPath(path);
  const response = await fetch(method === "GET" ? withCacheBust(targetPath) : targetPath, {
    ...options,
    cache: "no-store",
    credentials: "include",
    headers
  });

  if (response.status === 204) {
    return null;
  }

  let payload = {};
  const text = await response.text();
  if (text) {
    try {
      payload = JSON.parse(text);
    } catch (error) {
      payload = { error: text };
    }
  }

  if (!response.ok) {
    const error = new Error(payload.error || `Request failed with ${response.status}`);
    error.status = response.status;
    error.payload = payload;
    throw error;
  }

  return payload;
}

export function websocketURL(path) {
  const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${protocol}//${window.location.host}${appPath(path)}`;
}

export function withCacheBust(path) {
  const separator = path.includes("?") ? "&" : "?";
  return `${path}${separator}_=${Date.now()}`;
}

export function formatDate(value) {
  if (!value) {
    return "-";
  }
  return new Date(value).toLocaleString();
}

export function escapeHtml(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

export function escapeAttribute(value) {
  return escapeHtml(value).replaceAll("`", "&#96;");
}

export async function copyFromButton(button, text, copiedLabel = "Copied") {
  await copyText(text);
  const original = button.textContent;
  button.textContent = copiedLabel;
  window.setTimeout(() => {
    button.textContent = original;
  }, 900);
}

export async function copyText(text) {
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text);
      return;
    } catch (error) {
      // Continue to the plain-HTTP fallback below.
    }
  }

  const textarea = document.createElement("textarea");
  textarea.value = text;
  textarea.setAttribute("readonly", "readonly");
  textarea.style.position = "fixed";
  textarea.style.opacity = "0";
  document.body.appendChild(textarea);
  textarea.focus();
  textarea.select();
  const ok = document.execCommand("copy");
  document.body.removeChild(textarea);
  if (!ok) {
    throw new Error("clipboard copy failed");
  }
}

export async function loadHosts() {
  const response = await api(withProjectQuery("/api/hosts"));
  return response.items || [];
}

export async function loadHost(stableId, project = currentProjectName()) {
  return api(withProjectQuery(`/api/hosts/${encodeURIComponent(stableId)}`, project));
}

export async function loadDashboard(project = currentProjectName()) {
  return api(withProjectQuery("/api/dashboard", project));
}

export async function loadSystemOptions(project = currentProjectName()) {
  return api(withProjectQuery("/api/system/options", project));
}

export async function loadArtifacts(project = currentProjectName()) {
  const response = await api(withProjectQuery("/api/artifacts", project));
  return response.items || [];
}

export async function loadArtifact(urlPath, project = currentProjectName()) {
  return api(withProjectQuery(`/api/artifacts/${encodeURIComponent(urlPath)}`, project));
}

export async function loadProjects() {
  const response = await api("/api/projects");
  return response.items || [];
}

export async function loadProject(name) {
  return api(`/api/projects/${encodeURIComponent(name)}`);
}

export async function loadUsers() {
  const response = await api("/api/users");
  return response.items || [];
}

export async function loadUser(username) {
  return api(`/api/users/${encodeURIComponent(username)}`);
}

export async function loadProfile() {
  return api("/api/profile");
}

export function currentProjectName() {
  return new URLSearchParams(window.location.search).get("project")?.trim() || "";
}

export function withProjectQuery(path, project = currentProjectName()) {
  const url = new URL(appPath(path), window.location.origin);
  const selectedProject = String(project || "").trim();
  if (!selectedProject) {
    return `${url.pathname}${url.search}`;
  }
  url.searchParams.set("project", selectedProject);
  return `${url.pathname}${url.search}`;
}

export function viewHref(view, project = currentProjectName(), extraParams = {}) {
  const path = VIEW_PATHS[view] || view;
  const params = new URLSearchParams();

  if (PROJECT_SCOPED_VIEWS.has(view) && project) {
    params.set("project", project);
  }

  Object.entries(extraParams).forEach(([key, value]) => {
    if (value === undefined || value === null || value === "") {
      return;
    }
    params.set(key, String(value));
  });

  const query = params.toString();
  const resolvedPath = query ? `${path}?${query}` : path;
  return appPath(resolvedPath);
}

export function hostMetrics(hosts) {
  const list = Array.isArray(hosts) ? hosts : [];

  let onlineHosts = 0;
  let offlineHosts = 0;
  let activeConnections = 0;

  list.forEach((host) => {
    const activeCount = Array.isArray(host?.activeConnections) ? host.activeConnections.length : 0;
    if (activeCount > 0) {
      onlineHosts += 1;
      activeConnections += activeCount;
      return;
    }

    offlineHosts += 1;
  });

  return {
    uniqueHosts: list.length,
    onlineHosts,
    offlineHosts,
    activeConnections
  };
}

export function getClientRows(hosts) {
  return [...hosts]
    .flatMap((host) => {
      const activeConnections = host.activeConnections || [];
      if (!activeConnections.length) {
        return [makeClientRow(host, null)];
      }
      return activeConnections.map((connection) => makeClientRow(host, connection));
    })
    .sort((left, right) => {
      if (left.connected !== right.connected) {
        return left.connected ? -1 : 1;
      }

      const leftTime = new Date(left.lastActivityAt || left.dateAdded || 0).getTime();
      const rightTime = new Date(right.lastActivityAt || right.dateAdded || 0).getTime();
      if (leftTime !== rightTime) {
        return rightTime - leftTime;
      }

      return left.hostname.localeCompare(right.hostname);
    });
}

export function makeClientRow(host, connection) {
  return {
    key: `${host.stableId}:${connection?.connectionId || "offline"}`,
    stableId: host.stableId,
    hostId: host.hostId || host.stableId,
    connectionId: connection?.connectionId || "",
    hostname: hostNameLabel(host, connection),
    ip: connectionIpLabel(connection, host),
    remoteAddr: connection?.remoteAddr || host.remoteAddr || "",
    comment: connection?.comment || host.comment || "",
    version: connection?.version || host.version || "",
    project: host.project || "",
    tags: [...(host.tags || [])],
    platform: platformLabel(connection?.version || host.version),
    connected: Boolean(connection),
    dateAdded: host.dateAdded,
    lastActivityAt: host.lastActivityAt || host.lastConnectionAt || host.lastDisconnectAt || host.dateAdded,
    host,
    connection
  };
}

function hostNameLabel(host, connection) {
  return host.hostname || connection?.hostname || host.preferredAlias || connection?.connectionId || host.stableId;
}

function connectionIpLabel(connection, host) {
  return connection?.remoteIp || extractRemoteHost(connection?.remoteAddr) || host.ip || extractRemoteHost(host.remoteAddr) || "-";
}

export function extractRemoteHost(value) {
  const remote = String(value || "").trim();
  if (!remote) {
    return "";
  }

  if (remote.startsWith("[")) {
    const closing = remote.indexOf("]");
    return closing === -1 ? remote : remote.slice(1, closing);
  }

  const lastColon = remote.lastIndexOf(":");
  if (lastColon > -1 && /^\d+$/.test(remote.slice(lastColon + 1))) {
    return remote.slice(0, lastColon);
  }

  return remote;
}

export function rowMatchesQuery(row, query) {
  if (!query) {
    return true;
  }

  return [
    row.hostname,
    row.project,
    row.ip,
    row.remoteAddr,
    row.comment,
    row.hostId,
    row.stableId,
    row.connectionId,
    row.platform,
    ...(row.tags || [])
  ].filter(Boolean).some((value) => value.toLowerCase().includes(query));
}

export function groupHostsByProject(hosts) {
  const groups = new Map();
  hosts.forEach((host) => {
    const project = host.project || "Unassigned";
    if (!groups.has(project)) {
      groups.set(project, []);
    }
    groups.get(project).push(host);
  });

  return [...groups.entries()].sort(([left], [right]) => left.localeCompare(right));
}

export function parsePlatform(version) {
  const value = String(version || "").trim();
  const match = value.match(/-([A-Za-z0-9]+)_([A-Za-z0-9]+)$/);
  if (!match) {
    return {
      os: "",
      arch: "",
      label: "-"
    };
  }

  const os = match[1].toLowerCase();
  const arch = match[2].toLowerCase();
  return {
    os,
    arch,
    label: `${os} / ${arch}`
  };
}

export function platformLabel(version) {
  return parsePlatform(version).label;
}

export function resolveConnection(host, connectionId = "") {
  const requested = String(connectionId || "").trim();
  if (!host) {
    return null;
  }

  const activeConnections = host.activeConnections || [];
  if (!requested) {
    return activeConnections[0] || null;
  }

  return activeConnections.find((connection) => connection.connectionId === requested) || null;
}

export function buildJumpTarget(systemOptions, host = null, user = null) {
  const selectedHost = systemOptions?.defaultInterface;
  const selectedPort = systemOptions?.defaultPort;
  const username = String(user?.rsshUsername || user?.username || "").trim();
  if (selectedHost && selectedPort) {
    const address = `${selectedHost}:${selectedPort}`;
    return username ? `${username}@${address}` : address;
  }

  const match = String(host?.commands?.ssh || "").match(/-J\s+([^\s]+)/);
  if (match) {
    return match[1];
  }

  if (typeof window !== "undefined" && window.location?.host) {
    return username ? `${username}@${window.location.host}` : window.location.host;
  }

  return "";
}

export function hostCommandTemplates(host, connection, jumpTarget) {
  const target = connectTarget(host, connection);
  return {
    ssh: `ssh -J ${jumpTarget} ${target}`,
    scp: `scp -J ${jumpTarget} ${target}:/path/to/file .`,
    dynamicSocks: `ssh -D 1080 -J ${jumpTarget} ${target}`,
    remoteForward: `ssh -R 1234:localhost:1234 -J ${jumpTarget} ${target}`
  };
}

function connectTarget(host, connection) {
  if (connection?.connectionId) {
    return connection.connectionId;
  }

  const active = host?.activeConnections?.[0];
  if (active?.connectionId) {
    return active.connectionId;
  }

  const candidates = [
    host?.hostname,
    host?.preferredAlias,
    host?.hostId,
    host?.stableId
  ];

  for (const candidate of candidates) {
    const value = String(candidate || "").trim();
    if (value && !/\s/.test(value)) {
      return value;
    }
  }

  return host?.stableId || "";
}

export function hostPageHref(stableId, connectionId = "", project = currentProjectName()) {
  return viewHref("host", project, {
    stableId,
    connectionId
  });
}

export function artifactPageHref(urlPath, project = currentProjectName()) {
  return viewHref("download", project, { urlPath });
}

export function artifactSizeLabel(value) {
  return Number(value || 0).toFixed(2);
}

export function renderArtifactLink(label, url, options = {}) {
  const displayText = options.displayText || url;
  const copyText = options.copyText || displayText;
  return `
    <div class="artifact-link">
      <strong>${escapeHtml(label)}</strong>
      <div class="artifact-actions">
        <a class="inline-link" href="${escapeAttribute(url)}" target="_blank" rel="noreferrer">Open</a>
        <button class="ghost-button" data-copy-artifact="${escapeAttribute(copyText)}">Copy</button>
      </div>
      <code>${escapeHtml(displayText)}</code>
    </div>
  `;
}

function syncNavigation(project) {
  document.querySelectorAll(".nav-item").forEach((item) => {
    const href = item.getAttribute("href") || "";
    const pathname = stripBasePath(new URL(href, window.location.href).pathname);
    const route = Object.entries(VIEW_PATHS).find(([, path]) => path === pathname)?.[0];
    if (!route) {
      return;
    }
    item.setAttribute("href", viewHref(route, project));
  });
}

function applyCapabilities(user) {
  const isAdmin = user?.role === "admin";
  const canCreate = Boolean(user?.canCreateProjects);

  document.querySelectorAll("[data-admin-only]").forEach((node) => {
    node.classList.toggle("hidden", !isAdmin);
  });
  document.querySelectorAll("[data-create-project-only]").forEach((node) => {
    node.classList.toggle("hidden", !canCreate);
  });
}

function detectBasePath() {
  const pathname = normalizePath(window.location.pathname);
  if (pathname === "/") {
    return "";
  }

  for (const viewPath of KNOWN_VIEW_PATHS) {
    if (pathname === viewPath) {
      return "";
    }
    if (pathname.endsWith(viewPath)) {
      const prefix = pathname.slice(0, pathname.length - viewPath.length);
      if (prefix === "" || prefix.startsWith("/")) {
        return normalizeBasePath(prefix);
      }
    }
  }

  const segments = pathname.split("/").filter(Boolean);
  if (segments.length === 1) {
    return `/${segments[0]}`;
  }

  return "";
}

function normalizePath(pathname) {
  const value = String(pathname || "").trim();
  if (value === "" || value === "/") {
    return "/";
  }
  return value.endsWith("/") ? value.slice(0, -1) : value;
}

function normalizeBasePath(pathname) {
  const value = normalizePath(pathname);
  return value === "/" ? "" : value;
}

function stripBasePath(pathname) {
  const normalized = normalizePath(pathname);
  if (!APP_BASE_PATH) {
    return normalized;
  }
  if (normalized === APP_BASE_PATH) {
    return "/";
  }
  if (normalized.startsWith(`${APP_BASE_PATH}/`)) {
    const stripped = normalized.slice(APP_BASE_PATH.length);
    return stripped === "" ? "/" : stripped;
  }
  return normalized;
}

function currentViewPath() {
  return stripBasePath(window.location.pathname);
}

export function appPath(path) {
  let value = String(path || "").trim();
  if (value === "") {
    value = "/";
  }
  if (!value.startsWith("/")) {
    value = `/${value}`;
  }
  if (!APP_BASE_PATH) {
    return value;
  }
  if (value === APP_BASE_PATH || value.startsWith(`${APP_BASE_PATH}/`)) {
    return value;
  }
  return `${APP_BASE_PATH}${value}`;
}
