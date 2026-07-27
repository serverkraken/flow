/* clipboard.js — dependency-free copy-to-clipboard for the Frei ICS feed URL
 * and heading deep-links. A [data-copy] element writes its value to the
 * clipboard via the async Clipboard API and briefly swaps its label to
 * data-copied-label. No native browser popups (alert/confirm/prompt are
 * banned by verify-no-popups). Idempotent: a single delegated click
 * listener survives htmx swaps.
 *
 * A data-copy value starting with "#" (the heading head-anchor's
 * "#slug" href) is resolved against location.href at click time, so the
 * clipboard gets a full shareable deep link instead of the bare fragment —
 * the server renders the same "#slug" href/data-copy pair regardless of
 * which page it ends up on. */
(function () {
  "use strict";
  if (window.__flowClipboardBound) { return; }
  window.__flowClipboardBound = true;

  document.addEventListener("click", function (e) {
    var btn = e.target.closest("[data-copy]");
    if (!btn) { return; }
    e.preventDefault();
    var text = btn.getAttribute("data-copy") || "";
    if (text.charAt(0) === "#") {
      text = location.href.split("#")[0] + text;
    }
    var done = btn.getAttribute("data-copied-label") || "✓";
    var orig = btn.textContent;
    function flash() {
      btn.textContent = done;
      setTimeout(function () { btn.textContent = orig; }, 1500);
    }
    if (navigator.clipboard && navigator.clipboard.writeText) {
      navigator.clipboard.writeText(text).then(flash, flash);
    } else {
      flash();
    }
  });
})();
