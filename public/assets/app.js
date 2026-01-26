const toggle = document.getElementById('theme-toggle');
const themeIcon = toggle ? toggle.querySelector('i') : null;
const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
const stored = localStorage.getItem('theme');
const initial = stored || (prefersDark ? 'dark' : 'light');

const setTheme = (isDark) => {
  document.body.classList.toggle('dark', isDark);
  toggle.setAttribute('aria-label', isDark ? 'Light mode' : 'Dark mode');
  if (!themeIcon) {
    return;
  }
  themeIcon.className = isDark ? 'fa-solid fa-moon' : 'fa-regular fa-moon';
};

if (toggle) {
  setTheme(initial === 'dark');
  toggle.addEventListener('click', () => {
    const isDark = !document.body.classList.contains('dark');
    setTheme(isDark);
    localStorage.setItem('theme', isDark ? 'dark' : 'light');
  });
}
