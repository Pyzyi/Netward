// Applies the saved theme before the first paint (kept external for the CSP).
(function () {
  try {
    var t = localStorage.getItem('netzwart.theme');
    if (t === 'light' || t === 'dark') document.documentElement.dataset.theme = t;
  } catch (e) {}
})();
