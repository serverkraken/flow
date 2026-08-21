// connection.js — Verbindungsverlust als Band, nicht als Seite (Screen 20).
//
// Quelle der Wahrheit ist der SSE-Strom (htmx-ext-sse am Body): reißt er
// ab, versucht der Browser selbst neu zu verbinden; wir zeigen nur, was
// gerade gilt. Dazu die Browser-Ereignisse online/offline. Kein Overlay,
// keine leere Seite — die Stempeluhr zählt serverseitig weiter.
(function () {
	'use strict';
	var banner = document.getElementById('conn-banner');
	if (!banner) { return; }
	var text = banner.querySelector('[data-conn-text]');
	var hint = banner.querySelector('[data-conn-hint]');
	var lost = false;
	var hideTimer = 0;

	function show(state) {
		clearTimeout(hideTimer);
		banner.hidden = false;
		banner.setAttribute('data-state', state);
		text.textContent = banner.getAttribute('data-msg-' + state) || '';
		hint.textContent = '';
	}
	function onLost() {
		if (lost) { return; }
		lost = true;
		show('lost');
	}
	function onRetry() {
		if (!lost) { return; }
		show('retry');
	}
	function onBack() {
		if (!lost) { return; }
		lost = false;
		show('back');
		hideTimer = setTimeout(function () { banner.hidden = true; }, 3000);
	}

	window.addEventListener('offline', onLost);
	window.addEventListener('online', onRetry);
	// htmx-ext-sse meldet sich auf dem Element mit sse-connect (dem Body).
	document.body.addEventListener('htmx:sseError', onLost);
	document.body.addEventListener('htmx:sseClose', onLost);
	document.body.addEventListener('htmx:sseOpen', onBack);
})();
