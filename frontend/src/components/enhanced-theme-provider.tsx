'use client';

import React, { createContext, useContext, useEffect, useState, useCallback } from 'react';
import { ThemeProvider as NextThemesProvider, useTheme as useNextTheme } from 'next-themes';
import { Debug } from '@/utils/debug';

export interface ThemeDefinition {
  id: string;
  name: string;
  description?: string;
  source?: 'builtin' | 'custom';
  colors?: {
    light: {
      primary: string;
      accent: string;
      background: string;
      secondary: string;
    };
    dark: {
      primary: string;
      accent: string;
      background: string;
      secondary: string;
    };
  };
}

interface ColorThemeContextType {
  colorTheme: string;
  themes: ThemeDefinition[];
  setColorTheme: (theme: string) => void;
}

const ColorThemeContext = createContext<ColorThemeContextType | undefined>(undefined);

// Hook that combines next-themes mode with our color theme
export function useTheme() {
  const { theme: mode, setTheme: setMode, resolvedTheme } = useNextTheme();
  const colorContext = useContext(ColorThemeContext);

  if (colorContext === undefined) {
    throw new Error('useTheme must be used within a ThemeProvider');
  }

  return {
    theme: colorContext.colorTheme,
    mode: (mode || 'system') as 'light' | 'dark' | 'system',
    actualMode: (resolvedTheme || 'light') as 'light' | 'dark',
    themes: colorContext.themes,
    setTheme: colorContext.setColorTheme,
    setMode: (newMode: 'light' | 'dark' | 'system') => setMode(newMode),
  };
}

interface ColorThemeProviderProps {
  children: React.ReactNode;
  defaultColorTheme: string;
}

