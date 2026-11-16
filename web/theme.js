// Applies the saved theme before the page is drawn (no flash). Default: neon.
(function () {
  var t = 'neon';
  try { t = localStorage.getItem('kw-theme') || 'neon'; } catch (e) { /* storage blocked */ }
  if (['neon', 'light', 'dark', 'terminal'].indexOf(t) < 0) t = 'neon';
  document.documentElement.dataset.theme = t;
})();
