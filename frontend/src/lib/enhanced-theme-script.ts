export const enhancedThemeScript = `
(function() {
  const THEME_KEY = 'theme';
  const MODE_KEY = 'mode';
  const DEFAULT_THEME = 'graphite';
  const DEFAULT_MODE = 'system';

  function getSystemMode() {
    return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
  }

  function getActualMode(mode) {
    return mode === 'system' ? getSystemMode() : mode;
  }

  function loadThemeCSS(themeName) {
    // Remove other theme stylesheets
    const existingLinks = document.querySelectorAll('link[data-theme-css]');
    existingLinks.forEach(function(link) { link.remove(); });

    // Add new theme stylesheet
    const link = document.createElement('link');
    link.rel = 'stylesheet';
    link.href = '/themes/' + themeName + '.css';
    link.setAttribute('data-theme-css', themeName);
    
    // Insert after globals.css to ensure proper override
    const lastLink = document.querySelector('link[rel="stylesheet"]:last-of-type');
    if (lastLink) {
      lastLink.parentNode.insertBefore(link, lastLink.nextSibling);
    } else {
      document.head.appendChild(link);
    }
  }

  function applyThemeClass(actualMode) {
    // Apply dark mode class - CSS uses :root and .dark structure
    document.documentElement.classList.remove('dark');
    if (actualMode === 'dark') {
      document.documentElement.classList.add('dark');
    }
  }

  async function loadDefaultTheme() {
    try {
      const response = await fetch('/themes/themes.json');
      if (response.ok) {
        const themeData = await response.json();
        return themeData.default || DEFAULT_THEME;
      }
    } catch (e) {
      console.warn('Could not load themes.json, using fallback');
    }
    return DEFAULT_THEME;
  }

  async function initializeTheme() {
    try {
      const defaultTheme = await loadDefaultTheme();
      const savedTheme = localStorage.getItem(THEME_KEY) || defaultTheme;
      const savedMode = localStorage.getItem(MODE_KEY) || DEFAULT_MODE;
      const actualMode = getActualMode(savedMode);

      // Load theme CSS immediately
      loadThemeCSS(savedTheme);
      
      // Apply theme class immediately
      applyThemeClass(actualMode);
    } catch (e) {
      // Fallback if everything fails
      loadThemeCSS(DEFAULT_THEME);
      applyThemeClass(getActualMode(DEFAULT_MODE));
    }
  }

  // Initialize theme
  initializeTheme();
})();
`;
