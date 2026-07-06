// theme.js — Theme management
(function() {
  const KEY = 'na-theme';

  function getTheme() {
    return localStorage.getItem(KEY) || 'dark';
  }

  function setTheme(theme) {
    document.documentElement.setAttribute('data-theme', theme);
    localStorage.setItem(KEY, theme);
    document.dispatchEvent(new CustomEvent('na:theme', {detail: {theme}}));
  }

  function toggle() {
    const cur = document.documentElement.getAttribute('data-theme');
    setTheme(cur === 'dark' ? 'light' : 'dark');
  }

  // Apply immediately to avoid flash
  setTheme(getTheme());

  window.NATheme = {getTheme, setTheme, toggle};
})();
