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
  profileKeysList: document.getElementById("profileKeysList"),
  profileAuthorizedKeysInput: document.getElementById("profileAuthorizedKeysInput"),
  addProfileKeyButton: document.getElementById("addProfileKeyButton"),
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
  profile: null,
  sshKeys: []
};

elements.profileKeysForm.addEventListener("submit", saveSSHKeys);
elements.addProfileKeyButton.addEventListener("click", addSSHKeysFromInput);
elements.profileKeysList.addEventListener("click", (event) => {
  const removeButton = event.target.closest("[data-remove-key-index]");
  if (!removeButton) {
    return;
  }
  removeSSHKey(Number(removeButton.dataset.removeKeyIndex));
});
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
  pageState.sshKeys = parseAuthorizedKeys(profile.sshAuthorizedKeys || "");
  elements.profileAuthorizedKeysInput.value = "";
  renderSSHKeys();
  elements.profileNotice.classList.toggle("hidden", !profile.mustChangePassword);
  clearPasswordMessage();
}

function renderSSHKeys() {
  if (!pageState.sshKeys.length) {
    elements.profileKeysList.innerHTML = `<div class="ssh-key-empty">No SSH keys added yet.</div>`;
    return;
  }

  elements.profileKeysList.innerHTML = pageState.sshKeys.map((key, index) => {
    const details = describeAuthorizedKey(key);
    return `
      <article class="ssh-key-card">
        <div class="ssh-key-card-main">
          <p class="eyebrow">Key ${index + 1}</p>
          <strong>${escapeHtml(details.label)}</strong>
          <code>${escapeHtml(shortenAuthorizedKey(key))}</code>
        </div>
        <button class="ghost-button ssh-key-remove" type="button" data-remove-key-index="${index}">Remove</button>
      </article>
    `;
  }).join("");
}

function addSSHKeysFromInput() {
  const pendingKeys = parseAuthorizedKeys(elements.profileAuthorizedKeysInput.value);
  if (!pendingKeys.length) {
    renderStatus("Paste at least one SSH public key first.", true);
    return false;
  }

  let added = 0;
  const known = new Set(pageState.sshKeys);
  pendingKeys.forEach((key) => {
    if (known.has(key)) {
      return;
    }
    pageState.sshKeys.push(key);
    known.add(key);
    added++;
  });

  elements.profileAuthorizedKeysInput.value = "";
  renderSSHKeys();
  renderStatus(added > 0 ? `${added} SSH key${added === 1 ? "" : "s"} staged. Save to apply.` : "These SSH keys are already in your profile.");
  return added > 0;
}

function removeSSHKey(index) {
  if (!Number.isInteger(index) || index < 0 || index >= pageState.sshKeys.length) {
    return;
  }

  pageState.sshKeys.splice(index, 1);
  renderSSHKeys();
  renderStatus("SSH key removed from the staged list. Save to apply.");
}

async function saveSSHKeys(event) {
  event.preventDefault();

  const pending = parseAuthorizedKeys(elements.profileAuthorizedKeysInput.value);
  if (pending.length) {
    const known = new Set(pageState.sshKeys);
    pending.forEach((key) => {
      if (!known.has(key)) {
        pageState.sshKeys.push(key);
        known.add(key);
      }
    });
    elements.profileAuthorizedKeysInput.value = "";
    renderSSHKeys();
  }

  elements.saveProfileKeysButton.disabled = true;
  renderStatus("Saving SSH keys...");

  try {
    pageState.profile = await api("/api/profile", {
      method: "PATCH",
      body: JSON.stringify({
        sshAuthorizedKeys: pageState.sshKeys.join("\n")
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

function parseAuthorizedKeys(raw) {
  return String(raw || "")
    .replace(/\r\n/g, "\n")
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean);
}

function describeAuthorizedKey(key) {
  const parts = key.split(/\s+/);
  const keyType = parts[0] || "ssh key";
  const comment = parts.slice(2).join(" ").trim();
  return {
    type: keyType,
    label: comment || keyType
  };
}

function shortenAuthorizedKey(key) {
  const parts = key.split(/\s+/);
  const keyType = parts[0] || "ssh-key";
  const body = parts[1] || "";
  const suffix = body.length > 16 ? `${body.slice(0, 12)}...${body.slice(-8)}` : body;
  return `${keyType} ${suffix}`.trim();
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
