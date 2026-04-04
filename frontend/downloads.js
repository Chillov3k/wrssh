import {
  api,
  artifactPageHref,
  artifactSizeLabel,
  escapeAttribute,
  escapeHtml,
  initPage,
  loadArtifacts,
  markSynced
} from "./shared.js";

const artifactList = document.getElementById("artifactList");

const pageState = {
  artifacts: [],
  ctx: null
};

artifactList.addEventListener("click", async (event) => {
  const deleteButton = event.target.closest("[data-delete-artifact]");
  if (deleteButton) {
    await deleteArtifact(deleteButton);
    return;
  }

  const row = event.target.closest("[data-open-artifact]");
  if (!row || event.target.closest("a,button")) {
    return;
  }

  window.location.href = row.dataset.openArtifact;
});

initPage({
  title: "Downloads",
  requireProject: true,
  load: async (ctx) => {
    pageState.ctx = ctx;
    await refreshArtifacts(ctx);
  }
}).catch((error) => {
  console.error(error);
});

async function refreshArtifacts(ctx) {
  pageState.artifacts = await loadArtifacts();
  renderArtifacts();
  markSynced(ctx, `${ctx.project} · ${pageState.artifacts.length} artifact${pageState.artifacts.length === 1 ? "" : "s"}`);
}

function renderArtifacts() {
  if (!pageState.artifacts.length) {
    artifactList.innerHTML = `<div class="empty-state"><p>No artifacts available yet.</p></div>`;
    return;
  }

  artifactList.innerHTML = `
    <div class="table-toolbar">
      <div>
        <p class="eyebrow">Artifacts</p>
        <h3>${pageState.artifacts.length} download${pageState.artifacts.length === 1 ? "" : "s"}</h3>
      </div>
    </div>
    <div class="table-scroll">
      <div class="client-table artifact-table">
        <div class="client-table-head artifact-table-head">
          <span>Name</span>
          <span>Platform</span>
          <span>Callback</span>
          <span>Stats</span>
          <span>Actions</span>
        </div>
        <div class="client-table-body">
          ${pageState.artifacts.map((artifact) => renderRow(artifact)).join("")}
        </div>
      </div>
    </div>
  `;
}

function renderRow(artifact) {
  const href = artifactPageHref(artifact.urlPath, pageState.ctx?.project || "");
  return `
    <article class="client-row artifact-row" data-open-artifact="${escapeAttribute(href)}">
      <div class="client-cell">
        <strong>${escapeHtml(artifact.urlPath)}</strong>
        <span class="muted wrap-anywhere">${escapeHtml(artifact.downloadUrl)}</span>
      </div>
      <div class="client-cell">
        <strong>${escapeHtml(artifact.goos || "-")} / ${escapeHtml(artifact.goarch || "-")}</strong>
        <span class="muted">${escapeHtml(artifact.fileType || "-")}</span>
      </div>
      <div class="client-cell">
        <strong class="wrap-anywhere">${escapeHtml(artifact.callbackAddress || "-")}</strong>
        <span class="muted">${escapeHtml(artifact.workingDirectory || "-")}</span>
      </div>
      <div class="client-cell">
        <span>Hits ${escapeHtml(String(artifact.hits))}</span>
        <span class="muted">Size ${escapeHtml(artifactSizeLabel(artifact.fileSizeMb))} MB</span>
      </div>
      <div class="client-cell client-actions">
        <a class="action-button action-link" href="${escapeAttribute(href)}">Open</a>
        <button class="danger-button" data-delete-artifact="${escapeAttribute(artifact.urlPath)}">Delete</button>
      </div>
    </article>
  `;
}

async function deleteArtifact(button) {
  const urlPath = button.dataset.deleteArtifact || "";
  if (!window.confirm(`Delete artifact ${urlPath}?`)) {
    return;
  }

  const original = button.textContent;
  button.disabled = true;
  button.textContent = "Deleting...";

  try {
    await api(`/api/artifacts/${encodeURIComponent(urlPath)}?project=${encodeURIComponent(pageState.ctx?.project || "")}`, {
      method: "DELETE"
    });
    await refreshArtifacts(pageState.ctx);
  } catch (error) {
    button.disabled = false;
    button.textContent = original;
    window.alert(error.message);
  }
}
