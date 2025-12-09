'use client';

import { ChevronRight, ChevronDown } from 'lucide-react';
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
import { navigation, Radio } from '@/lib/navigation';

export function AppSidebar() {
  const pathname = usePathname();

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
