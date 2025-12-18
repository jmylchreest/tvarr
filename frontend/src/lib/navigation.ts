import {
  Play,
  Filter,
  Image,
  Activity,
  Server,
  Database,
  Zap,
  Bug,
  FileText,
  Settings,
  Radio,
  ArrowUpDown,
  Tv,
  Calendar,
  Paintbrush,
  Users,
  Archive,
  LucideIcon,
} from 'lucide-react';

export interface NavigationItem {
  title: string;
  url: string;
  icon: LucideIcon;
  operationType?: string;
}

export interface NavigationGroup {
  title: string;
  items: NavigationItem[];
}

export const navigation: NavigationGroup[] = [
  {
    title: 'Overview',
    items: [
      {
        title: 'Dashboard',
        url: '/',
        icon: Activity,
      },
      {
        title: 'Channel Browser',
        url: '/channels',
        icon: Tv,
      },
      {
        title: 'EPG Viewer',
        url: '/epg',
        icon: Calendar,
      },
    ],
  },
  {
    title: 'Proxy Config',
    items: [
      {
        title: 'Stream Sources',
        url: '/sources/stream',
        icon: Database,
        operationType: 'stream_ingestion',
      },
      {
        title: 'EPG Sources',
        url: '/sources/epg',
        icon: Server,
        operationType: 'epg_ingestion',
      },
      {
        title: 'Proxies',
        url: '/proxies',
        icon: Play,
        operationType: 'proxy_regeneration',
      },
    ],
  },
  {
    title: 'Global Config',
    items: [
      {
        title: 'Data Mapping',
        url: '/admin/data-mapping',
        icon: ArrowUpDown,
      },
      {
        title: 'Filters',
        url: '/admin/filters',
        icon: Filter,
      },
      {
        title: 'Encoding Profiles',
        url: '/admin/encoding-profiles',
        icon: Zap,
      },
      {
        title: 'Client Detection',
        url: '/admin/client-detection',
        icon: Users,
      },
      {
        title: 'Logos',
        url: '/admin/logos',
        icon: Image,
      },
      {
        title: 'Backups',
        url: '/admin/backups',
        icon: Archive,
      },
    ],
  },
  {
    title: 'Debug',
    items: [
      {
        title: 'Debug',
        url: '/debug',
        icon: Bug,
      },
      {
        title: 'Settings',
        url: '/settings',
        icon: Settings,
      },
      {
        title: 'Events',
        url: '/events',
        icon: Activity,
      },
      {
        title: 'Logs',
        url: '/logs',
        icon: FileText,
      },
      {
        title: 'Colour Palette',
        url: '/color-palette',
        icon: Paintbrush,
      },
    ],
  },
];

// Build a flat lookup map for efficient title/operation lookups
const navigationLookup = new Map<string, NavigationItem>();
for (const group of navigation) {
  for (const item of group.items) {
    navigationLookup.set(item.url, item);
  }
}

/**
 * Get the page title for a given pathname.
 * Returns 'tvarr' if no matching navigation item is found.
 */
export function getPageTitle(pathname: string): string {
  const normalizedPathname =
    pathname.endsWith('/') && pathname !== '/' ? pathname.slice(0, -1) : pathname;

  const item = navigationLookup.get(normalizedPathname);
  return item?.title ?? 'tvarr';
}

/**
 * Get the operation type for a given pathname (for notification bell filtering).
 * Returns undefined if no operation type is configured for the path.
 */
export function getOperationType(pathname: string): string | undefined {
  const normalizedPathname =
    pathname.endsWith('/') && pathname !== '/' ? pathname.slice(0, -1) : pathname;

  const item = navigationLookup.get(normalizedPathname);
  return item?.operationType;
}

export { Radio };
