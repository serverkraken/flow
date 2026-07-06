// Rendert alle <pre class="mermaid"> als gesetzte Figuren. Lädt mermaid.min.js
// selbst nach, aber nur wenn ein Diagramm im DOM ist. Idempotent über [data-mm-done].
(function () {
  var MAX = 20000;          // Input-Cap gegen Browser-DoS
  var loading = false, failed = false;
  function markError() { document.querySelectorAll('.mermaid-figure:not(.mermaid-error)').forEach(function (f) { f.classList.add('mermaid-error'); }); }
  function ensureLib(cb) {
    if (window.mermaid) return cb();
    if (failed) return markError();          // Lib zuvor nicht geladen → Figuren markieren, Quelle bleibt lesbar
    if (loading) return;
    loading = true;
    var s = document.createElement('script');
    s.src = '/static/vendor/mermaid.min.js';  // 'self' → CSP script-src ok
    s.onload = function () { loading = false; cb(); };
    s.onerror = function () { loading = false; failed = true; markError(); };
    document.head.appendChild(s);
  }
  function render() {
    var pending = document.querySelectorAll('pre.mermaid:not([data-mm-done])');
    if (!pending.length) return;
    ensureLib(function () {
      window.mermaid.initialize({ startOnLoad: false, securityLevel: 'strict', htmlLabels: false, flowchart: { htmlLabels: false } });
      pending.forEach(function (el) {
        el.setAttribute('data-mm-done', '1');
        var figure = el.closest('.mermaid-figure');
        var src = el.textContent || '';
        if (src.length > MAX) { figure.classList.add('mermaid-error'); return; }
        // mermaid.parse(..., {suppressErrors:true}) resolves to `false` on a
        // syntax error instead of throwing/rendering mermaid's own bomb-SVG —
        // validate BEFORE run() ever touches the element, so an invalid
        // diagram never gets its <pre> replaced: the source stays readable
        // and only the warn-styled .mermaid-error state is added.
        window.mermaid.parse(src, { suppressErrors: true }).then(function (ok) {
          if (!ok) { figure.classList.add('mermaid-error'); return; }
          return window.mermaid.run({ nodes: [el], suppressErrors: true });
        }).catch(function () { figure.classList.add('mermaid-error'); });
      });
    });
  }
  if (document.readyState !== 'loading') render(); else document.addEventListener('DOMContentLoaded', render);
  document.body.addEventListener('htmx:afterSwap', render);
})();
