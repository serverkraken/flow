// nodeform.js — das Register-Formular (Screen 23/08): Art erklärt sich,
// Eltern und Satz folgen der Art, das Kürzel entsteht aus dem Namen, das
// Monogramm zeigt sich, wie es in der Schiene stünde, und die Fußzeile
// sagt, wo das Register landet.
(function () {
	var form = document.querySelector('[data-node-form]');
	var kind = document.getElementById('node-kind');
	var parent = document.getElementById('node-parent');
	if (!form || !kind || !parent) return;
	var rate = document.getElementById('node-rate');
	var desc = form.querySelector('[data-kind-desc]');
	var parentField = form.querySelector('[data-parent-field]');
	var upstream = form.querySelector('[data-upstream-field]');
	var name = form.querySelector('[data-node-name]');
	var slug = form.querySelector('[data-node-slug]');
	var slugPreview = form.querySelector('[data-slug-preview]');
	var slugField = form.querySelector('[data-slug-field]');
	var mono = form.querySelector('[data-mono-preview]');
	var pathPreview = form.querySelector('[data-path-preview]');
	var editing = form.hasAttribute('data-editing');

	function slugify(s) {
		s = (s || '').toLowerCase().replace(/ä/g, 'ae').replace(/ö/g, 'oe').replace(/ü/g, 'ue').replace(/ß/g, 'ss');
		return s.replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '');
	}
	function initials(s) {
		var parts = (s || '').trim().split(/[\s\-_.]+/).filter(Boolean);
		if (!parts.length) return '';
		if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase();
		return (parts[0][0] + parts[1][0]).toUpperCase();
	}
	function syncKind() {
		var k = kind.value;
		var isEng = k === 'engagement';
		parent.disabled = isEng;
		if (isEng) parent.value = '';
		if (parentField) parentField.classList.toggle('hidden', isEng);
		if (rate) rate.style.display = isEng ? '' : 'none';
		if (upstream) upstream.classList.toggle('hidden', k !== 'repo');
		if (desc) desc.textContent = kind.getAttribute('data-desc-' + k) || '';
		Array.prototype.forEach.call(parent.options, function (o) {
			if (!o.value) return;
			var pk = o.getAttribute('data-parent-kind');
			o.hidden = !(pk === 'engagement' || pk === 'vorhaben');
		});
		syncPath();
	}
	function syncName() {
		var n = name ? name.value : '';
		if (!editing && slug && !slug.hasAttribute('data-touched')) slug.value = slugify(n);
		if (slugPreview) slugPreview.textContent = slug ? slug.value : slugify(n);
		if (mono) mono.textContent = initials(n);
		syncPath();
	}
	function syncPath() {
		if (!pathPreview) return;
		var n = name ? name.value.trim() : '';
		var p = (!parent.disabled && parent.value) ? parent.options[parent.selectedIndex].textContent.replace(/\s*·.*$/, '').trim() : '';
		pathPreview.textContent = n ? (p ? p + ' › ' + n : n) : '';
	}
	kind.addEventListener('change', syncKind);
	parent.addEventListener('change', syncPath);
	if (name) name.addEventListener('input', syncName);
	if (slug) slug.addEventListener('input', function () { slug.setAttribute('data-touched', ''); if (slugPreview) slugPreview.textContent = slug.value; });
	form.addEventListener('click', function (e) {
		if (e.target.closest('[data-slug-adjust]') && slugField) { e.preventDefault(); slugField.classList.remove('hidden'); if (slug) slug.focus(); }
	});
	form.addEventListener('change', function (e) {
		if (e.target.name === 'color' && mono) {
			var sw = e.target.parentElement.querySelector('[data-swatch]');
			mono.style.backgroundColor = sw ? sw.getAttribute('data-swatch') : 'rgb(var(--meta))';
		}
	});
	syncKind();
	syncName();
})();
