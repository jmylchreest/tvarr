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
} from 'lucide-react';
import { usePathname } from 'next/navigation';
import Link from 'next/link';
import { cn } from '@/lib/utils';
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
        icon: Radio,
      },
      {
        title: 'EPG Sources',
        url: '/sources/epg',
        icon: Database,
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
        icon: Server,
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
        icon: Zap,
      },
      {
        title: 'Logs',
        url: '/logs',
        icon: FileText,
      },
      {
        title: 'Color Palette',
        url: '/color-palette',
        icon: Palette,
      },
    ],
  },
];

export function AppSidebar() {
  const pathname = usePathname();

  return (
    <Sidebar variant="inset" collapsible="icon">
      <SidebarHeader>
        <SidebarMenuButton size="lg" asChild>
          <Link href="/">
            <div className="flex aspect-square size-8 items-center justify-center rounded-lg bg-sidebar-primary text-sidebar-primary-foreground">
              <Radio className="size-4" />
            </div>
            <div className="grid flex-1 text-left text-sm leading-tight">
              <span className="truncate font-semibold">tvarr</span>
            </div>
          </Link>
        </SidebarMenuButton>
      </SidebarHeader>
      <SidebarContent>
        {navigation.map((section) => (
          <SidebarGroup key={section.title}>
            <SidebarGroupLabel>{section.title}</SidebarGroupLabel>
            <SidebarGroupContent>
              <SidebarMenu>
                {section.items.map((item) => (
                  <SidebarMenuItem key={item.title}>
                    <SidebarMenuButton asChild isActive={pathname === item.url}>
                      <a href={item.url}>
                        <item.icon />
                        <span>{item.title}</span>
                      </a>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                ))}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        ))}
      </SidebarContent>
    </Sidebar>
  );
}
