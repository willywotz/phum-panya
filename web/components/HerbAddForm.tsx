'use client';

import { type FormEvent, useEffect, useState } from 'react';

import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { api } from '@/lib/api';
import { rowValue, type CrudRow } from '@/lib/crud';
import { useT } from '@/lib/i18n';

export function HerbAddForm({ onCreated }: { onCreated: () => void }) {
  const t = useT();
  const [thaiName, setThaiName] = useState('');
  const [dups, setDups] = useState<CrudRow[]>([]);

  useEffect(() => {
    if (thaiName.trim().length < 2) {
      setDups([]);
      return;
    }
    let cancelled = false;
    const handle = setTimeout(async () => {
      const found = await api.get<CrudRow[]>(
        `/api/herbs/near-duplicates?thaiName=${encodeURIComponent(thaiName)}`,
      );
      if (!cancelled) {
        setDups(found);
      }
    }, 300);
    return () => {
      cancelled = true;
      clearTimeout(handle);
    };
  }, [thaiName]);

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    await api.send('POST', '/api/herbs', { thai_name: thaiName });
    setThaiName('');
    setDups([]);
    onCreated();
  };

  return (
    <form onSubmit={submit} className="mb-4 space-y-2">
      <Label htmlFor="herb-add-name">{t('thaiNameAdd')}</Label>
      <Input
        id="herb-add-name"
        required
        value={thaiName}
        onChange={(e) => setThaiName(e.target.value)}
      />
      {dups.length > 0 && (
        <p role="status" className="text-sm text-muted-foreground">
          {t('mayDuplicate')}:{' '}
          {dups.map((d) => String(rowValue(d, 'thai_name'))).join(', ')}
        </p>
      )}
      <Button type="submit">{t('saveHerb')}</Button>
    </form>
  );
}