// Inner provider that handles color themes (runs after next-themes is initialized)
function ColorThemeProvider({ children, defaultColorTheme }: ColorThemeProviderProps) {
  const [colorTheme, setColorThemeState] = useState<string>(() => {
    // Initialize from localStorage on client
    if (typeof window !== 'undefined') {
      return localStorage.getItem('color-theme') || defaultColorTheme;
    }
    return defaultColorTheme;
  });
  const [themes, setThemes] = useState<ThemeDefinition[]>([]);
  const [mounted, setMounted] = useState(false);

  // Get CSS URL for a theme - static for built-in, API for custom
  const getThemeCssUrl = useCallback(
    (themeName: string, isCustom: boolean) => {
      // Custom themes must come from API, built-in themes use static path
      return isCustom ? `/api/v1/themes/${themeName}.css` : `/themes/${themeName}.css`;
    },
    []
  );

  // Parse CSS to extract color values from :root and .dark blocks
  const parseThemeColors = useCallback(async (themeId: string, isCustom: boolean = false) => {
    try {
      const response = await fetch(getThemeCssUrl(themeId, isCustom));
      if (!response.ok) {
        throw new Error(`Failed to load theme CSS: ${themeId}`);
      }

      const cssText = await response.text();

      const extractColors = (section: string) => {
        const primaryMatch = section.match(/--primary:\s*([^;]+);/);
        const accentMatch = section.match(/--accent:\s*([^;]+);/);
        const backgroundMatch = section.match(/--background:\s*([^;]+);/);
        const secondaryMatch = section.match(/--secondary:\s*([^;]+);/);

        return {
          primary: primaryMatch?.[1]?.trim() || '',
          accent: accentMatch?.[1]?.trim() || '',
          background: backgroundMatch?.[1]?.trim() || '',
          secondary: secondaryMatch?.[1]?.trim() || '',
        };
      };

      // Extract light mode colors from :root block
      const rootMatch = cssText.match(/:root\s*\{([^}]+)\}/);
      const lightColors = rootMatch
        ? extractColors(rootMatch[1])
        : { primary: '', accent: '', background: '', secondary: '' };

      // Extract dark mode colors from .dark block
      const darkMatch = cssText.match(/\.dark\s*\{([^}]+)\}/);
      const darkColors = darkMatch
        ? extractColors(darkMatch[1])
        : { primary: '', accent: '', background: '', secondary: '' };

      return { light: lightColors, dark: darkColors };
    } catch (error) {
      console.error(`Failed to parse colors for theme ${themeId}:`, error);
      return null;
    }
  }, [getThemeCssUrl]);

  // Apply color theme by loading CSS
  const applyColorTheme = useCallback((themeName: string, isCustom: boolean = false) => {
    // Built-in themes use static path, custom themes use API
    const cssUrl = getThemeCssUrl(themeName, isCustom);

    // Update the theme CSS link
    const themeLink = document.getElementById('theme-css') as HTMLLinkElement | null;
    if (themeLink) {
      if (!themeLink.href.endsWith(cssUrl)) {
        themeLink.href = cssUrl;
        Debug.log(`Theme CSS updated: ${themeName}`);
      }
    } else {
      // Fallback: create link if it doesn't exist
      const existingLinks = document.querySelectorAll('link[data-theme-css]');
      existingLinks.forEach((link) => link.remove());

      const link = document.createElement('link');
      link.rel = 'stylesheet';
      link.id = 'theme-css';
      link.href = cssUrl;
      link.setAttribute('data-theme-css', themeName);
      link.onload = () => Debug.log(`Theme CSS loaded: ${themeName}`);
      link.onerror = () => console.error(`Failed to load theme CSS: ${themeName}`);
      document.head.appendChild(link);
    }

    Debug.log(`Applied color theme: ${themeName} (${isCustom ? 'custom' : 'builtin'})`);
  }, [getThemeCssUrl]);

  // Load themes from API
  useEffect(() => {
    const loadThemes = async () => {
      let themeData: { themes: ThemeDefinition[]; default: string } | null = null;

      try {
        const apiResponse = await fetch('/api/v1/themes');
        if (apiResponse.ok) {
          themeData = await apiResponse.json();
          Debug.log('Loaded themes from API:', themeData);
        }
      } catch (error) {
        Debug.log('Failed to load themes from API:', error);
      }

      if (themeData && themeData.themes) {
        // Parse colors for themes that don't have them (API should provide colors, but fallback)
        const themesWithColors: ThemeDefinition[] = await Promise.all(
          themeData.themes.map(async (themeItem: ThemeDefinition): Promise<ThemeDefinition> => {
            if (themeItem.colors) {
              return themeItem;
            }
            const isCustom = themeItem.source === 'custom';
            const colors = await parseThemeColors(themeItem.id, isCustom);
            return {
              ...themeItem,
              colors: colors || undefined,
            };
          })
        );

        // Sort themes: default first, then built-in, then custom, then alphabetically
        const defaultThemeId = themeData.default || 'graphite';
        const sortedThemes = [...themesWithColors].sort((a, b) => {
          if (a.id === defaultThemeId && b.id !== defaultThemeId) return -1;
          if (b.id === defaultThemeId && a.id !== defaultThemeId) return 1;
          if (a.source === 'builtin' && b.source === 'custom') return -1;
          if (a.source === 'custom' && b.source === 'builtin') return 1;
          return a.name.localeCompare(b.name);
        });

        setThemes(sortedThemes);

        // Check if current color theme still exists
        const savedTheme = localStorage.getItem('color-theme');
        if (savedTheme && !sortedThemes.find((t) => t.id === savedTheme)) {
          Debug.log(`Theme "${savedTheme}" no longer exists, resetting to default`);
          setColorThemeState(defaultThemeId);
          localStorage.setItem('color-theme', defaultThemeId);
          applyColorTheme(defaultThemeId, false); // default is always builtin
        }
      } else {
        // Fallback to default theme
        console.error('Failed to load themes from API');
        setThemes([
          {
            id: 'graphite',
            name: 'Graphite',
            description: 'Default theme',
            source: 'builtin',
          },
        ]);
      }

      setMounted(true);
    };

    loadThemes();
  }, [parseThemeColors, applyColorTheme]);

  // Apply color theme on mount and when it changes
  useEffect(() => {
    if (!mounted) return;
    const themeInfo = themes.find((t) => t.id === colorTheme);
    const isCustom = themeInfo?.source === 'custom';
    applyColorTheme(colorTheme, isCustom);
  }, [colorTheme, mounted, themes, applyColorTheme]);

  const setColorTheme = useCallback(
    (newTheme: string) => {
      const themeInfo = themes.find((t) => t.id === newTheme);
      const isCustom = themeInfo?.source === 'custom';
      setColorThemeState(newTheme);
      localStorage.setItem('color-theme', newTheme);
      applyColorTheme(newTheme, isCustom);
    },
    [themes, applyColorTheme]
  );

  return (
    <ColorThemeContext.Provider
      value={{
        colorTheme,
        themes,
        setColorTheme,
      }}
    >
      {children}
    </ColorThemeContext.Provider>
  );
}

interface ThemeProviderProps {
  children: React.ReactNode;
  defaultTheme?: string;
  defaultMode?: 'light' | 'dark' | 'system';
}

// Main provider that wraps next-themes with our color theme provider
export function EnhancedThemeProvider({
  children,
  defaultTheme = 'graphite',
  defaultMode = 'system',
}: ThemeProviderProps) {
  return (
    <NextThemesProvider
      attribute="class"
      defaultTheme={defaultMode}
      enableSystem
      disableTransitionOnChange
    >
      <ColorThemeProvider defaultColorTheme={defaultTheme}>{children}</ColorThemeProvider>
    </NextThemesProvider>
  );
}
