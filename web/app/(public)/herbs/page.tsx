'use client';

import { useEffect, useState } from 'react';

import { api } from '@/lib/api';
import { useT } from '@/lib/i18n';

interface PublicHerb {
  id: number;
  thai_name: string;
  local_name: string;
  scientific_name: string;
  part_used: string;
  properties: string;
}

export default function HerbsPage() {
  const t = useT();
  const [herbs, setHerbs] = useState<PublicHerb[]>([]);

  useEffect(() => {
    api.get<PublicHerb[]>('/api/public/herbs').then(setHerbs);
  }, []);

  return (
    <section>
      <h1>{t('herbs')}</h1>
      <ul>
        {herbs.map((herb) => (
          <li key={herb.id}>
            <strong>{herb.thai_name}</strong>
            {herb.local_name ? ` (${herb.local_name})` : ''}
            {herb.scientific_name ? ` — ${herb.scientific_name}` : ''}
            {herb.part_used && (
              <p>
                {t('partUsed')}: {herb.part_used}
              </p>
            )}
            {herb.properties && (
              <p>
                {t('properties')}: {herb.properties}
              </p>
            )}
          </li>
        ))}
      </ul>
    </section>
  );
}
