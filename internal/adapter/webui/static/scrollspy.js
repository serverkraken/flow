// Highlights the active category-strip link as sections scroll into view.
(function () {
  function init() {
    var links = document.querySelectorAll('.catstrip-link');
    if (!links.length) return;

    var byId = {};
    links.forEach(function (link) {
      byId[link.dataset.target] = link;
      link.classList.remove('text-ink');
    });

    if (!('IntersectionObserver' in window)) {
      links[0].classList.add('text-ink');
      return;
    }

    var obs = new IntersectionObserver(function (entries) {
      entries.forEach(function (entry) {
        var link = byId[entry.target.id];
        if (link && entry.isIntersecting) {
          links.forEach(function (item) { item.classList.remove('text-ink'); });
          link.classList.add('text-ink');
        }
      });
    }, { rootMargin: '-40% 0px -55% 0px' });

    ['daily-sec', 'notes-sec', 'free-sec', 'system-sec'].forEach(function (id) {
      var el = document.getElementById(id);
      if (el) obs.observe(el);
    });
  }

  document.addEventListener('DOMContentLoaded', init);
  document.body.addEventListener('htmx:afterSwap', init);
})();
