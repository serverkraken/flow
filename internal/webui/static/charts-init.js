/* flow WebUI · ApexCharts initialisers.
 *
 * Each render target reads its data from a sibling <script
 * type="application/json"> block by id. The chart options encode the
 * Tufte-style ("ohne grid · ohne hintergrund") presentation rules from
 * internal/webui/DESIGN.md — animations off, no toolbar, hairline axes,
 * single accent / active color.
 *
 * Why a script and not inline-everything: the templ file generates the
 * HTML, the JSON block carries the data, and the JS encodes the chart
 * config so all three concerns are inspectable separately. */
(function () {
  'use strict';

  // Reads JSON payload by element id. Returns null on missing/invalid.
  function readJSON(id) {
    var el = document.getElementById(id);
    if (!el) return null;
    try {
      return JSON.parse(el.textContent || el.innerText || '');
    } catch (_) {
      return null;
    }
  }

  // Resolve the Tokyonight-Night CSS variables to absolute hex so the
  // chart library doesn't have to chase var(--…) values.
  function cssVar(name) {
    return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
  }

  // window.initWeekChart: 7-bar Mo-So bar chart. Today highlighted in
  // active color. Soll line as a dashed annotation at 8h.
  window.initWeekChart = function (targetId, dataId) {
    if (typeof ApexCharts !== 'function' && typeof ApexCharts !== 'object') return;
    var target = document.getElementById(targetId);
    if (!target) return;
    var bars = readJSON(dataId);
    if (!bars || !bars.length) return;

    var accent = cssVar('--color-accent') || '#7aa2f7';
    var active = cssVar('--color-active') || '#9ece6a';
    var muted = cssVar('--color-muted') || '#414868';
    var fgDim = cssVar('--color-fg-dim') || '#9aa5ce';
    var fg = cssVar('--color-fg') || '#c0caf5';

    var categories = bars.map(function (b) { return b.label; });
    var values = bars.map(function (b) { return Number(b.hours.toFixed(2)); });
    var colors = bars.map(function (b) { return b.isToday ? active : accent; });

    var options = {
      chart: {
        type: 'bar',
        height: 220,
        toolbar: { show: false },
        animations: { enabled: false },
        background: 'transparent',
        fontFamily: 'JetBrains Mono, ui-monospace, monospace',
      },
      series: [{ name: 'Stunden', data: values }],
      plotOptions: {
        bar: {
          columnWidth: '55%',
          distributed: true,
          dataLabels: { position: 'top' },
        },
      },
      colors: colors,
      legend: { show: false },
      dataLabels: {
        enabled: true,
        formatter: function (val, opts) {
          var idx = opts.dataPointIndex;
          var b = bars[idx];
          if (!b || b.hours === 0) return '';
          return b.hhmm + (b.isToday ? ' ▶' : '');
        },
        style: { fontSize: '10px', fontWeight: 600, colors: [fg] },
        offsetY: -16,
      },
      grid: { show: false },
      xaxis: {
        categories: categories,
        axisBorder: { show: true, color: muted },
        axisTicks: { show: false },
        labels: { style: { colors: fgDim, fontSize: '10px' } },
      },
      yaxis: {
        show: false,
        max: function (max) { return Math.max(max, 9); },
      },
      tooltip: { enabled: false },
      annotations: {
        yaxis: [{
          y: 8,
          borderColor: fgDim,
          strokeDashArray: 3,
          label: {
            text: '8h Soll',
            position: 'right',
            offsetX: -6,
            offsetY: -2,
            style: { color: fgDim, background: 'transparent', fontSize: '10px' },
          },
        }],
      },
    };

    var chart = new ApexCharts(target, options);
    chart.render();
  };

  // window.initSaldoSpark: 12-bar saldo sparkline. Positive bars = active
  // color (above the zero line), negative bars = err color (below).
  window.initSaldoSpark = function (targetId, dataId) {
    if (typeof ApexCharts !== 'function' && typeof ApexCharts !== 'object') return;
    var target = document.getElementById(targetId);
    if (!target) return;
    var bars = readJSON(dataId);
    if (!bars || !bars.length) return;

    var active = cssVar('--color-active') || '#9ece6a';
    var err = cssVar('--color-err') || '#f7768e';
    var muted = cssVar('--color-muted') || '#414868';

    var values = bars.map(function (b) { return Number(b.saldo.toFixed(2)); });
    var colors = bars.map(function (b) { return b.pos ? active : err; });

    var options = {
      chart: {
        type: 'bar',
        height: 60,
        sparkline: { enabled: true },
        animations: { enabled: false },
        background: 'transparent',
      },
      series: [{ name: 'Saldo (h)', data: values }],
      plotOptions: {
        bar: { columnWidth: '60%', distributed: true },
      },
      colors: colors,
      dataLabels: { enabled: false },
      legend: { show: false },
      tooltip: {
        custom: function (ctx) {
          var b = bars[ctx.dataPointIndex];
          if (!b) return '';
          var sign = b.saldo >= 0 ? '+' : '';
          return '<div style="padding:4px 8px; font-family: var(--font-mono); font-size:11px; background: var(--color-bg-dark); border: 1px solid ' + muted + ';">'
            + b.label + ' · ' + sign + b.saldo.toFixed(1) + 'h'
            + '</div>';
        },
      },
    };
    var chart = new ApexCharts(target, options);
    chart.render();
  };
})();
