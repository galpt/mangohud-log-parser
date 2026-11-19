// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 Galih Tama <galpt@v.recipes>

// Platform hooks for server-side rendering.
//
// ECharts expects a browser. This file supplies the few globals it touches
// while it renders to an SVG string inside goja, which has none of them.

// SSR renders synchronously, so a timer callback never has to fire. ECharts
// only schedules them for animation frames and deferred work that SSR skips.
// This runs before ECharts loads because some of its modules read the globals
// at load time.
var timerId = 0;
globalThis.setTimeout = function () { return ++timerId; };
globalThis.setInterval = globalThis.setTimeout;
globalThis.clearTimeout = function () {};
globalThis.clearInterval = function () {};

// A few ECharts code paths call console.warn without checking that it exists.
// Library callers have no use for the output, so it is dropped.
globalThis.console = {
  log: function () {},
  info: function () {},
  warn: function () {},
  error: function () {}
};

// Text is measured by Go with the embedded font, so layout matches what resvg
// draws later. ECharts falls back to a canvas for this, which does not exist
// here.
function installPlatform(measure) {
  echarts.setPlatformAPI({
    measureText: function (text, font) {
      return { width: measure(text, font) };
    }
  });
}

// Each call builds a chart and disposes of it, so nothing survives between
// renders.
function renderSVG(optionJSON, width, height) {
  const option = JSON.parse(optionJSON);
  if (option === null || typeof option !== 'object' || Array.isArray(option)) {
    throw new Error('option must be a JSON object');
  }
  const chart = echarts.init(null, null, { renderer: 'svg', ssr: true, width: width, height: height });
  try {
    chart.setOption(option);
    return chart.renderToSVGString();
  } finally {
    chart.dispose();
  }
}
