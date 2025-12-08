'use client';

import {
  Play,
  Filter,
  Image,
  Activity,
  Server,
  Database,
  Zap,
  Palette,
  Bug,
  FileText,
  Settings,
  Radio,
  ArrowUpDown,
  Tv,
  Calendar,
  Paintbrush,
  ChevronRight,
  ChevronDown,
  Users,
} from 'lucide-react';
import { useState } from 'react';
import { usePathname } from 'next/navigation';
import Link from 'next/link';
import {
  Sidebar,
  SidebarHeader,
  SidebarContent,
  SidebarGroup,
  SidebarGroupLabel,
  SidebarGroupContent,
  SidebarMenu,
  SidebarMenuItem,
  SidebarMenuButton,
} from '@/components/ui/sidebar';

const navigation = [
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
      },
      {
        title: 'EPG Sources',
        url: '/sources/epg',
        icon: Server,
      },
      {
        title: 'Proxies',
        url: '/proxies',
        icon: Play,
      },
    ],
  },
  {
    title: 'Global Config',
    items: [
      {
        title: 'Filters',
        url: '/admin/filters',
        icon: Filter,
      },
      {
        title: 'Data Mapping',
        url: '/admin/data-mapping',
        icon: ArrowUpDown,
      },
      {
        title: 'Logos',
        url: '/admin/logos',
        icon: Image,
      },
      {
        title: 'Relay Profiles',
        url: '/admin/relays',
        icon: Zap,
      },
      {
        title: 'Client Detection',
        url: '/admin/client-detection',
        icon: Users,
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

function getOperationTypeForPath(pathname: string): string | undefined {
  // Remove trailing slash for consistent matching
  const normalizedPathname =
    pathname.endsWith('/') && pathname !== '/' ? pathname.slice(0, -1) : pathname;

  switch (normalizedPathname) {
    case '/sources/stream':
      return 'stream_ingestion';
    case '/sources/epg':
      return 'epg_ingestion';
    case '/proxies':
      return 'proxy_regeneration';
    case '/events':
      return undefined; // Show all operation types
    default:
      return undefined;
  }
}

export function AppSidebar() {
  const pathname = usePathname();
  const operationType = getOperationTypeForPath(pathname);

  // Track collapsed groups; Debug collapsed by default
  const [collapsedGroups, setCollapsedGroups] = useState<Set<string>>(new Set(['Debug']));
  const toggleGroup = (title: string) => {
    setCollapsedGroups((prev) => {
      const next = new Set(prev);
      if (next.has(title)) {
        next.delete(title);
      } else {
        next.add(title);
      }
      return next;
    });
  };

  return (
    <Sidebar variant="inset" collapsible="icon">
      <SidebarHeader>
        <div className="flex items-center gap-2 px-2 py-2">
          <Radio className="h-4 w-4" />
          <div className="group-data-[collapsible=icon]:hidden">
            <h2 className="text-lg font-semibold">tvarr</h2>
          </div>
        </div>
      </SidebarHeader>

      <SidebarContent>
        {navigation.map((group) => {
          const isCollapsed = collapsedGroups.has(group.title);
          return (
            <SidebarGroup key={group.title}>
              <SidebarGroupLabel
                className="flex items-center justify-between cursor-pointer select-none"
                onClick={() => toggleGroup(group.title)}
              >
                <span className="flex items-center gap-1">
                  {isCollapsed ? (
                    <ChevronRight className="h-3 w-3" />
                  ) : (
                    <ChevronDown className="h-3 w-3" />
                  )}
                  {group.title}
                </span>
              </SidebarGroupLabel>
              {!isCollapsed && (
                <SidebarGroupContent>
                  <SidebarMenu>
                    {group.items.map((item) => (
                      <SidebarMenuItem key={item.title}>
                        <SidebarMenuButton
                          asChild
                          isActive={pathname === item.url}
                          tooltip={item.title}
                        >
                          <Link href={item.url}>
                            <item.icon />
                            <span>{item.title}</span>
                          </Link>
                        </SidebarMenuButton>
                      </SidebarMenuItem>
                    ))}
                  </SidebarMenu>
                </SidebarGroupContent>
              )}
            </SidebarGroup>
          );
        })}
      </SidebarContent>
    </Sidebar>
  );
}
