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
    <html lang="en" suppressHydrationWarning>
      <body>
        <script dangerouslySetInnerHTML={{ __html: enhancedThemeScript }} />
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
