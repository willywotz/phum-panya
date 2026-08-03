'use client';

import Link from 'next/link';
import { useRouter } from 'next/navigation';

import { Button } from '@/components/ui/button';
import { LangToggle, useT } from '@/lib/i18n';
import { ThemeToggle } from '@/lib/theme';

const navLinks = [
  { href: '/', key: 'home' },
  { href: '/doctors', key: 'doctors' },
  { href: '/recipes', key: 'recipes' },
  { href: '/herbs', key: 'herbs' },
] as const;

function PublicNav() {
  const t = useT();
  const router = useRouter();
  return (
    <nav
      className="no-print flex items-center gap-4 border-b bg-card px-4 py-3"
      aria-label={t('publicNav')}
    >
      <div className="mr-auto flex items-center gap-4">
        {navLinks.map(({ href, key }) => (
          <Link key={href} href={href} className="text-sm font-medium hover:text-primary">
            {t(key)}
          </Link>
        ))}
      </div>
      <Button type="button" variant="outline" size="sm" onClick={() => router.push('/login')}>
        {t('signIn')}
      </Button>
      <LangToggle />
      <ThemeToggle />
    </nav>
  );
}

export default function PublicLayout({ children }: { children: React.ReactNode }) {
  return (
    <>
      <PublicNav />
      <div className="mx-auto max-w-5xl px-4 py-6">{children}</div>
    </>
  );
}
