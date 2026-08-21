// editor-preview.js — die Vorschau des Wissens-Editors einklappen.
//
// Soennes Layout-Entscheidung: Stift und Vorschau nebeneinander, die
// Vorschau einklappbar. Der Zustand ist eine Wahl, die die Fläche sich
// merken muss — sie liegt im localStorage des Browsers, nicht am Dokument,
// denn sie sagt etwas über den Schreibenden, nicht über die Karte.
//
// Ereignis-Delegation auf dem Dokument (Muster railnav.js): htmx läuft hier
// mit allowScriptTags = false, und der Schalter soll jeden Tausch überleben.
(function () {
	'use strict';

	var STORE = 'flow-editor-preview';
	var HIDDEN = 'hidden';

	function load() {
		try { return localStorage.getItem(STORE) === HIDDEN; } catch (e) { return false; }
	}
	function save(hidden) {
		try {
			if (hidden) { localStorage.setItem(STORE, HIDDEN); } else { localStorage.removeItem(STORE); }
		} catch (e) { /* privater Modus */ }
	}

	function grid() { return document.querySelector('[data-editor-grid]'); }

	function apply(hidden) {
		var g = grid();
		if (!g) { return; }
		if (hidden) { g.setAttribute('data-preview', HIDDEN); } else { g.removeAttribute('data-preview'); }
		document.querySelectorAll('[data-preview-toggle]').forEach(function (btn) {
			btn.setAttribute('aria-pressed', hidden ? 'false' : 'true');
			var label = hidden ? btn.getAttribute('data-label-show') : btn.getAttribute('data-label-hide');
			if (label) { btn.textContent = label; }
		});
	}

	document.addEventListener('click', function (e) {
		var btn = e.target.closest('[data-preview-toggle]');
		if (!btn) { return; }
		e.preventDefault();
		var g = grid();
		var hidden = !(g && g.getAttribute('data-preview') === HIDDEN);
		save(hidden);
		apply(hidden);
	});

	function boot() { apply(load()); }
	if (document.readyState === 'loading') { document.addEventListener('DOMContentLoaded', boot); } else { boot(); }
})();
