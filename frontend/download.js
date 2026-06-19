import {
  api,
  copyFromButton,
  escapeHtml,
  initPage,
  loadArtifact,
  markSynced,
  renderArtifactLink,
  viewHref,
  withProjectQuery
} from "./shared.js";

const search = new URLSearchParams(window.location.search);

const elements = {
  artifactDetailEmpty: document.getElementById("artifactDetailEmpty"),
  artifactDetailContent: document.getElementById("artifactDetailContent"),
  artifactName: document.getElementById("artifactName"),
  artifactSubline: document.getElementById("artifactSubline"),
  artifactTypeBadge: document.getElementById("artifactTypeBadge"),
  artifactGoos: document.getElementById("artifactGoos"),
  artifactGoarch: document.getElementById("artifactGoarch"),
  artifactCallback: document.getElementById("artifactCallback"),
  artifactWorkingDirectory: document.getElementById("artifactWorkingDirectory"),
  artifactHits: document.getElementById("artifactHits"),
  artifactSize: document.getElementById("artifactSize"),
  artifactLogLevel: document.getElementById("artifactLogLevel"),
  artifactHostHeader: document.getElementById("artifactHostHeader"),
  downloadBinaryLink: document.getElementById("downloadBinaryLink"),
  copyDownloadUrlButton: document.getElementById("copyDownloadUrlButton"),
  artifactDownloadUrl: document.getElementById("artifactDownloadUrl"),
  artifactLinks: document.getElementById("artifactLinks"),
  artifactAdminActions: document.getElementById("artifactAdminActions"),
  deleteArtifactButton: document.getElementById("deleteArtifactButton"),
  artifactDeleteOverlay: document.getElementById("artifactDeleteOverlay"),
  artifactDeleteMessage: document.getElementById("artifactDeleteMessage"),
  artifactDeleteError: document.getElementById("artifactDeleteError"),
  confirmArtifactDeleteButton: document.getElementById("confirmArtifactDeleteButton"),
  cancelArtifactDeleteButton: document.getElementById("cancelArtifactDeleteButton")
};

const pageState = {
  artifact: null,
  ctx: null,
  pendingArtifactDelete: null
};

elements.artifactLinks.addEventListener("click", async (event) => {
  const button = event.target.closest("[data-copy-artifact]");
  if (!button) {
    return;
  }

  await copyFromButton(button, button.dataset.copyArtifact || "");
});

elements.copyDownloadUrlButton.addEventListener("click", async () => {
  if (!pageState.artifact) {
    return;
  }

  await copyFromButton(elements.copyDownloadUrlButton, pageState.artifact.downloadUrl, "URL Copied");
});

elements.deleteArtifactButton.addEventListener("click", async () => {
  if (!pageState.artifact) {
    return;
  }

  openArtifactDeleteModal(pageState.artifact.urlPath);
});

elements.artifactDeleteOverlay.addEventListener("click", (event) => {
  if (event.target === elements.artifactDeleteOverlay) {
    closeArtifactDeleteModal();
  }
});
elements.cancelArtifactDeleteButton.addEventListener("click", () => closeArtifactDeleteModal());
elements.confirmArtifactDeleteButton.addEventListener("click", deleteArtifactFromModal);

document.addEventListener("keydown", (event) => {
  if (event.key === "Escape" && elements.artifactDeleteOverlay.classList.contains("visible")) {
    closeArtifactDeleteModal();
  }
});

function openArtifactDeleteModal(urlPath) {
  urlPath = String(urlPath || "").trim();
  if (!urlPath) {
    return;
  }

  pageState.pendingArtifactDelete = { urlPath };
  elements.artifactDeleteError.textContent = "";
  elements.artifactDeleteMessage.textContent = `Delete agent "${urlPath}" from project ${pageState.ctx?.project || "-"}?`;
  elements.confirmArtifactDeleteButton.textContent = "Delete this agent";
  elements.confirmArtifactDeleteButton.disabled = false;
  elements.cancelArtifactDeleteButton.disabled = false;
  elements.artifactDeleteOverlay.classList.add("visible");
  elements.confirmArtifactDeleteButton.focus();
}

function closeArtifactDeleteModal(force = false) {
  if (!elements.artifactDeleteOverlay.classList.contains("visible")) {
    return;
  }
  if (!force && elements.cancelArtifactDeleteButton.disabled) {
    return;
  }

  pageState.pendingArtifactDelete = null;
  elements.artifactDeleteOverlay.classList.remove("visible");
  elements.artifactDeleteError.textContent = "";
  elements.confirmArtifactDeleteButton.textContent = "Delete this agent";
  elements.confirmArtifactDeleteButton.disabled = false;
  elements.cancelArtifactDeleteButton.disabled = false;
}

