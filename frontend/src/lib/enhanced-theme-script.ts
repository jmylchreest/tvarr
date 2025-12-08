// Synchronous theme initialization script.
// This script must execute BEFORE any content paints to prevent the "flashbang" effect.
// It reads localStorage synchronously and applies the theme immediately.
//
// This works in conjunction with next-themes for light/dark mode and our custom
// color theme system for theme palette selection.
export const enhancedThemeScript = `
(function() {
  var COLOR_THEME_KEY = 'color-theme';
  var MODE_KEY = 'theme'; // next-themes uses 'theme' key
  var DEFAULT_COLOR_THEME = 'graphite';
  var DEFAULT_MODE = 'system';

  // Read preferences synchronously from localStorage
  var savedColorTheme = null;
  var savedMode = null;
  try {
    savedColorTheme = localStorage.getItem(COLOR_THEME_KEY);
    savedMode = localStorage.getItem(MODE_KEY);
  } catch (e) {
    // localStorage may be blocked in some contexts
  }

  var colorTheme = savedColorTheme || DEFAULT_COLOR_THEME;
  var mode = savedMode || DEFAULT_MODE;

  // Determine actual mode (resolve 'system' to 'light' or 'dark')
  var actualMode = mode;
  if (mode === 'system') {
    actualMode = window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
  }

  // Apply dark class immediately BEFORE any content renders
  // next-themes will take over after hydration, but this prevents flash
  if (actualMode === 'dark') {
    document.documentElement.classList.add('dark');
  } else {
    document.documentElement.classList.remove('dark');
  }

  // Update the theme CSS link if it exists (placed in head by layout.tsx)
  // Use static path for built-in themes (fast, no backend dependency)
  // Custom themes will be switched to API path by the theme provider after hydration
  var themeLink = document.getElementById('theme-css');
  if (themeLink) {
    themeLink.href = '/themes/' + colorTheme + '.css';
  }

  // Store the initialized values for debugging/reference
  window.__THEME_INIT__ = {
    colorTheme: colorTheme,
    mode: mode,
    actualMode: actualMode
  };
})();
`;
