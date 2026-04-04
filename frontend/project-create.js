import {
  api,
  initPage,
  markSynced,
  viewHref
} from "./shared.js";

const form = document.getElementById("projectForm");
const projectNameInput = document.getElementById("projectNameInput");
const projectDescriptionInput = document.getElementById("projectDescriptionInput");
const projectTagsInput = document.getElementById("projectTagsInput");
const projectSubmitButton = document.getElementById("projectSubmitButton");
const projectFormOutput = document.getElementById("projectFormOutput");

const pageState = {
  ctx: null
};

form.addEventListener("submit", createProject);

initPage({
  title: "Create Project",
  load: async (ctx) => {
    pageState.ctx = ctx;
    if (!ctx.user.canCreateProjects) {
      projectSubmitButton.disabled = true;
      projectFormOutput.textContent = "Project creation is not allowed for this user.";
      return;
    }
    markSynced(ctx, "ready to create project");
  }
}).catch((error) => {
  console.error(error);
});

async function createProject(event) {
  event.preventDefault();

  if (!pageState.ctx?.user?.canCreateProjects) {
    return;
  }

  const payload = {
    name: projectNameInput.value.trim(),
    description: projectDescriptionInput.value.trim(),
    tags: parseTags(projectTagsInput.value)
  };

  projectFormOutput.textContent = "Creating project...";

  try {
    await api("/api/projects", {
      method: "POST",
      body: JSON.stringify(payload)
    });
    window.location.href = viewHref("projects");
  } catch (error) {
    projectFormOutput.textContent = error.message;
  }
}

function parseTags(raw) {
  return raw.split(",").map((value) => value.trim()).filter(Boolean);
}
