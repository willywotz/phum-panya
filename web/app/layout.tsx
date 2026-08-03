import type { Metadata } from 'next';

import '@fontsource/noto-sans-thai/400.css';
import '@fontsource/noto-sans-thai/500.css';
import '@fontsource/noto-sans-thai/700.css';
import './globals.css';
import '../print.css';
import { I18nProvider } from '@/lib/i18n';

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
        <I18nProvider>{children}</I18nProvider>
      </body>
    </html>
  );
}
