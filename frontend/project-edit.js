import {
  api,
  initPage,
  loadProject,
  markSynced,
  viewHref
} from "./shared.js";

const search = new URLSearchParams(window.location.search);
const projectQueryName = search.get("name") || "";

const form = document.getElementById("projectForm");
const projectNameInput = document.getElementById("projectNameInput");
const projectDescriptionInput = document.getElementById("projectDescriptionInput");
const projectTagsInput = document.getElementById("projectTagsInput");
const projectSubmitButton = document.getElementById("projectSubmitButton");
const projectDeleteButton = document.getElementById("projectDeleteButton");
const projectFormOutput = document.getElementById("projectFormOutput");

const pageState = {
  ctx: null,
  project: null
};

form.addEventListener("submit", saveProject);
projectDeleteButton.addEventListener("click", deleteProject);

initPage({
  title: "Edit Project",
  load: async (ctx) => {
    pageState.ctx = ctx;
    if (!projectQueryName) {
      projectSubmitButton.disabled = true;
      projectDeleteButton.disabled = true;
      projectFormOutput.textContent = "Project name is required.";
      return;
    }

    pageState.project = await loadProject(projectQueryName);
    renderProject();
    markSynced(ctx, `editing ${pageState.project.name}`);
  }
}).catch((error) => {
  console.error(error);
});

function renderProject() {
  const project = pageState.project;
  if (!project) {
    return;
  }

  projectNameInput.value = project.name || "";
  projectDescriptionInput.value = project.description || "";
  projectTagsInput.value = (project.tags || []).join(", ");
}

async function saveProject(event) {
  event.preventDefault();

  if (!pageState.project) {
    return;
  }

  const payload = {
    name: projectNameInput.value.trim(),
    description: projectDescriptionInput.value.trim(),
    tags: parseTags(projectTagsInput.value)
  };

  projectSubmitButton.disabled = true;
  projectFormOutput.textContent = "Saving project...";

  try {
    const project = await api(`/api/projects/${encodeURIComponent(pageState.project.name)}`, {
      method: "PATCH",
      body: JSON.stringify(payload)
    });
    window.location.href = viewHref("overview", project.name);
  } catch (error) {
    projectFormOutput.textContent = error.message;
    projectSubmitButton.disabled = false;
  }
}

async function deleteProject() {
  if (!pageState.project) {
    return;
  }

  const confirmText = `Delete project ${pageState.project.name}? All linked hosts and artifacts will be removed. Online clients will be killed first.`;
  if (!window.confirm(confirmText)) {
    return;
  }

  projectDeleteButton.disabled = true;
  projectFormOutput.textContent = "Deleting project...";

  try {
    await api(`/api/projects/${encodeURIComponent(pageState.project.name)}`, {
      method: "DELETE"
    });
    window.location.href = viewHref("projects");
  } catch (error) {
    projectFormOutput.textContent = error.message;
    projectDeleteButton.disabled = false;
  }
}

function parseTags(raw) {
  return raw.split(",").map((value) => value.trim()).filter(Boolean);
}
