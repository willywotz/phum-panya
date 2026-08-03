'use client';

import { useRouter } from 'next/navigation';

import { api } from '@/lib/api';
import { useMe } from '@/lib/auth';
import { useT } from '@/lib/i18n';

export default function StaffPage() {
  const router = useRouter();
  const t = useT();
  const { me } = useMe();

  const handleLogout = async () => {
    await api.send('POST', '/api/logout');
    router.replace('/login');
  };

  return (
    <main>
      <h1>{t('staffDashboard')}</h1>
      {me && <p>{me.role}</p>}
      <button type="button" onClick={handleLogout}>
        {t('logOut')}
      </button>
    </main>
  );
}
