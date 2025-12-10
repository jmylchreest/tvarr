'use client';

import { useBackendConnectivity } from '@/providers/backend-connectivity-provider';
import { BackendUnavailable } from '@/components/backend-unavailable';
import { AppSidebar } from '@/components/app-sidebar';
import { SidebarInset, SidebarTrigger, SidebarProvider } from '@/components/ui/sidebar';
import { Separator } from '@/components/ui/separator';
import { usePathname } from 'next/navigation';
import { NotificationBell } from '@/components/NotificationBell';
import { EnhancedThemeSelector } from '@/components/enhanced-theme-selector';
import { getPageTitle, getOperationType } from '@/lib/navigation';

interface AppLayoutProps {
  children: React.ReactNode;
}

export function AppLayout({ children }: AppLayoutProps) {
  const { isConnected, isChecking, hasInitialCheckCompleted, checkConnection, backendUrl } =
    useBackendConnectivity();
  const pathname = usePathname();
  const pageTitle = getPageTitle(pathname);
  const operationType = getOperationType(pathname);

  // Show loading state only during INITIAL check (not on navigation)
  if (!hasInitialCheckCompleted && isChecking) {
    return (
      <SidebarProvider>
        <AppSidebar />
        <SidebarInset>
          <main className="relative flex flex-1 flex-col bg-background">
            <header className="flex h-16 shrink-0 items-center gap-2 transition-[width,height] ease-linear group-has-data-[collapsible=icon]/sidebar-wrapper:h-12">
              <div className="flex items-center gap-2 px-4">
                <SidebarTrigger className="-ml-1" />
                <Separator orientation="vertical" className="mr-2 h-4" />
                <h1 className="text-2xl font-bold">Connecting...</h1>
              </div>
              <div className="flex items-center gap-2 ml-auto px-4">
                <EnhancedThemeSelector />
                <NotificationBell operationType={operationType} />
              </div>
            </header>
            <div className="flex flex-1 items-center justify-center">
              <div className="text-center space-y-4">
                <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary mx-auto"></div>
                <p className="text-muted-foreground">Connecting to backend...</p>
              </div>
            </div>
          </main>
        </SidebarInset>
      </SidebarProvider>
    );
  }

  // Show backend unavailable page only if we've checked and confirmed disconnection
  if (hasInitialCheckCompleted && !isConnected) {
    return (
      <BackendUnavailable
        onRetry={checkConnection}
        isRetrying={isChecking}
        backendUrl={backendUrl}
      />
    );
  }

  // Normal app layout when connected
  return (
    <SidebarProvider>
      <AppSidebar />
      <SidebarInset>
        <main className="relative flex flex-1 flex-col bg-background">
          <header className="flex h-16 shrink-0 items-center gap-2 transition-[width,height] ease-linear group-has-data-[collapsible=icon]/sidebar-wrapper:h-12">
            <div className="flex items-center gap-2 px-4">
              <SidebarTrigger className="-ml-1" />
              <Separator orientation="vertical" className="mr-2 h-4" />
              <h1 className="text-2xl font-bold">{pageTitle}</h1>
            </div>
            <div className="flex items-center gap-2 ml-auto px-4">
              <EnhancedThemeSelector />
              <NotificationBell operationType={operationType} />
            </div>
          </header>
          <div className="flex flex-1 flex-col gap-4 p-4 pt-0">{children}</div>
        </main>
      </SidebarInset>
    </SidebarProvider>
  );
}
