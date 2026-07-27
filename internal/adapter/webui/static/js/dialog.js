// Dialog behavior for styled native <dialog>: open via [data-dialog-open="id"],
// close via [data-dialog-close], Esc, or backdrop click; focus-trap inside the
// open dialog and return focus to the opener on close. Idempotent: safe to load
// once even if multiple Dialog components include the <script>.
(function () {
  if (window.__flowDialogInit) return;
  window.__flowDialogInit = true;

  var lastOpener = null;

  function focusable(dlg) {
    return Array.prototype.slice.call(dlg.querySelectorAll(
      'a[href],button:not([disabled]),textarea,input,select,[tabindex]:not([tabindex="-1"])'
    )).filter(function (el) { return el.offsetParent !== null; });
  }

  document.addEventListener('click', function (e) {
    var opener = e.target.closest('[data-dialog-open]');
    if (opener) {
      var dlg = document.getElementById(opener.getAttribute('data-dialog-open'));
      if (dlg && typeof dlg.showModal === 'function') {
        lastOpener = opener;
        dlg.showModal();
        var f = focusable(dlg);
        var auto = dlg.querySelector('[autofocus]');
        (auto || f[0] || dlg).focus();
      }
      return;
    }
    var closer = e.target.closest('[data-dialog-close]');
    if (closer) {
      var d = closer.closest('dialog');
      if (d) d.close();
      return;
    }
  });

  // Backdrop click closes the dialog. With showModal() the <dialog> element IS
  // the styled panel; a click on its ::backdrop is dispatched with e.target ===
  // the dialog element. So: target is the DIALOG and the pointer is outside the
  // panel's content box → it was a backdrop click → close.
  // CONTROLLER CORRECTION: the original plan left this branch a `/* no-op */`,
  // so backdrop-click never actually closed (dead code) despite the Dialog
  // contract promising it. Implemented properly below.
  document.addEventListener('click', function (e) {
    if (e.target.tagName === 'DIALOG' && e.target.open) {
      var rect = e.target.getBoundingClientRect();
      var inside = e.clientX >= rect.left && e.clientX <= rect.right &&
                   e.clientY >= rect.top && e.clientY <= rect.bottom;
      if (!inside) { e.target.close(); }
    }
  });

  // Focus trap + return focus.
  document.addEventListener('keydown', function (e) {
    if (e.key !== 'Tab') return;
    var dlg = document.querySelector('dialog[open]');
    if (!dlg) return;
    var f = focusable(dlg);
    if (f.length === 0) return;
    var first = f[0], last = f[f.length - 1];
    if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last.focus(); }
    else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first.focus(); }
  });

  document.addEventListener('close', function (e) {
    if (e.target.tagName === 'DIALOG' && lastOpener) {
      lastOpener.focus();
      lastOpener = null;
    }
  }, true);

  // Forms opting into [data-dialog-close-on-success] (e.g. the Nachbuchen
  // add/edit dialog) close their enclosing <dialog> after a successful
  // htmx submit, instead of staying open post-save.
  document.body.addEventListener('htmx:afterRequest', function (e) {
    var form = e.target;
    if (!form.hasAttribute || !form.hasAttribute('data-dialog-close-on-success')) return;
    if (!e.detail.successful) return;
    var dlg = form.closest('dialog');
    if (dlg) dlg.close();
  });
})();
