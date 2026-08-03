'use client';

import { type FormEvent, useCallback, useEffect, useState } from 'react';

import { CrudTable } from '@/components/CrudTable';
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
        <label htmlFor="pending-name">{t('pendingHerbName')}</label>
        <select
          id="pending-name"
          value={pendingName}
          required
          onChange={(event) => setPendingName(event.target.value)}
        >
          <option value="" />
          {pendingNames.map((name) => (
            <option key={name} value={name}>
              {name}
            </option>
          ))}
        </select>
        <label htmlFor="reconcile-herb">{t('reconcileToHerb')}</label>
        <select
          id="reconcile-herb"
          value={herbId}
          required
          onChange={(event) => setHerbId(event.target.value)}
        >
          <option value="" />
          {herbs.map((herb) => (
            <option key={rowId(herb)} value={rowId(herb)}>
              {String(rowValue(herb, 'thai_name'))}
            </option>
          ))}
        </select>
        <button type="submit">{t('reconcile')}</button>
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
