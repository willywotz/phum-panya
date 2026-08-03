'use client';

import Link from 'next/link';

import { RequireStaff } from '@/lib/auth';
import { LangToggle, useT } from '@/lib/i18n';

const navLinks = [
  { href: '/staff/districts', key: 'districts' },
  { href: '/staff/herbs', key: 'herbs' },
  { href: '/staff/users', key: 'users' },
  { href: '/staff/doctors', key: 'doctors' },
  { href: '/staff/recipes', key: 'recipes' },
  { href: '/staff/cases', key: 'cases' },
] as const;

function StaffNav() {
  const t = useT();
  return (
    <nav aria-label={t('staffNav')}>
      {navLinks.map(({ href, key }) => (
        <Link key={href} href={href}>
          {t(key)}
        </Link>
      ))}
      <LangToggle />
    </nav>
  );
}

export default function StaffLayout({ children }: { children: React.ReactNode }) {
  return (
    <RequireStaff>
      <StaffNav />
      {children}
    </RequireStaff>
  );
}
