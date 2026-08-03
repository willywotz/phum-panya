'use client';

import { useRouter } from 'next/navigation';
import { useEffect, useState } from 'react';

import { api } from '@/lib/api';
import { useMe } from '@/lib/auth';
import { useT } from '@/lib/i18n';

// The backend enforces a soft ceiling on media storage; the meter shows
// usage toward it for planning purposes only, not a hard limit.
const STORAGE_CEILING_BYTES = 30 * 1024 * 1024 * 1024;

function StorageMeter() {
  const t = useT();
  const [usedBytes, setUsedBytes] = useState<number | null>(null);

  useEffect(() => {
    api.get<{ used_bytes: number }>('/api/storage').then((res) => setUsedBytes(res.used_bytes));
  }, []);

  if (usedBytes === null) {
    return null;
  }
  const usedMb = usedBytes / (1024 * 1024);

  return (
    <section aria-label={t('storage')}>
      <h2>{t('storage')}</h2>
      <p>
        {t('storageUsed')}: {usedMb.toFixed(1)} MB
      </p>
      <progress value={usedBytes} max={STORAGE_CEILING_BYTES} />
    </section>
  );
}

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
      {me?.role === 'central_admin' && <StorageMeter />}
      <button type="button" onClick={handleLogout}>
        {t('logOut')}
      </button>
    </main>
  );
}
