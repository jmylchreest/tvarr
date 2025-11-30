'use client';

import { EnhancedThemeSelector } from '@/components/enhanced-theme-selector';
import { NotificationBell } from '@/components/NotificationBell';

interface AppHeaderProps {
  title: string;
}

function getOperationTypeForPage(title: string): string | undefined {
  switch (title) {
    case 'Stream Sources':
      return 'stream_ingestion';
    case 'EPG Sources':
      return 'epg_ingestion';
    case 'Proxies':
      return 'proxy_regeneration';
    default:
      return undefined;
  }
}

export function AppHeader({ title }: AppHeaderProps) {
  return (
    <header className="sticky top-0 z-20 flex h-16 shrink-0 items-center justify-between gap-2 px-6 border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
      <h1 className="text-2xl font-bold">{title}</h1>

      <div className="flex items-center gap-2">
        <NotificationBell operationType={getOperationTypeForPage(title)} />
        {/* Theme selector in top-right */}
        <EnhancedThemeSelector />
      </div>
    </header>
  );
}
