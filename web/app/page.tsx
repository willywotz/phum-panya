'use client';

import { useRouter } from 'next/navigation';

import { LangToggle, useT } from '@/lib/i18n';

export default function HomePage() {
  const router = useRouter();
  const t = useT();

  return (
    <main>
      <LangToggle />
      <button type="button" onClick={() => router.push('/login')}>
        {t('signIn')}
      </button>
    </main>
  );
}
