// editor-mode.js — Rich Text | Markdown und das Werkzeug-Band.
//
// Rich Text ist Standard: eine Fläche, der Stift (Milkdown). Markdown ist
// der Umschalter: Quelle links, Vorschau rechts. Beide schreiben dieselbe
// Markdown-Quelle (die Textarea). Die Wahl merkt sich der Browser. Die
// Befehle des Bands gehen im Rich-Text-Modus an den Stift
// (window.flowEditor, aus dem Bundle) und im Markdown-Modus als Einfügung
// in die Quelle. Ereignis-Delegation: überlebt jeden htmx-Tausch.
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

	// Einfügung in die Quelle (Markdown-Modus): Präfix je Zeile oder Umschluss.
	function wrapSource(ta, before, after, placeholder) {
		var s = ta.selectionStart, e = ta.selectionEnd, v = ta.value;
		var sel = v.slice(s, e) || placeholder || '';
		ta.value = v.slice(0, s) + before + sel + after + v.slice(e);
		ta.selectionStart = s + before.length;
		ta.selectionEnd = s + before.length + sel.length;
		ta.focus();
		ta.dispatchEvent(new Event('keyup', { bubbles: true }));
	}
	function prefixLines(ta, prefix) {
		var s = ta.selectionStart, e = ta.selectionEnd, v = ta.value;
		var ls = v.lastIndexOf('\n', s - 1) + 1;
		var le = v.indexOf('\n', e); if (le < 0) { le = v.length; }
		var block = v.slice(ls, le).split('\n').map(function (l) { return prefix + l; }).join('\n');
		ta.value = v.slice(0, ls) + block + v.slice(le);
		ta.selectionStart = ls; ta.selectionEnd = ls + block.length;
		ta.focus();
		ta.dispatchEvent(new Event('keyup', { bubbles: true }));
	}
	function sourceCommand(ta, cmd) {
		switch (cmd) {
			case 'h2': return prefixLines(ta, '## ');
			case 'h3': return prefixLines(ta, '### ');
			case 'bold': return wrapSource(ta, '**', '**', 'fett');
			case 'italic': return wrapSource(ta, '*', '*', 'kursiv');
			case 'code': return wrapSource(ta, '`', '`', 'code');
			case 'list': return prefixLines(ta, '- ');
			case 'task': return prefixLines(ta, '- [ ] ');
			case 'table': return wrapSource(ta, '\n| Spalte | Spalte |\n| --- | --- |\n| ', ' |  |\n', '');
			case 'diagram': return wrapSource(ta, '\n```mermaid\n', '\n```\n', 'flowchart LR\n  A --> B');
		}
	}

	document.addEventListener('click', function (e) {
		var set = e.target.closest('[data-editor-mode-set]');
		if (set) { e.preventDefault(); var m = set.getAttribute('data-editor-mode-set'); save(m); apply(m, true); return; }
		var cmd = e.target.closest('[data-md-cmd]');
		if (cmd) {
			e.preventDefault();
			var name = cmd.getAttribute('data-md-cmd');
			if (mode() === 'rich' && window.flowEditor) { window.flowEditor.cmd(name); }
			else { var ta = source(); if (ta) { sourceCommand(ta, name); } }
		}
	});

	function boot() { apply(load(), false); }
	if (document.readyState === 'loading') { document.addEventListener('DOMContentLoaded', boot); } else { boot(); }
	// Der Stift meldet sich, wenn er steht — dann noch einmal den Modus setzen.
	document.addEventListener('flow:editor-ready', function () { apply(mode(), false); });
})();
