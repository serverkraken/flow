/* historie-select.js — dependency-free bulk-selection for the Historie page.
 *
 * Drives select-mode (toggle), per-block / per-day / all-unassigned / whole-week
 * selection, the project fuzzy-picker (filter + pick + inline-create), and fills
 * the hidden ids/projectId/newProject fields on the SelectionActionBar form so
 * htmx posts the real selection to /ui/historie/reassign | /ui/historie/bulk-delete.
 *
 * Re-binds after htmx swaps (SSE/pagination re-render the fragment). Esc exits.
 * No native popups: the bulk-delete confirmation goes through the in-design
 * <dialog> ConfirmDialog (data-dialog-open) — this file never calls the native
 * browser confirm/alert/prompt dialogs.
 */
(function () {
  "use strict";

  var selectMode = false;
  var selected = new Set();

  function $(s, r) { return (r || document).querySelector(s); }
  function $all(s, r) { return Array.prototype.slice.call((r || document).querySelectorAll(s)); }

  function setMode(on) {
    selectMode = on;
    document.body.classList.toggle("is-selecting", on);
    var bar = $("#actionBar");
    if (bar) { bar.classList.toggle("hidden", !on); }
    $all("[data-select-toggle]").forEach(function (b) {
      var done = b.getAttribute("data-label-done");
      var sel = b.getAttribute("data-label-select");
      if (done && sel) {
        var icon = on ? "✓" : "▎";
        b.innerHTML = '<span aria-hidden="true">' + icon + "</span> " + (on ? done : sel);
      }
    });
    $all(".row-chk").forEach(function (c) { c.classList.toggle("hidden", !on); });
    $all(".day-chk").forEach(function (d) { d.classList.toggle("hidden", !on); d.classList.toggle("flex", on); });
    if (!on) { selected.clear(); paint(); }
    updateCount();
  }

  function toggleId(id, on) {
    if (!id) { return; }
    if (on === undefined) { on = !selected.has(id); }
    if (on) { selected.add(id); } else { selected.delete(id); }
  }

  function paint() {
    $all("[data-session-id]").forEach(function (el) {
      el.classList.toggle("is-selected", selected.has(el.getAttribute("data-session-id")));
    });
    $all(".row-chk").forEach(function (c) {
      var id = idForRow(c);
      if (id) { c.checked = selected.has(id); }
    });
  }

  // A row checkbox lives inside an <li data-session-id>; resolve its session id.
  function idForRow(chk) {
    var li = chk.closest("[data-session-id]");
    return li ? li.getAttribute("data-session-id") : null;
  }

  function updateCount() {
    $all("[data-sel-count]").forEach(function (el) { el.textContent = String(selected.size); });
    var hidden = $("[data-bulk-ids]");
    if (hidden) { hidden.value = Array.from(selected).join(","); }
  }

  // Resolve the day bucket for a block element (set on its wrapper).
  function blockDay(el) {
    var wrap = el.closest("[data-block-wrap]");
    return wrap ? wrap.getAttribute("data-day") : null;
  }
  function blockUnassigned(el) {
    var wrap = el.closest("[data-block-wrap]");
    return wrap ? wrap.getAttribute("data-unassigned") === "1" : false;
  }

  document.addEventListener("click", function (e) {
    var t = e.target;

    if (t.closest("[data-select-toggle]")) { e.preventDefault(); setMode(!selectMode); return; }
    if (t.closest("[data-bulk-cancel]")) { e.preventDefault(); setMode(false); return; }

    // Bulk-delete confirmed: submit via htmx (ConfirmDialog sets type=button).
    var delConfirm = t.closest("[data-bulk-delete-confirm]");
    if (delConfirm) {
      e.preventDefault();
      var delUrl = delConfirm.getAttribute("data-delete-url");
      var delFormId = delConfirm.getAttribute("data-form-id");
      submitDelete(delUrl, delFormId);
      return;
    }

    if (t.closest("[data-select-unassigned]")) {
      e.preventDefault();
      if (!selectMode) { setMode(true); }
      $all("[data-block-wrap][data-unassigned='1'] [data-session-id]").forEach(function (el) {
        toggleId(el.getAttribute("data-session-id"), true);
      });
      // List/agenda unassigned rows carry a dashed shell; select by checkbox aria.
      $all("li[data-session-id]").forEach(function (li) {
        var dashed = li.className.indexOf("border-dashed") !== -1;
        if (dashed) { toggleId(li.getAttribute("data-session-id"), true); }
      });
      paint(); updateCount(); return;
    }

    var dayBtn = t.closest("[data-day-select]");
    if (dayBtn && selectMode) {
      e.preventDefault();
      var day = dayBtn.getAttribute("data-day-select");
      $all("[data-block-wrap][data-day='" + day + "'] [data-session-id]").forEach(function (el) {
        toggleId(el.getAttribute("data-session-id"), true);
      });
      paint(); updateCount(); return;
    }

    // Block click: select in select-mode, otherwise open the single-edit dialog.
    var blk = t.closest(".block[data-session-id]");
    if (blk) {
      if (selectMode) {
        e.preventDefault(); e.stopImmediatePropagation();
        toggleId(blk.getAttribute("data-session-id"));
        paint(); updateCount();
      } else {
        e.preventDefault();
        openEdit(blk);
      }
      return;
    }

    // Assign toggle → open/close the fuzzy picker.
    if (t.closest("[data-assign-toggle]")) {
      e.preventDefault();
      var panel = $("[data-assign-panel]");
      if (panel) { panel.classList.toggle("hidden"); }
      var fi = $("[data-fuzzy-filter]");
      if (fi && panel && !panel.classList.contains("hidden")) { fi.focus(); }
      return;
    }

    // Pick an existing project → write id + submit the reassign form.
    var pick = t.closest("[data-project-id]");
    if (pick) {
      e.preventDefault();
      var pid = pick.getAttribute("data-project-id");
      var pidField = $("[data-bulk-project-id]");
      var newField = $("[data-bulk-new-project]");
      if (pidField) { pidField.value = pid; }
      if (newField) { newField.value = ""; }
      submitForm("[data-assign-toggle]");
      return;
    }

    // Inline-create → write the filter text as newProject + submit.
    if (t.closest("[data-new-project]")) {
      e.preventDefault();
      var fi2 = $("[data-fuzzy-filter]");
      var name = fi2 ? fi2.value.trim() : "";
      if (!name) { return; }
      var pidField2 = $("[data-bulk-project-id]");
      var newField2 = $("[data-bulk-new-project]");
      if (pidField2) { pidField2.value = ""; }
      if (newField2) { newField2.value = name; }
      submitForm("[data-assign-toggle]");
      return;
    }

    // Close the picker on an outside click.
    var panelOpen = $("[data-assign-panel]");
    if (panelOpen && !panelOpen.classList.contains("hidden") &&
        !panelOpen.contains(t) && !t.closest("[data-assign-toggle]")) {
      panelOpen.classList.add("hidden");
    }
  }, true);

  // Submit the bulk reassign form via the assign route (htmx if available).
  function submitForm() {
    var form = $("#historieBulkForm");
    var btn = $("[data-assign-toggle]");
    if (!form || !btn) { return; }
    updateCount();
    var url = btn.getAttribute("data-assign-url") || form.getAttribute("action");
    if (window.htmx && url) {
      window.htmx.ajax("POST", url, { source: form, target: "#content", swap: "innerHTML" });
    } else if (url) {
      form.setAttribute("action", url);
      form.submit();
    }
    var panel = $("[data-assign-panel]");
    if (panel) { panel.classList.add("hidden"); }
    setMode(false);
  }

  // Submit the bulk-delete via htmx (mirrors submitForm but uses the delete URL).
  function submitDelete(url, formId) {
    var form = document.getElementById(formId);
    if (!form || !url) { return; }
    updateCount();
    if (window.htmx) {
      window.htmx.ajax("POST", url, { source: form, target: "#content", swap: "innerHTML" });
    } else {
      form.setAttribute("action", url);
      form.submit();
    }
    setMode(false);
  }

  // Checkbox change (agenda / list rows).
  document.addEventListener("change", function (e) {
    var c = e.target;
    if (c.classList && c.classList.contains("row-chk")) {
      toggleId(idForRow(c), c.checked);
      paint(); updateCount();
    }
  });

  // Fuzzy filter: simple contains-match over picker rows + live inline-create label.
  document.addEventListener("input", function (e) {
    var fi = e.target;
    if (!fi.matches || !fi.matches("[data-fuzzy-filter]")) { return; }
    var q = fi.value.trim().toLowerCase();
    $all("[data-project-id]").forEach(function (row) {
      var name = (row.getAttribute("data-project-name") || "").toLowerCase();
      row.classList.toggle("hidden", q !== "" && name.indexOf(q) === -1);
    });
    var label = $("[data-new-project-label]");
    if (label) { label.textContent = q ? '„' + fi.value.trim() + '"' : ""; }
  });

  // Single-edit dialog: prefill the form from the clicked block + open the <dialog>.
  function openEdit(blk) {
    var wrap = blk.closest("[data-block-wrap]");
    if (!wrap) { return; }
    var dlg = document.getElementById("editSession");
    if (!dlg) { return; }
    var set = function (sel, val) { var el = $(sel, dlg); if (el) { el.value = val || ""; } };
    set("[data-edit-field-id]", wrap.getAttribute("data-edit-id"));
    set("[data-edit-field-date]", wrap.getAttribute("data-date"));
    set("[data-edit-field-from]", wrap.getAttribute("data-edit-from"));
    set("[data-edit-field-to]", wrap.getAttribute("data-edit-to"));
    set("[data-edit-field-tag]", wrap.getAttribute("data-edit-tag"));
    // Textarea needs value set differently.
    var noteEl = $("[data-edit-field-note]", dlg);
    if (noteEl) { noteEl.value = wrap.getAttribute("data-edit-note") || ""; }
    // Select the matching project option.
    var projSel = $("[data-edit-field-project]", dlg);
    if (projSel) { projSel.value = wrap.getAttribute("data-edit-project") || ""; }
    if (typeof dlg.showModal === "function") { dlg.showModal(); } else { dlg.setAttribute("open", ""); }
  }

  document.addEventListener("keydown", function (e) {
    if (e.key === "Escape" && selectMode) { setMode(false); }
  });

  // Re-bind state after htmx swaps re-render the fragment.
  document.body.addEventListener("htmx:afterSwap", function () {
    if (selectMode) { setMode(false); }
    paint(); updateCount();
  });
})();
