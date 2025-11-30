'use client';

import { Check, Monitor, Sun, Moon, Palette } from 'lucide-react';
import { useTheme } from '@/components/enhanced-theme-provider';
import { Button } from '@/components/ui/button';
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

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="sm" className="w-9 h-9">
          <Palette className="h-4 w-4" />
          <span className="sr-only">Theme selector</span>
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-56 dropdown-backdrop">
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
        <DropdownMenuLabel>Theme</DropdownMenuLabel>

        {themes.map((themeOption) => (
          <DropdownMenuItem
            key={themeOption.id}
            onClick={() => setTheme(themeOption.id)}
            className="flex items-center justify-between py-2"
          >
            <div className="flex items-center gap-3">
              <ColorPreview colors={themeOption.colors?.[actualMode]} />
              <span>{themeOption.name}</span>
            </div>
            {theme === themeOption.id && <Check className="h-4 w-4" />}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
