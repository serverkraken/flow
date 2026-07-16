/* wissen-select.js — bounded document selection for Wissen bulk curation. */
(function () {
  "use strict";

  if (window.flowWissenSelectBound) { return; }
  window.flowWissenSelectBound = true;

  var selecting = false;
  var selected = new Set();

  function all(selector) {
    return Array.prototype.slice.call(document.querySelectorAll(selector));
  }

  function rowID(row) {
    return row ? row.getAttribute("data-document-id") : "";
  }

  function setSelecting(on) {
    selecting = on;
    document.body.classList.toggle("wissen-selecting", on);
    all("[data-wissen-select-toggle]").forEach(function (button) {
      button.textContent = on ? button.getAttribute("data-label-done") : button.getAttribute("data-label-select");
    });
    if (!on) { selected.clear(); }
    paint();
  }

  function paint() {
    var visible = new Set();
    all("[data-wissen-row]").forEach(function (row) {
      var id = rowID(row);
      visible.add(id);
      var checked = selected.has(id);
      row.classList.toggle("bg-blue/5", checked);
      var checkbox = row.querySelector("[data-wissen-checkbox]");
      if (checkbox) {
        checkbox.classList.toggle("hidden", !selecting);
        checkbox.checked = checked;
      }
    });
    Array.from(selected).forEach(function (id) {
      if (!visible.has(id)) { selected.delete(id); }
    });
    var ids = Array.from(selected);
    all("[data-wissen-count]").forEach(function (node) { node.textContent = String(ids.length); });
    all("[data-wissen-ids]").forEach(function (input) { input.value = ids.join(","); });
    all("[data-wissen-action-bar]").forEach(function (bar) {
      bar.classList.toggle("hidden", !selecting || ids.length === 0);
    });
    var selectedRows = all("[data-wissen-row]").filter(function (row) {
      return selected.has(rowID(row));
    });
    var contextAllowed = ids.length > 0 && selectedRows.every(function (row) {
      return row.getAttribute("data-context-eligible") === "true" && row.getAttribute("data-archived") !== "true";
    });
    var hasActive = selectedRows.some(function (row) { return row.getAttribute("data-archived") !== "true"; });
    var hasArchived = selectedRows.some(function (row) { return row.getAttribute("data-archived") === "true"; });
    all("[data-wissen-context-action]").forEach(function (button) { button.disabled = !contextAllowed; });
    all("[data-wissen-submit]").forEach(function (button) {
      if (button.hasAttribute("data-wissen-context-action")) { return; }
      var action = button.getAttribute("data-wissen-action");
      button.disabled = ids.length === 0 || (action === "archive" && !hasActive) || (action === "restore" && !hasArchived);
    });
  }

  document.addEventListener("click", function (event) {
    var toggle = event.target.closest("[data-wissen-select-toggle]");
    if (toggle) { event.preventDefault(); setSelecting(!selecting); return; }
    if (event.target.closest("[data-wissen-cancel]")) { event.preventDefault(); setSelecting(false); return; }
    var row = event.target.closest("[data-wissen-row]");
    if (row && selecting && !event.target.closest("[data-wissen-checkbox]")) {
      event.preventDefault();
      var id = rowID(row);
      if (selected.has(id)) { selected.delete(id); } else { selected.add(id); }
      paint();
    }
  });

  document.addEventListener("change", function (event) {
    if (!event.target.matches("[data-wissen-checkbox]")) { return; }
    var id = rowID(event.target.closest("[data-wissen-row]"));
    if (event.target.checked) { selected.add(id); } else { selected.delete(id); }
    paint();
  });

  document.addEventListener("keydown", function (event) {
    if (event.key === "Escape" && selecting && !document.querySelector("dialog[open]")) {
      setSelecting(false);
    }
  });

  document.body.addEventListener("wissenBulkDone", function () { setSelecting(false); });
  document.body.addEventListener("htmx:afterSwap", paint);
})();
