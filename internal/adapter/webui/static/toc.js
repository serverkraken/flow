(function () {
  function build() {
    var tocs = document.querySelectorAll('[data-toc-nav]');
    var prose = document.querySelector('.prose');
    if (!tocs.length || !prose) return;

    var headings = prose.querySelectorAll('h1, h2, h3');
    tocs.forEach(function (toc) {
      toc.innerHTML = '';
      headings.forEach(function (heading, index) {
        if (!heading.id) heading.id = 'h-' + index;
        var link = document.createElement('a');
        link.href = '#' + heading.id;
        link.textContent = heading.textContent;
        link.className = 'block py-1 text-muted hover:text-ink toc-' + heading.tagName.toLowerCase();
        toc.appendChild(link);
      });
      // Empty doc (no headings): hide the surrounding .blk rail block instead
      // of leaving a bare eyebrow + empty <nav> frame standing (Lesesaal
      // docrail "Auf dieser Seite" — Codex #10).
      var block = toc.closest('[data-toc-block]');
      if (block) block.hidden = headings.length === 0;
    });
  }

  document.addEventListener('DOMContentLoaded', build);
  document.body.addEventListener('htmx:afterSwap', build);
})();
