'use client';

import Link from 'next/link';
import { useRouter } from 'next/navigation';

import { LangToggle, useT } from '@/lib/i18n';

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
    <nav className="no-print" aria-label={t('publicNav')}>
      {navLinks.map(({ href, key }) => (
        <Link key={href} href={href}>
          {t(key)}
        </Link>
      ))}
      <button type="button" onClick={() => router.push('/login')}>
        {t('signIn')}
      </button>
      <LangToggle />
    </nav>
  );
}

export default function PublicLayout({ children }: { children: React.ReactNode }) {
  return (
    <>
      <PublicNav />
      {children}
    </>
  );
}
