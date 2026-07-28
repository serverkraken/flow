// Klick auf ein eingebettetes Dokument-Bild öffnet es groß in #doc-lightbox.
// Der Server markiert zoombare Bilder mit class="zoomable" (nur solche mit
// nutzbarer Quelle — siehe safeImageHTMLRenderer); dieses Skript macht sie
// bedienbar. Esc, Backdrop-Klick und Fokus-Falle liefert dialog.js generisch
// für jedes dialog[open]; nur der Fokus-Rücksprung hängt dort an
// [data-dialog-open] und passiert deshalb hier selbst.
// Idempotent über [data-lb-done], re-scannt bei htmx:afterSwap.
(function () {
  if (window.__flowLightboxInit) return;
  window.__flowLightboxInit = true;

  var opener = null;

  function dlg() { return document.getElementById('doc-lightbox'); }

  // upgrade macht jedes noch unbehandelte zoombare Bild bedienbar: ein <img>
  // ist von sich aus weder fokussierbar noch als Bedienelement erkennbar.
  function upgrade() {
    var d = dlg();
    if (!d) return;                                  // Seite ohne Overlay → nichts zu tun
    var label = d.dataset.zoomLabel || '';
    document.querySelectorAll('img.zoomable:not([data-lb-done])').forEach(function (img) {
      img.setAttribute('data-lb-done', '1');
      img.setAttribute('tabindex', '0');
      img.setAttribute('role', 'button');
      var alt = img.getAttribute('alt') || '';
      if (label) img.setAttribute('aria-label', alt ? alt + ' — ' + label : label);
    });
  }

  function open(img) {
    var d = dlg();
    if (!d || typeof d.showModal !== 'function') return;
    var target = d.querySelector('.lightbox-img');
    if (!target) return;
    // Nur kopieren, was schon im DOM steht — das Skript trifft KEINE eigene
    // URL-Entscheidung; welcher src überhaupt entstehen darf, hat der
    // Renderer server-seitig entschieden.
    target.setAttribute('src', img.currentSrc || img.src);
    target.setAttribute('alt', img.getAttribute('alt') || '');
    opener = img;
    d.showModal();
  }

  document.addEventListener('click', function (e) {
    var img = e.target.closest ? e.target.closest('img.zoomable') : null;
    if (img) open(img);
  });

  document.addEventListener('keydown', function (e) {
    if (e.key !== 'Enter' && e.key !== ' ' && e.key !== 'Spacebar') return;
    var el = document.activeElement;
    if (!el || !el.matches || !el.matches('img.zoomable')) return;
    e.preventDefault();                              // Space soll nicht scrollen
    open(el);
  });

  // Fokus zurück aufs Bild, sobald das Overlay schließt — egal ob per ✕, Esc
  // oder Backdrop. 'close' bubbelt nicht, daher Capture-Phase (wie dialog.js).
  document.addEventListener('close', function (e) {
    if (e.target.id !== 'doc-lightbox') return;
    var t = e.target.querySelector('.lightbox-img');
    if (t) t.removeAttribute('src');                 // Bild freigeben, kein Aufblitzen beim nächsten Öffnen
    if (opener) { if (opener.isConnected) opener.focus(); opener = null; }
  }, true);

  if (document.readyState !== 'loading') upgrade();
  else document.addEventListener('DOMContentLoaded', upgrade);
  document.body.addEventListener('htmx:afterSwap', upgrade);
})();
