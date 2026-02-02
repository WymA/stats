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

const setCopyFeedback = (button) => {
  button.classList.add('copied');
  window.setTimeout(() => button.classList.remove('copied'), 1200);
};

const handleCopy = async (value, button) => {
  if (!value) {
    return;
  }
  if (navigator.clipboard && window.isSecureContext) {
    try {
      await navigator.clipboard.writeText(value);
      setCopyFeedback(button);
      return;
    } catch (error) {
      // Fall back to legacy copy.
    }
  }
  const textarea = document.createElement('textarea');
  textarea.value = value;
  textarea.setAttribute('readonly', '');
  textarea.style.position = 'absolute';
  textarea.style.left = '-9999px';
  document.body.appendChild(textarea);
  textarea.select();
  try {
    document.execCommand('copy');
    setCopyFeedback(button);
  } finally {
    document.body.removeChild(textarea);
  }
};

document.addEventListener('click', (event) => {
  const target = event.target.closest('.copy-button');
  if (!target) {
    return;
  }
  const value = target.getAttribute('data-copy');
  handleCopy(value, target);
});

if (toggle) {
  setTheme(initial === 'dark');
  toggle.addEventListener('click', () => {
    const isDark = !document.body.classList.contains('dark');
    setTheme(isDark);
    localStorage.setItem('theme', isDark ? 'dark' : 'light');
  });
}
