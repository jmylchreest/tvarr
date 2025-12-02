'use client';

import React, { createContext, useContext, useEffect, useState, useCallback } from 'react';
import { Debug } from '@/utils/debug';

export interface ThemeDefinition {
  id: string;
  name: string;
  description?: string;
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

type ThemeMode = 'light' | 'dark' | 'system';

interface ThemeContextType {
  theme: string;
  mode: ThemeMode;
  actualMode: 'light' | 'dark';
  themes: ThemeDefinition[];
  setTheme: (theme: string) => void;
  setMode: (mode: ThemeMode) => void;
}

const ThemeContext = createContext<ThemeContextType | undefined>(undefined);

export function useTheme() {
  const context = useContext(ThemeContext);
  if (context === undefined) {
    throw new Error('useTheme must be used within a ThemeProvider');
  }
  return context;
}

interface ThemeProviderProps {
  children: React.ReactNode;
  defaultTheme?: string;
  defaultMode?: ThemeMode;
}

export function EnhancedThemeProvider({
  children,
  defaultTheme: defaultThemeProp = 'graphite',
  defaultMode = 'system',
}: ThemeProviderProps) {
  const [theme, setThemeState] = useState<string>(defaultThemeProp);
  const [mode, setModeState] = useState<ThemeMode>(defaultMode);
  const [actualMode, setActualMode] = useState<'light' | 'dark'>('light');
  const [themes, setThemes] = useState<ThemeDefinition[]>([]);
  const [isLoading, setIsLoading] = useState(true);

  // Parse CSS to extract color values
  const parseThemeColors = useCallback(async (themeId: string) => {
    try {
      const response = await fetch(`/themes/${themeId}.css`);
      if (!response.ok) {
        throw new Error(`Failed to load theme CSS: ${themeId}`);
      }

      const cssText = await response.text();

      // Extract colors from :root and .dark sections
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

      // Extract light mode colors from :root
      const rootMatch = cssText.match(/:root\s*\{([^}]+)\}/);
      const lightColors = rootMatch
        ? extractColors(rootMatch[1])
        : {
            primary: '',
            accent: '',
            background: '',
            secondary: '',
          };

      // Extract dark mode colors from .dark
      const darkMatch = cssText.match(/\.dark\s*\{([^}]+)\}/);
      const darkColors = darkMatch
        ? extractColors(darkMatch[1])
        : {
            primary: '',
            accent: '',
            background: '',
            secondary: '',
          };

      return {
        light: lightColors,
        dark: darkColors,
      };
    } catch (error) {
      console.error(`Failed to parse colors for theme ${themeId}:`, error);
      return null;
    }
  }, []);

  const applyTheme = useCallback((themeName: string, actualMode: 'light' | 'dark') => {
    // Remove existing theme stylesheets
    const existingLinks = document.querySelectorAll('link[data-theme-css]');
    existingLinks.forEach((link) => link.remove());

    // Add new theme stylesheet
    const link = document.createElement('link');
    link.rel = 'stylesheet';
    link.href = `/themes/${themeName}.css`;
    link.setAttribute('data-theme-css', themeName);
    link.onload = () => Debug.log(`Theme CSS loaded: ${themeName}`);
    link.onerror = () => console.error(`Failed to load theme CSS: ${themeName}`);
    document.head.appendChild(link);

    // Apply dark mode class - the CSS uses :root and .dark structure
    document.documentElement.classList.remove('dark');
    if (actualMode === 'dark') {
      document.documentElement.classList.add('dark');
    }

    Debug.log(`Applied theme: ${themeName} (${actualMode})`);
    Debug.log(`Document classes:`, document.documentElement.className);
    Debug.log(`Has dark class:`, document.documentElement.classList.contains('dark'));
  }, []);

  // Load themes from JSON file
  useEffect(() => {
    const loadThemes = async () => {
      try {
        const response = await fetch('/themes/themes.json');
        if (!response.ok) {
          throw new Error('Failed to load themes');
        }
        const themeData = await response.json();
        const themesWithColors = await Promise.all(
          (themeData.themes || []).map(async (theme: ThemeDefinition) => {
            const colors = await parseThemeColors(theme.id);
            return {
              ...theme,
              colors,
            };
          })
        );

        // Sort themes: default theme first, then alphabetically by name
        const defaultTheme = themeData.default || 'graphite';
        const sortedThemes = [...themesWithColors].sort((a, b) => {
          // If one is the default theme, it goes first
          if (a.id === defaultTheme && b.id !== defaultTheme) return -1;
          if (b.id === defaultTheme && a.id !== defaultTheme) return 1;
          // If both are default or both are not default, sort alphabetically
          return a.name.localeCompare(b.name);
        });

        setThemes(sortedThemes);

        // If no saved theme, use the default from JSON
        const savedTheme = localStorage.getItem('theme');
        if (!savedTheme) {
          setThemeState(themeData.default || 'graphite');
        }
      } catch (error) {
        console.error('Failed to load themes:', error);
        // Fallback to default theme with basic info
        setThemes([
          {
            id: 'graphite',
            name: 'Graphite',
            description: 'Default theme',
          },
        ]);
        setThemeState('graphite');
      } finally {
        setIsLoading(false);
      }
    };

    loadThemes();
  }, [parseThemeColors]);

  useEffect(() => {
    if (isLoading) return;

    // Load theme from localStorage
    const savedTheme = localStorage.getItem('theme');
    const savedMode = localStorage.getItem('mode') as ThemeMode | null;

    const finalTheme =
      savedTheme && themes.find((t) => t.id === savedTheme) ? savedTheme : defaultThemeProp;
    const finalMode = savedMode || defaultMode;

    setThemeState(finalTheme);
    setModeState(finalMode);

    // Apply theme immediately
    const immediateActualMode =
      finalMode === 'system'
        ? window.matchMedia('(prefers-color-scheme: dark)').matches
          ? 'dark'
          : 'light'
        : (finalMode as 'light' | 'dark');

    setActualMode(immediateActualMode);
    applyTheme(finalTheme, immediateActualMode);
  }, [applyTheme, defaultThemeProp, defaultMode, themes, isLoading]);

  useEffect(() => {
    const calculateActualMode = () => {
      if (mode === 'system') {
        return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
      }
      return mode;
    };

    const updateActualMode = () => {
      const newActualMode = calculateActualMode();
      setActualMode(newActualMode);
      applyTheme(theme, newActualMode);
    };

    // Initial calculation
    updateActualMode();

    if (mode === 'system') {
      const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)');
      const listener = () => updateActualMode();
      mediaQuery.addEventListener('change', listener);
      return () => mediaQuery.removeEventListener('change', listener);
    }
  }, [mode, theme, applyTheme]);

  const setTheme = (newTheme: string) => {
    setThemeState(newTheme);
    localStorage.setItem('theme', newTheme);
    applyTheme(newTheme, actualMode);
  };

  const setMode = (newMode: ThemeMode) => {
    setModeState(newMode);
    localStorage.setItem('mode', newMode);
  };

  return (
    <ThemeContext.Provider
      value={{
        theme,
        mode,
        actualMode,
        themes,
        setTheme,
        setMode,
      }}
    >
      {children}
    </ThemeContext.Provider>
  );
}
