(function () {
  function build() {
    var toc = document.getElementById('toc');
    var prose = document.querySelector('.prose');
    if (!toc || !prose) return;

    toc.innerHTML = '';
    var headings = prose.querySelectorAll('h1, h2, h3');
    headings.forEach(function (heading, index) {
      if (!heading.id) heading.id = 'h-' + index;
      var link = document.createElement('a');
      link.href = '#' + heading.id;
      link.textContent = heading.textContent;
      link.className = 'block py-1 text-muted hover:text-ink toc-' + heading.tagName.toLowerCase();
      toc.appendChild(link);
    });
  }

  document.addEventListener('DOMContentLoaded', build);
  document.body.addEventListener('htmx:afterSwap', build);
})();
