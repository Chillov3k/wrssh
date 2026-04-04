import {
  api,
  escapeAttribute,
  escapeHtml,
  formatDate,
  initPage,
  loadProjects,
  markSynced,
  viewHref
} from "./shared.js";

const projectList = document.getElementById("projectList");
const createProjectLink = document.getElementById("createProjectLink");

const pageState = {
  projects: [],
  ctx: null
};

projectList.addEventListener("click", async (event) => {
  const openButton = event.target.closest("[data-open-project]");
  if (openButton) {
    window.location.href = openButton.dataset.openProject;
    return;
  }

  const editButton = event.target.closest("[data-edit-project]");
  if (editButton) {
    window.location.href = editButton.dataset.editProject;
    return;
  }

  const deleteButton = event.target.closest("[data-delete-project]");
  if (deleteButton) {
    await deleteProject(deleteButton);
  }
});

initPage({
  title: "Projects",
  load: async (ctx) => {
    pageState.ctx = ctx;
    createProjectLink.href = viewHref("projectCreate");
    createProjectLink.classList.toggle("hidden", !ctx.user.canCreateProjects);
    await refreshProjects(ctx);
  }
}).catch((error) => {
  console.error(error);
});

async function refreshProjects(ctx) {
  pageState.projects = await loadProjects();
  renderProjects();
  markSynced(ctx, `${pageState.projects.length} project${pageState.projects.length === 1 ? "" : "s"}`);
}

function renderProjects() {
  if (!pageState.projects.length) {
    projectList.innerHTML = `<div class="empty-state"><p>No projects defined yet.</p></div>`;
    return;
  }

  projectList.innerHTML = pageState.projects.map((project) => renderProjectCard(project)).join("");
}

function renderProjectCard(project) {
  const openHref = viewHref("overview", project.name);
  const editHref = viewHref("projectEdit", "", { name: project.name });
  const tags = (project.tags || []).map((tag) => `<span class="tag">${escapeHtml(tag)}</span>`).join("");
  const runtimeChip = project.runtimeStatus
    ? `<span class="chip">runtime ${escapeHtml(project.runtimeStatus)}</span>`
    : "";
  const runtimeError = project.runtimeLastError
    ? `<p class="muted">${escapeHtml(project.runtimeLastError)}</p>`
    : "";
  return `
    <article class="project-block project-card-compact">
      <div class="host-row-head">
        <div>
          <p class="eyebrow">project</p>
          <strong>${escapeHtml(project.name)}</strong>
        </div>
        <div class="inline-actions">
          <span class="chip">${escapeHtml(String(project.hostsCount || 0))} hosts</span>
          <span class="chip">${escapeHtml(String(project.activeConnections || 0))} connections</span>
        </div>
      </div>
      <p class="muted">${escapeHtml(project.description || "No description provided.")}</p>
      <div class="tag-list">
        ${tags || `<span class="tag">no tags</span>`}
      </div>
      <div class="project-stats-line muted">
        <span>${escapeHtml(String(project.artifactsCount || 0))} artifacts</span>
        <span>${escapeHtml(String(project.offlineHostsCount || 0))} offline hosts</span>
        <span>last activity ${escapeHtml(formatDate(project.lastActivityAt))}</span>
      </div>
      <div class="inline-actions">
        ${runtimeChip}
      </div>
      ${runtimeError}
      <div class="inline-actions">
        <button class="action-button" data-open-project="${escapeAttribute(openHref)}">Open</button>
        <button class="ghost-button" data-edit-project="${escapeAttribute(editHref)}">Edit</button>
        <button class="danger-button" data-delete-project="${escapeAttribute(project.name)}" data-hosts-count="${escapeAttribute(String(project.hostsCount || 0))}" data-artifacts-count="${escapeAttribute(String(project.artifactsCount || 0))}">Delete</button>
      </div>
    </article>
  `;
}

async function deleteProject(button) {
  const projectName = button.dataset.deleteProject || "";
  const hostsCount = button.dataset.hostsCount || "0";
  const artifactsCount = button.dataset.artifactsCount || "0";
  const confirmText = `Delete project ${projectName}? This will remove the project, ${hostsCount} hosts and ${artifactsCount} artifacts. Online clients will be killed first.`;
  if (!window.confirm(confirmText)) {
    return;
  }

  const original = button.textContent;
  button.disabled = true;
  button.textContent = "Deleting...";

  try {
    await api(`/api/projects/${encodeURIComponent(projectName)}`, {
      method: "DELETE"
    });
    await refreshProjects(pageState.ctx);
  } catch (error) {
    button.disabled = false;
    button.textContent = original;
    window.alert(error.message);
  }
}
