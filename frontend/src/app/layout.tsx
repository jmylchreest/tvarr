import type { Metadata } from 'next';
import './globals.css';
import { EnhancedThemeProvider } from '@/components/enhanced-theme-provider';
import { BackendConnectivityProvider } from '@/providers/backend-connectivity-provider';
import { ProgressProvider } from '@/providers/ProgressProvider';
import { FeatureFlagsProvider } from '@/providers/FeatureFlagsProvider';
import { PageLoadingProvider } from '@/providers/PageLoadingProvider';
import { AppLayout } from '@/components/app-layout';
import { enhancedThemeScript } from '@/lib/enhanced-theme-script';

export const metadata: Metadata = {
  title: 'tvarr',
  description: 'Modern web interface for tvarr IPTV management',
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" suppressHydrationWarning style={{ backgroundColor: 'var(--background)' }}>
      <head>
        {/* Theme CSS must load first to prevent flash of unstyled content */}
        {/* Use static path for initial load (works without backend) */}
        <link id="theme-css" rel="stylesheet" href="/themes/graphite.css" />
        {/* Theme script runs synchronously to apply dark mode class before content renders */}
        <script dangerouslySetInnerHTML={{ __html: enhancedThemeScript }} />
      </head>
      <body className="bg-background text-foreground">
        <EnhancedThemeProvider defaultTheme="graphite" defaultMode="system">
          <FeatureFlagsProvider>
            <BackendConnectivityProvider>
              <ProgressProvider>
                <PageLoadingProvider>
                  <AppLayout>{children}</AppLayout>
                </PageLoadingProvider>
              </ProgressProvider>
            </BackendConnectivityProvider>
          </FeatureFlagsProvider>
        </EnhancedThemeProvider>
      </body>
    </html>
  );
}
