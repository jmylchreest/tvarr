'use client';

import { useMemo } from 'react';
import { Check, Monitor, Sun, Moon, Palette } from 'lucide-react';
import { useTheme, ThemeDefinition } from '@/components/enhanced-theme-provider';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
  DropdownMenuSeparator,
  DropdownMenuLabel,
} from '@/components/ui/dropdown-menu';

export function EnhancedThemeSelector() {
  const { theme, mode, actualMode, themes, setTheme, setMode } = useTheme();

  // Group themes by source (built-in vs custom)
  const { builtinThemes, customThemes } = useMemo(() => {
    const builtin: ThemeDefinition[] = [];
    const custom: ThemeDefinition[] = [];

    themes.forEach((t) => {
      if (t.source === 'custom') {
        custom.push(t);
      } else {
        builtin.push(t);
      }
    });

    return { builtinThemes: builtin, customThemes: custom };
  }, [themes]);

  const getModeIcon = (themeMode: string) => {
    switch (themeMode) {
      case 'light':
        return <Sun className="h-4 w-4" />;
      case 'dark':
        return <Moon className="h-4 w-4" />;
      case 'system':
        return <Monitor className="h-4 w-4" />;
      default:
        return <Monitor className="h-4 w-4" />;
    }
  };

  const ColorPreview = ({
    colors,
  }: {
    colors?: { primary: string; accent: string; background: string; secondary: string } | null;
  }) => {
    if (!colors) {
      return (
        <div className="flex gap-1">
          {[1, 2, 3, 4].map((i) => (
            <div
              key={i}
              className="w-3 h-3 rounded-sm border border-border/50 bg-muted"
              title="Loading..."
            />
          ))}
        </div>
      );
    }

    return (
      <div className="flex gap-1">
        {Object.entries({
          primary: colors.primary,
          accent: colors.accent,
          background: colors.background,
          secondary: colors.secondary,
        }).map(([name, color]) => (
          <div
            key={name}
            className="w-3 h-3 rounded-sm border border-border/50"
            style={{ backgroundColor: color }}
            title={name}
          />
        ))}
      </div>
    );
  };

  const ThemeMenuItem = ({ themeOption }: { themeOption: ThemeDefinition }) => (
    <DropdownMenuItem
      key={themeOption.id}
      onClick={() => setTheme(themeOption.id)}
      className="flex items-center justify-between py-2"
    >
      <div className="flex items-center gap-3">
        <ColorPreview colors={themeOption.colors?.[actualMode]} />
        <span>{themeOption.name}</span>
        {themeOption.source === 'custom' && (
          <Badge variant="outline" className="text-xs px-1.5 py-0 h-4">
            Custom
          </Badge>
        )}
      </div>
      {theme === themeOption.id && <Check className="h-4 w-4" />}
    </DropdownMenuItem>
  );

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="sm" className="w-9 h-9">
          <Palette className="h-4 w-4" />
          <span className="sr-only">Theme selector</span>
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-64 dropdown-backdrop max-h-[70vh] overflow-y-auto">
        <DropdownMenuLabel>Mode</DropdownMenuLabel>

        {(['system', 'light', 'dark'] as const).map((themeMode) => (
          <DropdownMenuItem
            key={themeMode}
            onClick={() => setMode(themeMode)}
            className="flex items-center justify-between"
          >
            <div className="flex items-center gap-2">
              {getModeIcon(themeMode)}
              <span className="capitalize">{themeMode}</span>
            </div>
            {mode === themeMode && <Check className="h-4 w-4" />}
          </DropdownMenuItem>
        ))}

        <DropdownMenuSeparator />
        <DropdownMenuLabel>Built-in Themes</DropdownMenuLabel>

        {builtinThemes.map((themeOption) => (
          <ThemeMenuItem key={themeOption.id} themeOption={themeOption} />
        ))}

        {customThemes.length > 0 && (
          <>
            <DropdownMenuSeparator />
            <DropdownMenuLabel>Custom Themes</DropdownMenuLabel>
            {customThemes.map((themeOption) => (
              <ThemeMenuItem key={themeOption.id} themeOption={themeOption} />
            ))}
          </>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
