'use client';

import { useEffect, useState } from 'react';

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
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
      <h1 className="mb-4 text-2xl font-bold text-primary">{t('herbs')}</h1>
      <div className="grid gap-3">
        {herbs.map((herb) => (
          <Card key={herb.id}>
            <CardHeader>
              <CardTitle>
                {herb.thai_name}
                {herb.local_name ? ` (${herb.local_name})` : ''}
              </CardTitle>
            </CardHeader>
            <CardContent className="text-muted-foreground">
              {herb.scientific_name && <p className="italic">{herb.scientific_name}</p>}
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
            </CardContent>
          </Card>
        ))}
      </div>
    </section>
  );
}
