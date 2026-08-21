// neuekarte.js — Neue Karte per Taste N (Screen 12, im Mockup ⌘N).
//
// Der Dialog steht leer in der Hülle; beim Öffnen holt er sein Formular
// per htmx mit dem Register, auf dem man gerade steht (data-context-node
// oder data-node-id auf der Seite). Die Pfadvorschau rechnet wie der Server:
// Muster je Typ, Datum, Slug aus dem Titel (klein, Umlaute ersetzt, alles
// andere ein Bindestrich). Enter legt an.
(function () {
	'use strict';
	if (window.__flowNeueKarteBound) { return; }
	window.__flowNeueKarteBound = true;

	function dialog() { return document.getElementById('neue-karte'); }
	function contextNode() {
		var el = document.querySelector('[data-context-node]') || document.querySelector('[data-node-id]');
		return el ? (el.getAttribute('data-context-node') || el.getAttribute('data-node-id') || '') : '';
	}
	function open() {
		var dlg = dialog();
		if (!dlg || typeof dlg.showModal !== 'function') { window.location.href = '/wissen/neu'; return; }
		var body = document.getElementById('neue-karte-body');
		var url = body.getAttribute('data-src');
		var node = contextNode();
		if (node) { url += '?node=' + encodeURIComponent(node); }
		if (window.htmx) {
			window.htmx.ajax('GET', url, { target: '#neue-karte-body', swap: 'innerHTML' }).then(function () {
				var t = body.querySelector('[data-nk-title]');
				if (t) { t.focus(); }
				preview();
			});
		}
		if (!dlg.open) { dlg.showModal(); }
	}

	function slugify(s) {
		s = (s || '').toLowerCase().replace(/ä/g, 'ae').replace(/ö/g, 'oe').replace(/ü/g, 'ue').replace(/ß/g, 'ss');
		s = s.replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '');
		return s || 'karte';
	}
	function preview() {
		var form = document.querySelector('[data-neue-karte-form]');
		if (!form) { return; }
		var type = form.querySelector('[data-nk-type]:checked');
		var title = form.querySelector('[data-nk-title]');
		var out = form.querySelector('[data-nk-preview]');
		var titleField = form.querySelector('[data-nk-title-field]');
		var pathInput = form.querySelector('[data-nk-path]');
		if (!type || !out) { return; }
		var pattern = type.getAttribute('data-pattern') || '{slug}';
		var needsTitle = pattern.indexOf('{slug}') >= 0;
		if (titleField) { titleField.classList.toggle('hidden', !needsTitle); }
		var path = pattern.replace('{date}', form.getAttribute('data-date') || '').replace('{slug}', slugify(title ? title.value : ''));
		out.textContent = path;
		if (pathInput && !pathInput.hasAttribute('data-touched')) { pathInput.value = path; }
	}

	// ⌘N gehört auf dem Mac dem Browser (neues Fenster) und erreicht die
	// Seite nie. Deshalb: die Taste N allein, solange kein Feld den Fokus
	// hat — und Strg/⌘+N dort, wo der Browser es durchlässt.
	function inEditable(el) {
		if (!el) { return false; }
		var tag = (el.tagName || '').toLowerCase();
		return tag === 'input' || tag === 'textarea' || tag === 'select' || el.isContentEditable;
	}
	document.addEventListener('keydown', function (e) {
		if (e.key.toLowerCase() !== 'n' || e.shiftKey || e.altKey) { return; }
		if (e.metaKey || e.ctrlKey) { e.preventDefault(); open(); return; }
		if (inEditable(document.activeElement) || document.querySelector('dialog[open]')) { return; }
		e.preventDefault();
		open();
	});
	document.addEventListener('click', function (e) {
		var opener = e.target.closest('[data-neue-karte]');
		if (opener) { e.preventDefault(); open(); return; }
		var adjust = e.target.closest('[data-nk-adjust]');
		if (adjust) {
			e.preventDefault();
			var f = document.querySelector('[data-nk-path-field]');
			if (f) { f.classList.remove('hidden'); var i = f.querySelector('[data-nk-path]'); if (i) { i.focus(); } }
		}
	});
	document.addEventListener('input', function (e) {
		if (e.target.matches && e.target.matches('[data-nk-path]')) { e.target.setAttribute('data-touched', ''); return; }
		if (e.target.closest && e.target.closest('[data-neue-karte-form]')) { preview(); }
	});
	document.addEventListener('change', function (e) {
		if (e.target.closest && e.target.closest('[data-neue-karte-form]')) { preview(); }
	});
})();
