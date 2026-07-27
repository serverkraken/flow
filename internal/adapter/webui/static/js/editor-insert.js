// Editor-Einfügehelfer (Lesesaal L6 Task 6): zwei Werkzeugleisten-Buttons
// öffnen je einen generischen Picker (Artefakt/Seite, htmx-Fragmente von
// /ui/editor/artefakte bzw. /ui/editor/seiten). Ein Klick auf eine Zeile
// fügt den vorgerechneten Markdown-Wert (![[slug]] bzw. [[pfad]]) am
// selectionStart der Editor-Textarea ein, setzt den Cursor dahinter und
// triggert die Live-Vorschau neu. Kein Popup — reines Zeigen/Verstecken
// eines absolut positionierten Panels neben dem auslösenden Button (Muster
// palette.js, aber ohne <dialog>/showModal).
(function () {
  function closeAll(except) {
    document.querySelectorAll('[data-insert-picker]').forEach(function (p) {
      if (p !== except) p.hidden = true;
    });
  }

  // insertAtCursor writes text at the textarea's current selectionStart
  // (an empty/never-focused textarea reports selectionStart 0, so the
  // "leere Textarea → am Anfang einfügen" error path falls out naturally),
  // moves the cursor to just after the inserted text, and — if htmx is
  // present — fires the textarea's own "keyup" trigger so its existing
  // hx-trigger="keyup changed delay:400ms" re-POSTs /wissen/preview.
  function insertAtCursor(textarea, text) {
    var start = typeof textarea.selectionStart === 'number' ? textarea.selectionStart : textarea.value.length;
    var end = typeof textarea.selectionEnd === 'number' ? textarea.selectionEnd : start;
    var value = textarea.value;
    textarea.value = value.slice(0, start) + text + value.slice(end);
    var pos = start + text.length;
    textarea.focus();
    textarea.setSelectionRange(pos, pos);
    if (window.htmx) { window.htmx.trigger(textarea, 'keyup'); }
  }

  document.addEventListener('click', function (e) {
    var toggle = e.target.closest('[data-insert-toggle]');
    if (toggle) {
      var panel = document.getElementById(toggle.getAttribute('data-insert-toggle'));
      if (!panel) { return; }
      var willOpen = panel.hidden;
      closeAll(willOpen ? panel : null);
      panel.hidden = !willOpen;
      if (willOpen) {
        var filter = panel.querySelector('[data-insert-filter]');
        if (filter) { filter.value = ''; filter.focus(); }
      }
      return;
    }

    var row = e.target.closest('[data-insert-value]');
    if (row) {
      e.preventDefault();
      var panelEl = row.closest('[data-insert-picker]');
      var anchor = panelEl ? panelEl.closest('[data-insert-anchor]') : null;
      var targetId = anchor ? anchor.getAttribute('data-insert-target') : null;
      var textarea = targetId ? document.getElementById(targetId) : null;
      if (textarea) { insertAtCursor(textarea, row.getAttribute('data-insert-value')); }
      if (panelEl) { panelEl.hidden = true; }
      return;
    }

    if (!e.target.closest('[data-insert-picker]') && !e.target.closest('[data-insert-toggle]')) {
      closeAll(null);
    }
  });

  document.addEventListener('keydown', function (e) {
    if (e.key === 'Escape') { closeAll(null); }
  });
})();
