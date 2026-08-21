// sessiondialog.js — Dauer und Handgriffe im Buchungs-Dialog (Screen 26).
//
// Die Dauer rechnet sich aus Von und Bis; −15/+15 verschieben das Ende,
// „auf viertel runden" zieht beide Zeiten auf die Viertelstunde. Sonst
// ändert sich nichts — Nachbarbuchungen bleiben, wo sie sind. Delegiert
// auf das Dokument, damit jeder Dialog (Heute, Cockpit, Historie) es hat.
(function () {
	'use strict';
	if (window.__flowSessionDialogBound) { return; }
	window.__flowSessionDialogBound = true;

	function mins(v) {
		var m = /^(\d{1,2}):(\d{2})$/.exec(v || '');
		return m ? (+m[1]) * 60 + (+m[2]) : null;
	}
	function hhmm(n) {
		n = ((n % 1440) + 1440) % 1440;
		var h = Math.floor(n / 60), m = n % 60;
		return (h < 10 ? '0' : '') + h + ':' + (m < 10 ? '0' : '') + m;
	}
	function update(form) {
		var from = mins(form.querySelector('[name=from]').value);
		var to = mins(form.querySelector('[name=to]').value);
		var out = form.querySelector('[data-session-duration]');
		if (!out) { return; }
		if (from === null || to === null) { out.textContent = '—'; return; }
		var d = to - from;
		if (d < 0) { d += 1440; }
		out.textContent = Math.floor(d / 60) + ':' + ((d % 60) < 10 ? '0' : '') + (d % 60) + ' h';
	}
	document.addEventListener('input', function (e) {
		var form = e.target.closest && e.target.closest('[data-session-form]');
		if (form) { update(form); }
	});
	document.addEventListener('click', function (e) {
		var shift = e.target.closest('[data-session-shift]');
		var round = e.target.closest('[data-session-round]');
		if (!shift && !round) { return; }
		var form = e.target.closest('[data-session-form]');
		if (!form) { return; }
		e.preventDefault();
		var fromEl = form.querySelector('[name=from]'), toEl = form.querySelector('[name=to]');
		if (shift) {
			var to = mins(toEl.value);
			if (to !== null) { toEl.value = hhmm(to + (+shift.getAttribute('data-session-shift'))); }
		} else {
			[fromEl, toEl].forEach(function (el) {
				var v = mins(el.value);
				if (v !== null) { el.value = hhmm(Math.round(v / 15) * 15); }
			});
		}
		update(form);
	});
	// Beim Öffnen steht die Dauer schon da.
	document.addEventListener('htmx:afterSwap', function () {
		document.querySelectorAll('[data-session-form]').forEach(update);
	});
	document.querySelectorAll('[data-session-form]').forEach(update);
})();
