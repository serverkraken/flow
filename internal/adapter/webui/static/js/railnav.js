// railnav.js — Auf- und Zuklappen des Registerbaums in der Schiene.
//
// Warum eine Datei und kein Inline-Skript im Fragment: dieses Repo fährt
// htmx mit allowScriptTags = false, Skripte in nachgeladenen Fragmenten
// werden also NICHT ausgeführt. Ein Inline-Skript stirbt hier lautlos — die
// Schiene steht dann vollständig da und klappt trotzdem nie auf.
//
// Deshalb Ereignis-Delegation auf dem Dokument: das Skript wird einmal mit
// der Hülle geladen und ist damit unabhängig davon, wann das Fragment
// eintrifft (load, htmx-Tausch, SSE-Neuladung).
(function () {
	'use strict';

	var STORE = 'flow-nav-open';

	function load() {
		try {
			var raw = JSON.parse(localStorage.getItem(STORE));
			return (raw && typeof raw === 'object') ? raw : {};
		} catch (e) { return {}; }
	}
	function save(v) {
		try { localStorage.setItem(STORE, JSON.stringify(v)); } catch (e) { /* privater Modus */ }
	}

	// choice hält true UND false: "zu" muss als Entscheidung speicherbar sein,
	// sonst reißt die Ahnenkette der aktuellen Seite den Zweig immer wieder auf.
	var choice = load();

	function list() { return document.getElementById('navtree-list'); }

	function parents(l) {
		var map = {};
		l.querySelectorAll('[data-id]').forEach(function (el) {
			map[el.getAttribute('data-id')] = el.getAttribute('data-parent') || '';
		});
		return map;
	}

	function apply() {
		var l = list();
		if (!l) { return; }
		var parentOf = parents(l);

		// Die Zeile der aktuellen Seite markieren und ihre Ahnen aufklappen.
		var activeID = '';
		l.querySelectorAll('a[href]').forEach(function (a) {
			if (a.getAttribute('href') !== window.location.pathname) { return; }
			a.classList.add('nv-row-active');
			var row = a.closest('[data-id]');
			if (row) { activeID = row.getAttribute('data-id'); }
		});
		var auto = {}, walked = {};
		for (var p = parentOf[activeID]; p && !walked[p]; p = parentOf[p]) {
			walked[p] = true;
			auto[p] = true;
		}

		function isOpen(id) {
			return Object.prototype.hasOwnProperty.call(choice, id) ? !!choice[id] : !!auto[id];
		}
		function isVisible(id) {
			var seen = {};
			for (var q = parentOf[id]; q; q = parentOf[q]) {
				// Elternteil nicht im Baum: ein von BuildTree auf Ebene 0
				// hochgeholtes Waisenkind trägt seine tote ParentID weiter.
				// Niemals hinter einer Zeile verstecken, die niemand öffnen kann.
				if (seen[q] || !(q in parentOf)) { return true; }
				seen[q] = true;
				if (!isOpen(q)) { return false; }
			}
			return true;
		}

		l.querySelectorAll('[data-id]').forEach(function (el) {
			el.classList.toggle('hidden', !isVisible(el.getAttribute('data-id')));
		});
		l.querySelectorAll('[data-nv-caret]').forEach(function (btn) {
			var open = isOpen(btn.getAttribute('data-nv-caret'));
			btn.textContent = open ? '▾' : '▸';
			btn.setAttribute('aria-expanded', open ? 'true' : 'false');
		});
	}

	document.addEventListener('click', function (e) {
		var btn = e.target.closest && e.target.closest('[data-nv-caret]');
		if (!btn) { return; }
		e.preventDefault();
		e.stopPropagation();
		var id = btn.getAttribute('data-nv-caret');
		var l = list();
		if (!l) { return; }
		// Den aktuellen Zustand aus dem Pfeil selbst lesen — apply() hat ihn
		// dort hinterlassen, und so braucht der Handler keine eigene Kopie.
		choice[id] = btn.getAttribute('aria-expanded') !== 'true';
		save(choice);
		apply();
	});

	document.addEventListener('DOMContentLoaded', apply);
	document.body && document.body.addEventListener('htmx:afterSwap', apply);
	document.addEventListener('htmx:afterSwap', apply);
	apply();
})();
