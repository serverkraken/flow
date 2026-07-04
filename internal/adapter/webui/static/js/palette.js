// ⌘K-Palette: öffnen/fokussieren, Pfeil-Navigation über [data-palette-row],
// Enter folgt der markierten Zeile, Esc schließt. Kein Framework.
(function () {
  var dlg, input, sel = -1;
  function rows() { return dlg ? Array.prototype.slice.call(dlg.querySelectorAll('[data-palette-row]')) : []; }
  function mark(n) {
    var r = rows();
    sel = Math.max(0, Math.min(n, r.length - 1));
    r.forEach(function (el, i) { el.setAttribute('aria-selected', i === sel ? 'true' : 'false'); });
    if (r[sel]) r[sel].scrollIntoView({ block: 'nearest' });
  }
  function open() {
    dlg = document.getElementById('palette');
    input = document.getElementById('palette-input');
    if (!dlg) return;
    dlg.showModal();
    input.value = '';
    input.focus();
    sel = -1;
  }
  document.addEventListener('keydown', function (e) {
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
      e.preventDefault();
      dlg && dlg.open ? dlg.close() : open();
      return;
    }
    if (!dlg || !dlg.open) return;
    if (e.key === 'ArrowDown') { e.preventDefault(); mark(sel + 1); }
    if (e.key === 'ArrowUp') { e.preventDefault(); mark(sel - 1); }
    if (e.key === 'Enter') {
      var r = rows();
      if (r[sel >= 0 ? sel : 0]) { e.preventDefault(); window.location.href = r[sel >= 0 ? sel : 0].href; }
    }
  });
  document.addEventListener('click', function (e) {
    var b = e.target.closest('[data-palette-open]');
    if (b) { e.preventDefault(); open(); }
    if (dlg && dlg.open && e.target === dlg) dlg.close(); // Klick auf Backdrop
  });
  document.body.addEventListener('htmx:afterSwap', function (e) {
    if (e.target && e.target.id === 'palette-results') mark(-1);
  });
})();
