import type { Metadata } from 'next';

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
    <html lang="th">
      <body id="app">
        <I18nProvider>{children}</I18nProvider>
      </body>
    </html>
  );
}