async function deleteArtifactFromModal() {
  const urlPath = pageState.pendingArtifactDelete?.urlPath || "";
  if (!urlPath) {
    return;
  }

  elements.artifactDeleteError.textContent = "";
  elements.confirmArtifactDeleteButton.disabled = true;
  elements.cancelArtifactDeleteButton.disabled = true;
  elements.confirmArtifactDeleteButton.textContent = "Deleting Agent...";

  try {
    await api(withProjectQuery(`/api/artifacts/${encodeURIComponent(urlPath)}`, pageState.ctx?.project || ""), {
      method: "DELETE"
    });
    closeArtifactDeleteModal(true);
    window.location.href = viewHref("downloads", pageState.ctx?.project || "");
  } catch (error) {
    elements.artifactDeleteError.textContent = error.message;
    elements.confirmArtifactDeleteButton.textContent = "Delete this agent";
    elements.confirmArtifactDeleteButton.disabled = false;
    elements.cancelArtifactDeleteButton.disabled = false;
  }
}

initPage({
  title: "Download Detail",
  requireProject: true,
  load: async (ctx) => {
    pageState.ctx = ctx;
    document.getElementById("backToDownloadsLink").href = viewHref("downloads", ctx.project);

    const urlPath = currentUrlPath();
    if (!urlPath) {
      renderEmpty("No artifact was selected. Open one from the Downloads page.");
      markSynced(ctx, "no artifact selected");
      return;
    }

    try {
      pageState.artifact = await loadArtifact(urlPath, ctx.project);
    } catch (error) {
      renderEmpty(error.message || "Artifact not found.");
      markSynced(ctx, "artifact not found");
      return;
    }

    renderArtifact();
    markSynced(ctx, `${ctx.project} · ${pageState.artifact.urlPath}`);
  }
}).catch((error) => {
  console.error(error);
});

function currentUrlPath() {
  return search.get("urlPath") || "";
}

function renderEmpty(message) {
  pageState.artifact = null;
  elements.artifactDetailEmpty.classList.remove("hidden");
  elements.artifactDetailContent.classList.add("hidden");
  elements.artifactDetailEmpty.innerHTML = `<p>${escapeHtml(message)}</p>`;
}

function renderArtifact() {
  const artifact = pageState.artifact;
  if (!artifact) {
    renderEmpty("Artifact not found.");
    return;
  }

  elements.artifactDetailEmpty.classList.add("hidden");
  elements.artifactDetailContent.classList.remove("hidden");
  elements.artifactAdminActions.classList.remove("hidden");

  elements.artifactName.textContent = artifact.urlPath;
  elements.artifactSubline.textContent = [
    `${artifact.goos || "-"} / ${goarchLabel(artifact)}`,
    artifact.fileType || "-",
    artifact.version || "unknown version"
  ].join(" · ");
  elements.artifactTypeBadge.textContent = artifact.fileType || "-";
  elements.artifactGoos.textContent = artifact.goos || "-";
  elements.artifactGoarch.textContent = goarchLabel(artifact);
  elements.artifactCallback.textContent = artifact.callbackAddress || "-";
  elements.artifactWorkingDirectory.textContent = artifact.workingDirectory || "-";
  elements.artifactHits.textContent = String(artifact.hits ?? 0);
  elements.artifactSize.textContent = `${Number(artifact.fileSizeMb || 0).toFixed(2)} MB`;
  elements.artifactLogLevel.textContent = artifact.logLevel || "-";
  elements.artifactHostHeader.textContent = artifact.useHostHeader ? "enabled" : "disabled";
  elements.downloadBinaryLink.href = artifact.downloadUrl;
  elements.artifactDownloadUrl.textContent = artifact.downloadUrl || "-";

  elements.artifactLinks.innerHTML = artifactDownloadLinks(artifact).map(([label, url, command]) => renderArtifactLink(label, url, command ? {
    displayText: command,
    copyText: command
  } : {})).join("");
}

function goarchLabel(artifact) {
  if (!artifact?.goarch) {
    return "-";
  }

  if (artifact.goarch === "arm" && artifact.goarm) {
    return `${artifact.goarch} (arm${artifact.goarm})`;
  }

  return artifact.goarch;
}

function powerShellInlineCommand(url) {
  const value = String(url || "").trim();
  return value ? `iwr ${value} -UseBasicParsing | iex` : "";
}

function artifactDownloadLinks(artifact) {
  const goos = String(artifact?.goos || "").toLowerCase();
  const links = [
    ["Binary URL", artifact.downloadUrl]
  ];

  if (goos !== "windows") {
    links.push(["Bash", artifact.templateShellUrl]);
  }

  links.push(["Python", artifact.templatePythonUrl]);

  if (goos === "windows") {
    links.push(["PowerShell", artifact.templatePs1Url, powerShellInlineCommand(artifact.templatePs1Url)]);
  }

  return links.filter(([, url]) => Boolean(url));
}
