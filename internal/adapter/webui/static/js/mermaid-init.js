// Rendert alle <pre class="mermaid"> als gesetzte Figuren. Lädt mermaid.min.js
// selbst nach, aber nur wenn ein Diagramm im DOM ist. Idempotent über [data-mm-done].
(function () {
  var MAX = 20000;          // Input-Cap gegen Browser-DoS
  var loading = false, failed = false;
  // document.currentScript is only valid synchronously while THIS script
  // (deferred or not) is executing — captured now so the later async
  // ensureLib() callback can still build a cache-busted vendor URL (Lesesaal
  // L4 Task 7: the mermaid.min.js self-inject has no <script src> of its own
  // for AssetURL to touch, so the version travels as data-v on this init
  // tag's own <script> instead).
  var v = (document.currentScript && document.currentScript.dataset.v) || '';
  function markError() { document.querySelectorAll('.mermaid-figure:not(.mermaid-error)').forEach(function (f) { f.classList.add('mermaid-error'); }); }
  var waiting = [];                         // Rückrufe, die auf das Laden warten
  function ensureLib(cb, onFail) {
    if (window.mermaid) return cb();
    if (failed) { markError(); if (onFail) onFail(); return; } // Lib zuvor nicht geladen → Figuren markieren, Quelle bleibt lesbar
    waiting.push([cb, onFail]);
    if (loading) return;
    loading = true;
    var s = document.createElement('script');
    s.src = '/static/vendor/mermaid.min.js' + (v ? '?v=' + v : '');  // 'self' → CSP script-src ok
    s.onload = function () { loading = false; var w = waiting; waiting = []; w.forEach(function (p) { p[0](); }); };
    s.onerror = function () { loading = false; failed = true; markError(); var w = waiting; waiting = []; w.forEach(function (p) { if (p[1]) p[1](); }); };
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

  // Brücke für den Stift (editor.mjs): ein Diagramm aus Quelle zu SVG —
  // dieselbe Lib, dieselbe Initialisierung, dieselbe Kappe wie die Figuren
  // der Leseansicht. Lehnt ab (reject) bei Syntaxfehler, Überlänge oder
  // fehlender Lib; der Stift zeichnet dann den Fehlerrahmen.
  var seq = 0;
  window.flowMermaid = {
    render: function (src) {
      return new Promise(function (resolve, reject) {
        if (!src || src.length > MAX) return reject(new Error('mermaid: source too long'));
        ensureLib(function () {
          window.mermaid.initialize({ startOnLoad: false, securityLevel: 'strict', htmlLabels: false, flowchart: { htmlLabels: false } });
          window.mermaid.parse(src, { suppressErrors: true }).then(function (ok) {
            if (!ok) throw new Error('mermaid: syntax');
            return window.mermaid.render('flow-mm-' + (++seq), src);
          }).then(function (r) { resolve(r.svg); }).catch(reject);
        }, function () { reject(new Error('mermaid: lib failed')); });
      });
    },
  };
})();
