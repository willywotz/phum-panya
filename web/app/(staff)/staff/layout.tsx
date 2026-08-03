'use client';

import Link from 'next/link';

import { RequireStaff } from '@/lib/auth';
import { LangToggle, useT } from '@/lib/i18n';
import { ThemeToggle } from '@/lib/theme';

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
    <nav
      className="flex items-center gap-4 border-b bg-card px-4 py-3"
      aria-label={t('staffNav')}
    >
      <div className="mr-auto flex items-center gap-4">
        {navLinks.map(({ href, key }) => (
          <Link key={href} href={href} className="text-sm font-medium hover:text-primary">
            {t(key)}
          </Link>
        ))}
      </div>
      <LangToggle />
      <ThemeToggle />
    </nav>
  );
}

export default function StaffLayout({ children }: { children: React.ReactNode }) {
  return (
    <RequireStaff>
      <StaffNav />
      <div className="mx-auto max-w-5xl px-4 py-6">{children}</div>
    </RequireStaff>
  );
}
