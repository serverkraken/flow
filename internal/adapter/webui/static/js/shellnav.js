// shellnav.js — das Verhalten der Hülle im Tablet-Band (Katalog 3.13,
// Screen 21A).
//
// Zwei Overlays, dasselbe Muster: der ☰ der 76px-Schiene schiebt die volle
// Schiene ein, der Panel-Schalter einer Dreispalter-Fläche schiebt die
// 372px-Kasten-Spalte ein. Außenklick und Esc schließen beide.
//
// Als Datei, nicht als Inline-Skript: dieses Repo fährt htmx mit
// allowScriptTags = false, und statische Dateien sind hier ohnehin die
// Konvention (dialog.js, railnav.js).
(function () {
	'use strict';

	function rail() { return document.getElementById('app-rail'); }
	function cols() { return document.querySelector('.k3-cols'); }

	function setRail(open) {
		var r = rail();
		if (!r) { return; }
		r.classList.toggle('rail-expanded', open);
		var btn = r.querySelector('[data-rail-expand]');
		if (btn) { btn.setAttribute('aria-expanded', open ? 'true' : 'false'); }
	}

	function setPanel(open) {
		var c = cols();
		if (!c) { return; }
		c.classList.toggle('panel-open', open);
		document.querySelectorAll('[data-panel-toggle]').forEach(function (b) {
			b.setAttribute('aria-expanded', open ? 'true' : 'false');
		});
	}

	document.addEventListener('click', function (e) {
		var t = e.target;
		if (!t.closest) { return; }
		if (t.closest('[data-rail-expand]')) {
			var r = rail();
			setRail(!(r && r.classList.contains('rail-expanded')));
			return;
		}
		if (t.closest('[data-panel-toggle]')) {
			var c = cols();
			setPanel(!(c && c.classList.contains('panel-open')));
			return;
		}
		// Außenklick schließt, was offen ist.
		var r2 = rail();
		if (r2 && r2.classList.contains('rail-expanded') && !r2.contains(t)) { setRail(false); }
		var c2 = cols();
		if (c2 && c2.classList.contains('panel-open') && !t.closest('.k3-panel')) { setPanel(false); }
	});

	document.addEventListener('keydown', function (e) {
		if (e.key === 'Escape') { setRail(false); setPanel(false); }
	});
})();
