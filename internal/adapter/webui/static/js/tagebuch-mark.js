// tagebuch-mark.js — Screen 27: eine Stelle in der Tagesnotiz markieren und
// einem Register zuordnen.
//
// Warum eine Datei und kein Inline-Skript: dieses Repo fährt CSP ERZWINGEND
// (CSPEnforce), jedes Inline-Skript braucht also die Nonce der Anfrage — ein
// Skript ohne sie wird stumm geblockt. Genau daran ist die portierte Fassung
// gescheitert: markieren tat nichts, die Form kam nie. Dazu kommt htmx mit
// allowScriptTags = false, das Skripte in nachgeladenen Fragmenten ohnehin
// nie ausführt.
//
// Deshalb Ereignis-Delegation auf dem Dokument: einmal mit der Hülle geladen,
// unabhängig davon, wann die Fläche eintrifft (load, htmx-Tausch, SSE).
(function () {
	'use strict';

	function els() {
		return {
			body: document.getElementById('tagebuch-mark-body'),
			form: document.getElementById('tagebuch-mark-form'),
			quote: document.getElementById('tagebuch-mark-quote'),
			display: document.getElementById('tagebuch-mark-quote-display'),
		};
	}

	document.addEventListener('mouseup', function (ev) {
		var e = els();
		if (!e.body || !e.form) { return; }
		// Nur Auswahl INNERHALB der Notiz zählt — eine Markierung in der
		// Erklärspalte ist keine Stelle, die man zuordnen könnte.
		if (!e.body.contains(ev.target)) { return; }
		var sel = window.getSelection();
		var text = sel ? sel.toString().trim() : '';
		if (!text) { e.form.classList.add('hidden'); return; }
		if (e.quote) { e.quote.value = text; }
		if (e.display) { e.display.textContent = '„' + text + '“'; }
		e.form.classList.remove('hidden');
	});
})();
