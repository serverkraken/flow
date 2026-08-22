// editor-mode.js — Rich Text | Markdown und das Werkzeug-Band.
//
// Rich Text ist Standard: eine Fläche, der Stift (Milkdown). Markdown ist
// der Umschalter: Quelle links, Vorschau rechts. Beide schreiben dieselbe
// Markdown-Quelle (die Textarea). Die Wahl merkt sich der Browser.
// Ereignis-Delegation: überlebt jeden htmx-Tausch.
(function () {
	'use strict';
	var STORE = 'flow-editor-mode';

	function body() { return document.querySelector('[data-editor-body]'); }
	function source() { return document.getElementById('editor-body'); }
	function mode() { var b = body(); return b ? (b.getAttribute('data-mode') || 'rich') : 'rich'; }
	function load() { try { return localStorage.getItem(STORE) === 'markdown' ? 'markdown' : 'rich'; } catch (e) { return 'rich'; } }
	function save(m) { try { localStorage.setItem(STORE, m); } catch (e) { /* privater Modus */ } }

	function apply(m, fromUser) {
		var b = body(), ta = source();
		if (!b || !ta) { return; }
		var was = b.getAttribute('data-mode');
		b.setAttribute('data-mode', m);
		document.querySelectorAll('[data-editor-mode-set]').forEach(function (btn) {
			btn.setAttribute('aria-pressed', btn.getAttribute('data-editor-mode-set') === m ? 'true' : 'false');
		});
		if (m === 'markdown') {
			// Die Quelle ist aktuell (der Stift schreibt bei jeder Änderung hinein);
			// die Vorschau holt sich den Stand.
			ta.dispatchEvent(new Event('flow:preview', { bubbles: true }));
			if (fromUser) { ta.focus(); }
		} else if (was === 'markdown' && window.flowEditor) {
			// Zurück zum Stift: die Quelle kann sich geändert haben.
			window.flowEditor.setMarkdown(ta.value);
			if (fromUser) { window.flowEditor.focus(); }
		}
	}

	document.addEventListener('click', function (e) {
		var set = e.target.closest('[data-editor-mode-set]');
		if (set) { e.preventDefault(); var m = set.getAttribute('data-editor-mode-set'); save(m); apply(m, true); return; }
	});

	function boot() { apply(load(), false); }
	if (document.readyState === 'loading') { document.addEventListener('DOMContentLoaded', boot); } else { boot(); }
	// Der Stift meldet sich, wenn er steht — dann noch einmal den Modus setzen.
	document.addEventListener('flow:editor-ready', function () { apply(mode(), false); });
})();
