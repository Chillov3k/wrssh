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
  deleteArtifactButton: document.getElementById("deleteArtifactButton")
};

const pageState = {
  artifact: null,
  ctx: null
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

  const urlPath = pageState.artifact.urlPath;
  if (!window.confirm(`Delete artifact ${urlPath}?`)) {
    return;
  }

  const original = elements.deleteArtifactButton.textContent;
  elements.deleteArtifactButton.disabled = true;
  elements.deleteArtifactButton.textContent = "Deleting...";

  try {
    await api(withProjectQuery(`/api/artifacts/${encodeURIComponent(urlPath)}`, pageState.ctx?.project || ""), {
      method: "DELETE"
    });
    window.location.href = viewHref("downloads", pageState.ctx?.project || "");
  } catch (error) {
    elements.deleteArtifactButton.disabled = false;
    elements.deleteArtifactButton.textContent = original;
    window.alert(error.message);
  }
});

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

  elements.artifactLinks.innerHTML = [
    ["Binary URL", artifact.downloadUrl],
    ["Bash", artifact.templateShellUrl],
    ["Python", artifact.templatePythonUrl],
    ["PowerShell", artifact.templatePs1Url]
  ].filter(([, url]) => Boolean(url)).map(([label, url]) => renderArtifactLink(label, url)).join("");
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
