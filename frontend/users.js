import {
  api,
  copyFromButton,
  escapeAttribute,
  escapeHtml,
  formatDate,
  initPage,
  loadProjects,
  loadUsers,
  markSynced,
  viewHref
} from "./shared.js";
import { ProjectPicker } from "./project-picker.js";

const elements = {
  usersNoAccess: document.getElementById("usersNoAccess"),
  usersAdminViews: document.getElementById("usersAdminViews"),
  createForm: document.getElementById("createUserForm"),
  createUsernameInput: document.getElementById("createUsernameInput"),
  createCanCreateProjects: document.getElementById("createCanCreateProjects"),
  createProjectsPicker: document.getElementById("createProjectsPicker"),
  createAuthorizedKeysInput: document.getElementById("createAuthorizedKeysInput"),
  createUserOutput: document.getElementById("createUserOutput"),
  managedUsersList: document.getElementById("managedUsersList")
};

const pageState = {
  ctx: null,
  projects: [],
  users: [],
  projectPicker: new ProjectPicker(elements.createProjectsPicker, { placeholder: "Choose allowed projects" })
};

elements.createForm.addEventListener("submit", createUser);

elements.createUserOutput.addEventListener("click", async (event) => {
  const button = event.target.closest("[data-copy-secret]");
  if (!button) {
    return;
  }

  await copyFromButton(button, button.dataset.copySecret || "", "Password Copied");
});

elements.managedUsersList.addEventListener("click", async (event) => {
  const deleteButton = event.target.closest("[data-delete-user]");
  if (deleteButton) {
    await deleteUser(deleteButton.dataset.deleteUser || "");
    return;
  }

  const openButton = event.target.closest("[data-open-user]");
  if (openButton) {
    window.location.href = openButton.dataset.openUser;
  }
});

initPage({
  title: "Manage Users",
  load: async (ctx) => {
    pageState.ctx = ctx;

    if (ctx.user.role !== "admin") {
      elements.usersNoAccess.classList.remove("hidden");
      elements.usersAdminViews.classList.add("hidden");
      markSynced(ctx, "user management unavailable");
      return;
    }

    elements.usersNoAccess.classList.add("hidden");
    elements.usersAdminViews.classList.remove("hidden");
    await refreshState();
  }
}).catch((error) => {
  console.error(error);
});

async function refreshState() {
  const [projects, users] = await Promise.all([loadProjects(), loadUsers()]);
  pageState.projects = projects;
  pageState.users = (users || []).filter((user) => user.role !== "admin");

  syncProjectPicker();
  renderUsersList();

  if (pageState.ctx) {
    markSynced(pageState.ctx, `${pageState.users.length} managed users · ${pageState.projects.length} available projects`);
  }
}

function syncProjectPicker() {
  pageState.projectPicker.setOptions(pageState.projects.map((project) => project.name));
  pageState.projectPicker.setSelected([]);
}

function renderUsersList() {
  if (!pageState.users.length) {
    elements.managedUsersList.innerHTML = `<div class="empty-state compact-empty"><p>No managed users created yet.</p></div>`;
    return;
  }

  elements.managedUsersList.innerHTML = pageState.users.map((user) => {
    const editHref = viewHref("userEdit", "", { username: user.username });
    const projectTags = user.allowedProjects.length
      ? user.allowedProjects.map((project) => `<span class="tag">${escapeHtml(project)}</span>`).join("")
      : `<span class="tag">no project access</span>`;

    return `
      <article class="managed-user-row">
        <div class="managed-user-main">
          <div class="managed-user-title">
            <strong>${escapeHtml(user.username)}</strong>
            <span class="chip">${escapeHtml(String(user.sshKeyCount || 0))} keys</span>
            ${user.mustChangePassword ? `<span class="chip">temporary password</span>` : ""}
            ${user.canCreateProjects ? `<span class="chip">can create projects</span>` : ""}
          </div>
          <p class="muted">RSSH username: ${escapeHtml(user.rsshUsername || user.username)}</p>
          <div class="tag-list">${projectTags}</div>
        </div>
        <div class="managed-user-meta">
          <span class="muted">Updated ${escapeHtml(formatDate(user.updatedAt))}</span>
        </div>
        <div class="managed-user-actions">
          <button class="action-button" data-open-user="${escapeAttribute(editHref)}">Edit User</button>
          <button class="danger-button" data-delete-user="${escapeAttribute(user.username)}">Delete</button>
        </div>
      </article>
    `;
  }).join("");
}

function selectedProjects() {
  return pageState.projectPicker.selectedValues();
}

async function createUser(event) {
  event.preventDefault();

  const payload = {
    username: elements.createUsernameInput.value.trim(),
    canCreateProjects: elements.createCanCreateProjects.checked,
    allowedProjects: selectedProjects(),
    sshAuthorizedKeys: elements.createAuthorizedKeysInput.value
  };

  renderStatus(elements.createUserOutput, "Creating user...");

  try {
    const created = await api("/api/users", {
      method: "POST",
      body: JSON.stringify(payload)
    });

    elements.createForm.reset();
    pageState.projectPicker.setSelected([]);
    await refreshState();
    renderSecret(elements.createUserOutput, `User ${created.username} created. This is a temporary password and must be changed on first login.`, created.generatedPassword, created.username);
  } catch (error) {
    renderStatus(elements.createUserOutput, error.message, true);
  }
}

async function deleteUser(username) {
  const targetUsername = String(username || "").trim();
  if (!targetUsername) {
    return;
  }

  if (!window.confirm(`Delete user ${targetUsername}? Their credentials and project grants will be removed.`)) {
    return;
  }

  try {
    await api(`/api/users/${encodeURIComponent(targetUsername)}`, {
      method: "DELETE"
    });
    await refreshState();
  } catch (error) {
    window.alert(error.message);
  }
}

function renderStatus(target, message, isError = false) {
  target.classList.remove("hidden");
  target.classList.toggle("error-note", isError);
  target.innerHTML = `<p class="${isError ? "error-text" : "muted"}">${escapeHtml(message)}</p>`;
}

function renderSecret(target, message, password, username) {
  const editHref = viewHref("userEdit", "", { username });
  target.classList.remove("hidden");
  target.classList.remove("error-note");
  target.innerHTML = `
    <p class="muted">${escapeHtml(message)}</p>
    <div class="secret-row">
      <code>${escapeHtml(password || "-")}</code>
      ${password ? `<button class="ghost-button" data-copy-secret="${escapeAttribute(password)}">Copy Password</button>` : ""}
      <a class="ghost-button action-link" href="${escapeAttribute(editHref)}">Edit User</a>
    </div>
  `;
}
