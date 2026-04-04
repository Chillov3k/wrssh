import {
  api,
  copyFromButton,
  escapeAttribute,
  escapeHtml,
  initPage,
  loadProjects,
  loadUser,
  markSynced,
  viewHref
} from "./shared.js";
import { ProjectPicker } from "./project-picker.js";

const search = new URLSearchParams(window.location.search);
const targetUsername = search.get("username") || "";

const elements = {
  userEditNoAccess: document.getElementById("userEditNoAccess"),
  userEditPanel: document.getElementById("userEditPanel"),
  userEditEmpty: document.getElementById("userEditEmpty"),
  backToUsersLink: document.getElementById("backToUsersLink"),
  editHeading: document.getElementById("editUserHeading"),
  editForm: document.getElementById("editUserForm"),
  editUsernameInput: document.getElementById("editUsernameInput"),
  editRSSHUsernameInput: document.getElementById("editRSSHUsernameInput"),
  editCanCreateProjects: document.getElementById("editCanCreateProjects"),
  editProjectsPicker: document.getElementById("editProjectsPicker"),
  editAuthorizedKeysInput: document.getElementById("editAuthorizedKeysInput"),
  editUserOutput: document.getElementById("editUserOutput"),
  saveUserButton: document.getElementById("saveUserButton"),
  resetPasswordButton: document.getElementById("resetPasswordButton"),
  deleteUserButton: document.getElementById("deleteUserButton")
};

const pageState = {
  ctx: null,
  projects: [],
  user: null,
  projectPicker: new ProjectPicker(elements.editProjectsPicker, { placeholder: "Choose allowed projects" })
};

elements.editForm.addEventListener("submit", saveUser);
elements.resetPasswordButton.addEventListener("click", resetPassword);
elements.deleteUserButton.addEventListener("click", deleteUser);
elements.editUserOutput.addEventListener("click", async (event) => {
  const button = event.target.closest("[data-copy-secret]");
  if (!button) {
    return;
  }

  await copyFromButton(button, button.dataset.copySecret || "", "Password Copied");
});

initPage({
  title: "Edit User",
  load: async (ctx) => {
    pageState.ctx = ctx;
    elements.backToUsersLink.href = viewHref("users");

    if (ctx.user.role !== "admin") {
      elements.userEditNoAccess.classList.remove("hidden");
      elements.userEditPanel.classList.add("hidden");
      markSynced(ctx, "user management unavailable");
      return;
    }

    elements.userEditNoAccess.classList.add("hidden");
    elements.userEditPanel.classList.remove("hidden");

    if (!targetUsername) {
      renderEmpty("No user selected.");
      markSynced(ctx, "no managed user selected");
      return;
    }

    await refreshUser();
  }
}).catch((error) => {
  console.error(error);
});

async function refreshUser() {
  try {
    const [projects, user] = await Promise.all([loadProjects(), loadUser(targetUsername)]);
    pageState.projects = projects;
    pageState.user = user;
    renderUser();
    markSynced(pageState.ctx, `editing ${user.username}`);
  } catch (error) {
    renderEmpty(error.message || "Managed user not found.");
    markSynced(pageState.ctx, "managed user unavailable");
  }
}

function renderEmpty(message) {
  pageState.user = null;
  elements.userEditEmpty.classList.remove("hidden");
  elements.editForm.classList.add("hidden");
  elements.userEditEmpty.innerHTML = `<p>${escapeHtml(message)}</p>`;
}

function renderUser() {
  const user = pageState.user;
  if (!user) {
    renderEmpty("Managed user not found.");
    return;
  }

  elements.userEditEmpty.classList.add("hidden");
  elements.editForm.classList.remove("hidden");
  elements.editHeading.textContent = `Edit ${user.username}`;
  elements.editUsernameInput.value = user.username || "";
  elements.editRSSHUsernameInput.value = user.rsshUsername || user.username || "";
  elements.editCanCreateProjects.checked = Boolean(user.canCreateProjects);
  elements.editAuthorizedKeysInput.value = user.sshAuthorizedKeys || "";
  pageState.projectPicker.setOptions(pageState.projects.map((project) => project.name));
  pageState.projectPicker.setSelected(user.allowedProjects || []);
}

function selectedProjects() {
  return pageState.projectPicker.selectedValues();
}

async function saveUser(event) {
  event.preventDefault();
  if (!pageState.user) {
    return;
  }

  elements.saveUserButton.disabled = true;
  renderStatus("Saving user...");

  try {
    await api(`/api/users/${encodeURIComponent(pageState.user.username)}`, {
      method: "PATCH",
      body: JSON.stringify({
        canCreateProjects: elements.editCanCreateProjects.checked,
        allowedProjects: selectedProjects(),
        sshAuthorizedKeys: elements.editAuthorizedKeysInput.value
      })
    });
    await refreshUser();
    renderStatus(`User ${pageState.user.username} updated.`);
  } catch (error) {
    renderStatus(error.message, true);
  } finally {
    elements.saveUserButton.disabled = false;
  }
}

async function resetPassword() {
  if (!pageState.user) {
    return;
  }

  elements.resetPasswordButton.disabled = true;
  renderStatus("Resetting password...");

  try {
    const updated = await api(`/api/users/${encodeURIComponent(pageState.user.username)}`, {
      method: "PATCH",
      body: JSON.stringify({ resetPassword: true })
    });
    await refreshUser();
    renderSecret(`Password reset for ${updated.username}. This temporary password must be changed on next login.`, updated.generatedPassword);
  } catch (error) {
    renderStatus(error.message, true);
  } finally {
    elements.resetPasswordButton.disabled = false;
  }
}

async function deleteUser() {
  if (!pageState.user) {
    return;
  }

  if (!window.confirm(`Delete user ${pageState.user.username}? Their credentials and project grants will be removed.`)) {
    return;
  }

  elements.deleteUserButton.disabled = true;
  renderStatus("Deleting user...");

  try {
    await api(`/api/users/${encodeURIComponent(pageState.user.username)}`, {
      method: "DELETE"
    });
    window.location.href = viewHref("users");
  } catch (error) {
    renderStatus(error.message, true);
    elements.deleteUserButton.disabled = false;
  }
}

function renderStatus(message, isError = false) {
  elements.editUserOutput.classList.remove("hidden");
  elements.editUserOutput.classList.toggle("error-note", isError);
  elements.editUserOutput.innerHTML = `<p class="${isError ? "error-text" : "muted"}">${escapeHtml(message)}</p>`;
}

function renderSecret(message, password) {
  elements.editUserOutput.classList.remove("hidden");
  elements.editUserOutput.classList.remove("error-note");
  elements.editUserOutput.innerHTML = `
    <p class="muted">${escapeHtml(message)}</p>
    <div class="secret-row">
      <code>${escapeHtml(password || "-")}</code>
      ${password ? `<button class="ghost-button" data-copy-secret="${escapeAttribute(password)}">Copy Password</button>` : ""}
    </div>
  `;
}
