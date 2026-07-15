(() => {
  const projectTypes = new Set([
    "project", "agent", "memory", "instruction", "skill", "plan", "spec", "activecontext",
  ]);

  function setField(field, visible) {
    if (!field) return;
    field.hidden = !visible;
    for (const input of field.querySelectorAll("input, select")) {
      input.disabled = !visible;
    }
  }

  function sync(form) {
    const type = form.querySelector("[data-document-type]")?.value || "free";
    const project = form.querySelector("[data-document-project]");
    const date = form.querySelector("[data-document-date]");
    const path = form.querySelector("[data-document-path]");
    const nodeMirror = form.querySelector('[name="node"]');
    const isDaily = type === "daily";

    if (form.dataset.documentType === "daily" && !isDaily && /^daily\/\d{4}-\d{2}-\d{2}$/.test(path?.value || "")) {
      path.value = "";
    }

    setField(form.querySelector('[data-metadata-field="project"]'), projectTypes.has(type));
    setField(form.querySelector('[data-metadata-field="date"]'), isDaily);
    setField(form.querySelector('[data-metadata-field="path"]'), !isDaily);
    setField(form.querySelector('[data-metadata-field="derived-path"]'), isDaily);

    if (project) project.required = type === "project";
    if (date) date.required = isDaily;
    if (path) path.required = !isDaily;
    if (nodeMirror) nodeMirror.value = project?.disabled ? "" : project?.value || "";

    const derived = form.querySelector("[data-derived-path]");
    if (derived) derived.textContent = date?.value ? `daily/${date.value}` : "daily/YYYY-MM-DD";
    form.dataset.documentType = type;
  }

  function init(root = document) {
    for (const form of root.querySelectorAll("[data-document-metadata]")) {
      if (form.dataset.metadataReady === "true") continue;
      form.dataset.metadataReady = "true";
      form.addEventListener("change", () => sync(form));
      sync(form);
    }
  }

  document.addEventListener("DOMContentLoaded", () => init());
  document.addEventListener("htmx:afterSwap", (event) => init(event.target));
})();
