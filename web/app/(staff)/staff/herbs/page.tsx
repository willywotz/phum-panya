'use client';

import { type FormEvent, useCallback, useEffect, useState } from 'react';

import { CrudTable } from '@/components/CrudTable';
import { Button } from '@/components/ui/button';
import { Label } from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { api } from '@/lib/api';
import { type CrudRow, rowId, rowValue } from '@/lib/crud';
import { useT } from '@/lib/i18n';
import { herbSpec } from '@/lib/specs/herb';

// Ingredient rows with a typed herb name that has not been matched to the
// catalog yet. An admin reconciles each pending name to one catalog herb.
function ReconcilePanel() {
  const t = useT();
  const [pendingNames, setPendingNames] = useState<string[]>([]);
  const [herbs, setHerbs] = useState<CrudRow[]>([]);
  const [pendingName, setPendingName] = useState('');
  const [herbId, setHerbId] = useState('');

  const refresh = useCallback(async () => {
    const [pending, herbList] = await Promise.all([
      api.get<{ pending_names: string[] }>('/api/herbs/pending'),
      api.get<CrudRow[]>('/api/herbs'),
    ]);
    setPendingNames(pending.pending_names);
    setHerbs(herbList);
  }, []);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    await api.send('POST', '/api/herbs/reconcile', {
      pending_name: pendingName,
      herb_id: Number(herbId),
    });
    setPendingName('');
    setHerbId('');
    await refresh();
  };

  if (pendingNames.length === 0) {
    return null;
  }

  return (
    <section>
      <h2>{t('pendingHerbs')}</h2>
      <form onSubmit={handleSubmit}>
        <Label htmlFor="pending-name">{t('pendingHerbName')}</Label>
        <Select
          value={pendingName || undefined}
          required
          onValueChange={setPendingName}
        >
          <SelectTrigger id="pending-name" aria-label={t('pendingHerbName')}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {pendingNames.map((name) => (
              <SelectItem key={name} value={name}>
                {name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Label htmlFor="reconcile-herb">{t('reconcileToHerb')}</Label>
        <Select
          value={herbId || undefined}
          required
          onValueChange={setHerbId}
        >
          <SelectTrigger id="reconcile-herb" aria-label={t('reconcileToHerb')}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {herbs.map((herb) => (
              <SelectItem key={rowId(herb)} value={String(rowId(herb))}>
                {String(rowValue(herb, 'thai_name'))}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Button type="submit">{t('reconcile')}</Button>
      </form>
    </section>
  );
}

export default function HerbsPage() {
  return (
    <>
      <CrudTable spec={herbSpec} />
      <ReconcilePanel />
    </>
  );
}
