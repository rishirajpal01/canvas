(() => {
  let stored;
  try { stored = localStorage.getItem('kaleido-theme'); } catch (_) {}
  const theme = stored === 'dark' ? 'dark' : 'light';
  document.documentElement.dataset.theme = theme;

  document.addEventListener('DOMContentLoaded', () => {
    const button = document.querySelector('[data-theme-toggle]');
    if (!button) return;
    const label = button.querySelector('[data-theme-label]');
    function sync() {
      const dark = document.documentElement.dataset.theme === 'dark';
      button.setAttribute('aria-pressed', String(dark));
      label.textContent = dark ? 'Light mode' : 'Dark mode';
    }
    button.addEventListener('click', () => {
      const next = document.documentElement.dataset.theme === 'dark' ? 'light' : 'dark';
      document.documentElement.dataset.theme = next;
      try { localStorage.setItem('kaleido-theme', next); } catch (_) {}
      sync();
      document.dispatchEvent(new Event('themechange'));
    });
    sync();
  });
})();
