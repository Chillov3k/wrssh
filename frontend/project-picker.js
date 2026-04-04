export class ProjectPicker {
  constructor(root, { placeholder = "Select projects" } = {}) {
    this.root = root;
    this.placeholder = placeholder;
    this.options = [];
    this.selected = new Set();
    this.query = "";

    this.handleDocumentClick = this.handleDocumentClick.bind(this);

    this.trigger = document.createElement("button");
    this.trigger.type = "button";
    this.trigger.className = "project-picker-trigger";
    this.trigger.addEventListener("click", () => this.toggle());

    this.menu = document.createElement("div");
    this.menu.className = "project-picker-menu hidden";

    this.search = document.createElement("input");
    this.search.type = "search";
    this.search.className = "project-picker-search";
    this.search.placeholder = "Search projects";
    this.search.addEventListener("input", () => {
      this.query = this.search.value.trim().toLowerCase();
      this.renderOptions();
    });

    this.optionsBox = document.createElement("div");
    this.optionsBox.className = "project-picker-options";

    this.menu.appendChild(this.search);
    this.menu.appendChild(this.optionsBox);
    this.root.innerHTML = "";
    this.root.appendChild(this.trigger);
    this.root.appendChild(this.menu);

    document.addEventListener("click", this.handleDocumentClick);
    this.renderTrigger();
    this.renderOptions();
  }

  setOptions(items) {
    this.options = Array.isArray(items)
      ? items.map((item) => String(item || "").trim()).filter(Boolean)
      : [];

    const allowed = new Set(this.options);
    this.selected = new Set([...this.selected].filter((value) => allowed.has(value)));
    this.renderTrigger();
    this.renderOptions();
  }

  setSelected(values) {
    const allowed = new Set(this.options);
    this.selected = new Set(
      (Array.isArray(values) ? values : [])
        .map((value) => String(value || "").trim())
        .filter((value) => value && allowed.has(value))
    );
    this.renderTrigger();
    this.renderOptions();
  }

  selectedValues() {
    return [...this.selected];
  }

  toggle() {
    if (this.trigger.disabled) {
      return;
    }
    this.menu.classList.toggle("hidden");
    if (!this.menu.classList.contains("hidden")) {
      this.search.focus();
      this.search.select();
    }
  }

  close() {
    this.menu.classList.add("hidden");
  }

  handleDocumentClick(event) {
    if (!this.root.contains(event.target)) {
      this.close();
    }
  }

  renderTrigger() {
    if (!this.options.length) {
      this.trigger.textContent = "No projects available";
      this.trigger.disabled = true;
      return;
    }

    this.trigger.disabled = false;
    const selected = this.selectedValues();
    if (!selected.length) {
      this.trigger.textContent = this.placeholder;
      return;
    }

    if (selected.length <= 2) {
      this.trigger.textContent = selected.join(", ");
      return;
    }

    this.trigger.textContent = `${selected.length} projects selected`;
  }

  renderOptions() {
    this.optionsBox.innerHTML = "";

    const filtered = this.options.filter((item) => item.toLowerCase().includes(this.query));
    if (!filtered.length) {
      const empty = document.createElement("p");
      empty.className = "muted checklist-empty";
      empty.textContent = this.options.length ? "No projects match the current search." : "No projects exist yet.";
      this.optionsBox.appendChild(empty);
      return;
    }

    filtered.forEach((project) => {
      const option = document.createElement("label");
      option.className = "project-picker-option";

      const checkbox = document.createElement("input");
      checkbox.type = "checkbox";
      checkbox.checked = this.selected.has(project);
      checkbox.addEventListener("change", () => {
        if (checkbox.checked) {
          this.selected.add(project);
        } else {
          this.selected.delete(project);
        }
        this.renderTrigger();
      });

      const text = document.createElement("span");
      text.textContent = project;

      option.appendChild(checkbox);
      option.appendChild(text);
      this.optionsBox.appendChild(option);
    });
  }
}
