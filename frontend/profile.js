import {
  api,
  escapeHtml,
  initPage,
  loadProfile,
  markSynced,
  viewHref
} from "./shared.js";

const elements = {
  profileNotice: document.getElementById("profileNotice"),
  profileUsername: document.getElementById("profileUsername"),
  profileRole: document.getElementById("profileRole"),
  profileKeysForm: document.getElementById("profileKeysForm"),
  profileAuthorizedKeysInput: document.getElementById("profileAuthorizedKeysInput"),
  saveProfileKeysButton: document.getElementById("saveProfileKeysButton"),
  profilePasswordForm: document.getElementById("profilePasswordForm"),
  profileCurrentPasswordInput: document.getElementById("profileCurrentPasswordInput"),
  profileNewPasswordInput: document.getElementById("profileNewPasswordInput"),
  profileConfirmPasswordInput: document.getElementById("profileConfirmPasswordInput"),
  profilePasswordMessage: document.getElementById("profilePasswordMessage"),
  saveProfilePasswordButton: document.getElementById("saveProfilePasswordButton"),
  profileOutput: document.getElementById("profileOutput")
};

const pageState = {
  ctx: null,
  profile: null
};

elements.profileKeysForm.addEventListener("submit", saveSSHKeys);
elements.profilePasswordForm.addEventListener("submit", changePassword);

initPage({
  title: "Profile",
  load: async (ctx) => {
    pageState.ctx = ctx;
    await refreshProfile();
  }
}).catch((error) => {
  console.error(error);
});

async function refreshProfile() {
  pageState.profile = await loadProfile();
  renderProfile();
  markSynced(pageState.ctx, `profile ${pageState.profile.username}`);
}

function renderProfile() {
  const profile = pageState.profile;
  elements.profileUsername.textContent = profile.username || "-";
  elements.profileRole.textContent = profile.role || "-";
  elements.profileAuthorizedKeysInput.value = profile.sshAuthorizedKeys || "";
  elements.profileNotice.classList.toggle("hidden", !profile.mustChangePassword);
  clearPasswordMessage();
}

async function saveSSHKeys(event) {
  event.preventDefault();

  elements.saveProfileKeysButton.disabled = true;
  renderStatus("Saving SSH keys...");

  try {
    pageState.profile = await api("/api/profile", {
      method: "PATCH",
      body: JSON.stringify({
        sshAuthorizedKeys: elements.profileAuthorizedKeysInput.value
      })
    });
    renderProfile();
    renderStatus("SSH keys updated.");
  } catch (error) {
    renderStatus(error.message, true);
  } finally {
    elements.saveProfileKeysButton.disabled = false;
  }
}

async function changePassword(event) {
  event.preventDefault();

  const currentPassword = elements.profileCurrentPasswordInput.value;
  const newPassword = elements.profileNewPasswordInput.value;
  const confirmPassword = elements.profileConfirmPasswordInput.value;

  clearPasswordMessage();
  if (newPassword !== confirmPassword) {
    renderPasswordMessage("New password and confirmation must match.", true);
    return;
  }
  if (!isStrongPassword(newPassword)) {
    renderPasswordMessage("Password must be at least 10 characters and include uppercase, lowercase, and a special character.", true);
    return;
  }

  elements.saveProfilePasswordButton.disabled = true;
  renderPasswordMessage("Changing password...");

  try {
    const wasTemporary = Boolean(pageState.profile?.mustChangePassword);
    pageState.profile = await api("/api/profile", {
      method: "PATCH",
      body: JSON.stringify({
        currentPassword,
        newPassword,
        confirmPassword
      })
    });
    elements.profilePasswordForm.reset();
    renderProfile();
    renderPasswordMessage("Password changed.");
    if (wasTemporary) {
      window.location.href = viewHref("projects");
    }
  } catch (error) {
    renderPasswordMessage(error.message, true);
  } finally {
    elements.saveProfilePasswordButton.disabled = false;
  }
}

function renderStatus(message, isError = false) {
  elements.profileOutput.classList.remove("hidden");
  elements.profileOutput.classList.toggle("error-note", isError);
  elements.profileOutput.classList.toggle("warning-note", !isError && Boolean(pageState.profile?.mustChangePassword));
  elements.profileOutput.innerHTML = `<p class="${isError ? "error-text" : "muted"}">${escapeHtml(message)}</p>`;
}

function isStrongPassword(password) {
  return password.length >= 10 && /[A-Z]/.test(password) && /[a-z]/.test(password) && /[^A-Za-z0-9]/.test(password);
}

function renderPasswordMessage(message, isError = false) {
  elements.profilePasswordMessage.classList.remove("hidden", "error-text", "muted");
  elements.profilePasswordMessage.classList.add(isError ? "error-text" : "muted");
  elements.profilePasswordMessage.textContent = message;
}

function clearPasswordMessage() {
  elements.profilePasswordMessage.classList.add("hidden");
  elements.profilePasswordMessage.classList.remove("error-text", "muted");
  elements.profilePasswordMessage.textContent = "";
}
