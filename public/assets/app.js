const toggle = document.getElementById('theme-toggle');
const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
const stored = localStorage.getItem('theme');
const initial = stored || (prefersDark ? 'dark' : 'light');
if (initial === 'dark') {
  document.body.classList.add('dark');
  toggle.textContent = 'Light mode';
}
toggle.addEventListener('click', () => {
  const isDark = document.body.classList.toggle('dark');
  toggle.textContent = isDark ? 'Light mode' : 'Dark mode';
  localStorage.setItem('theme', isDark ? 'dark' : 'light');
});
