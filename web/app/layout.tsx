import type { Metadata } from 'next';

import '@fontsource/sarabun/300.css';
import '@fontsource/sarabun/500.css';
import '@fontsource/sarabun/700.css';
import './globals.css';
import '../print.css';
import { I18nProvider } from '@/lib/i18n';
import { ThemeProvider } from '@/lib/theme';

export const metadata: Metadata = {
  title: 'phum-panya',
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="th" suppressHydrationWarning>
      <body id="app" className="min-h-dvh bg-background text-foreground antialiased">
        <ThemeProvider>
          <I18nProvider>{children}</I18nProvider>
        </ThemeProvider>
      </body>
    </html>
  );
}
