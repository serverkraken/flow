(function () {
	var kind = document.getElementById('node-kind');
	var parent = document.getElementById('node-parent');
	var rate = document.getElementById('node-rate');
	if (!kind || !parent) return;
	function sync() {
		var isEng = kind.value === 'engagement';
		parent.disabled = isEng;
		if (isEng) parent.value = '';
		if (rate) rate.style.display = isEng ? '' : 'none';
		Array.prototype.forEach.call(parent.options, function (o) {
			if (!o.value) return;
			var pk = o.getAttribute('data-parent-kind');
			o.hidden = !(pk === 'engagement' || pk === 'vorhaben');
		});
	}
	kind.addEventListener('change', sync);
	sync();
})();
