import {
  escapeHtml,
  formatDate,
  hostMetrics,
  initPage,
  loadDashboard,
  loadHosts,
  loadProject,
  markSynced,
  viewHref
} from "./shared.js";

const summaryGrid = document.getElementById("summaryGrid");
const projectProfile = document.getElementById("projectProfile");

initPage({
  title: "Overview",
  requireProject: true,
  load: async (ctx) => {
    let project;
    try {
      project = await loadProject(ctx.project);
    } catch (error) {
      window.location.href = viewHref("projects");
      return;
    }

    const [dashboard, hosts] = await Promise.all([loadDashboard(), loadHosts()]);
    const metrics = hostMetrics(hosts);

    renderSummary(dashboard);
    renderProjectProfile(project);
    markSynced(ctx, `${ctx.project} · ${metrics.activeConnections} online / ${metrics.uniqueHosts} unique hosts`);
  }
}).catch((error) => {
  console.error(error);
});

function renderSummary(dashboard) {
  const metrics = [
    ["Connected", dashboard?.connectedHosts ?? 0],
    ["Offline", dashboard?.offlineHosts ?? 0],
    ["Live Connections", dashboard?.activeSessions ?? 0],
    ["Artifacts", dashboard?.availableBuilds ?? 0]
  ];

  summaryGrid.innerHTML = metrics.map(([label, value]) => `
    <article class="summary-card">
      <p class="eyebrow">${escapeHtml(label)}</p>
      <strong>${escapeHtml(String(value))}</strong>
    </article>
  `).join("");
}

function renderProjectProfile(project) {
  projectProfile.innerHTML = `
    <article class="project-block project-profile-block">
      <div class="host-row-head">
        <div>
          <p class="eyebrow">selected project</p>
          <strong>${escapeHtml(project.name)}</strong>
        </div>
        <div class="inline-actions">
          <span class="chip">${escapeHtml(String(project.hostsCount || 0))} hosts</span>
          <span class="chip">${escapeHtml(String(project.activeConnections || 0))} connections</span>
          <span class="chip">${escapeHtml(String(project.artifactsCount || 0))} artifacts</span>
        </div>
      </div>
      <p class="muted">${escapeHtml(project.description || "No description provided for this project.")}</p>
      <div class="tag-list">
        ${(project.tags || []).length
          ? project.tags.map((tag) => `<span class="tag">${escapeHtml(tag)}</span>`).join("")
          : `<span class="tag">no tags</span>`}
      </div>
      <p class="muted project-footnote">Last activity: ${escapeHtml(formatDate(project.lastActivityAt))}</p>
    </article>
  `;
}
