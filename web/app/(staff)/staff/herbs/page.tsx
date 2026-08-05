'use client';

import { type FormEvent, useCallback, useEffect, useState } from 'react';

import { CrudTable } from '@/components/CrudTable';
import { HerbAddForm } from '@/components/HerbAddForm';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogFooter,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '@/components/ui/alert-dialog';
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
import { useMe } from '@/lib/auth';
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

function MergePanel({ reloadKey }: { reloadKey: number }) {
  const t = useT();
  const [herbs, setHerbs] = useState<CrudRow[]>([]);
  const [sourceId, setSourceId] = useState('');
  const [canonicalId, setCanonicalId] = useState('');
  const [done, setDone] = useState(false);

  const refresh = useCallback(async () => {
    setHerbs(await api.get<CrudRow[]>('/api/herbs'));
  }, []);

  useEffect(() => {
    refresh();
  }, [refresh, reloadKey]);

  const merge = async () => {
    await api.send('POST', `/api/herbs/${sourceId}/merge/${canonicalId}`);
    setSourceId('');
    setCanonicalId('');
    setDone(true);
    await refresh();
  };

  return (
    <section className="mt-6">
      <h2>{t('mergeHerbs')}</h2>
      <div className="flex flex-wrap items-end gap-3">
        <div>
          <Label htmlFor="merge-source">{t('sourceHerb')}</Label>
          <Select value={sourceId || undefined} onValueChange={setSourceId}>
            <SelectTrigger id="merge-source" aria-label={t('sourceHerb')}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {herbs.map((h) => (
                <SelectItem key={rowId(h)} value={String(rowId(h))}>
                  {String(rowValue(h, 'thai_name'))}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div>
          <Label htmlFor="merge-canonical">{t('canonicalHerb')}</Label>
          <Select value={canonicalId || undefined} onValueChange={setCanonicalId}>
            <SelectTrigger id="merge-canonical" aria-label={t('canonicalHerb')}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {herbs.map((h) => (
                <SelectItem key={rowId(h)} value={String(rowId(h))}>
                  {String(rowValue(h, 'thai_name'))}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <AlertDialog>
          <AlertDialogTrigger asChild>
            <Button disabled={!sourceId || !canonicalId || sourceId === canonicalId}>
              {t('mergeHerbs')}
            </Button>
          </AlertDialogTrigger>
          <AlertDialogContent>
            <AlertDialogTitle>{t('confirmMerge')}</AlertDialogTitle>
            <AlertDialogFooter>
              <AlertDialogCancel>{t('cancel')}</AlertDialogCancel>
              <AlertDialogAction onClick={merge}>{t('confirm')}</AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      </div>
      {done && <p role="status">{t('merged')}</p>}
    </section>
  );
}

export default function HerbsPage() {
  const { me } = useMe();
  const [reloadKey, setReloadKey] = useState(0);
  const bump = () => setReloadKey((k) => k + 1);
  return (
    <>
      <HerbAddForm onCreated={bump} />
      <CrudTable key={reloadKey} spec={herbSpec} />
      {me?.role === 'central_admin' && <MergePanel reloadKey={reloadKey} />}
      <ReconcilePanel />
    </>
  );
}
