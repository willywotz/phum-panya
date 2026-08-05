'use client';

import { type FormEvent, useCallback, useEffect, useState } from 'react';

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
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { ApiError, api } from '@/lib/api';
import { RequireAdmin } from '@/lib/auth';
import { rowValue, type CrudRow } from '@/lib/crud';
import { useT } from '@/lib/i18n';

function YearLocks() {
  const t = useT();
  const [locks, setLocks] = useState<CrudRow[]>([]);
  const [year, setYear] = useState('');
  const [error, setError] = useState('');

  const refresh = useCallback(async () => {
    setLocks(await api.get<CrudRow[]>('/api/year-locks'));
  }, []);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const lock = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setError('');
    try {
      await api.send('POST', '/api/year-locks', { dataYear: Number(year) });
      setYear('');
      await refresh();
    } catch (err) {
      if (err instanceof ApiError) {
        const body = err.body as { error?: { message?: string } };
        setError(body.error?.message ?? t('lockFailed'));
      }
    }
  };

  const unlock = async (dataYear: number) => {
    await api.send('DELETE', `/api/year-locks/${dataYear}`);
    await refresh();
  };

  return (
    <>
      <form onSubmit={lock} className="mb-6 flex items-end gap-3">
        <div>
          <Label htmlFor="data-year">{t('dataYear')}</Label>
          <Input
            id="data-year"
            type="number"
            required
            value={year}
            onChange={(e) => setYear(e.target.value)}
          />
        </div>
        <Button type="submit">{t('lockYear')}</Button>
      </form>
      {error && <p role="alert" className="mb-4 text-destructive">{error}</p>}
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('dataYear')}</TableHead>
            <TableHead>{t('actions')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {locks.map((lockRow) => {
            const dataYear = Number(rowValue(lockRow, 'data_year'));
            return (
              <TableRow key={dataYear} aria-label={String(dataYear)}>
                <TableCell>{dataYear}</TableCell>
                <TableCell>
                  <AlertDialog>
                    <AlertDialogTrigger asChild>
                      <Button size="sm" variant="outline">
                        {t('unlock')}
                      </Button>
                    </AlertDialogTrigger>
                    <AlertDialogContent>
                      <AlertDialogTitle>{t('confirmUnlock')}</AlertDialogTitle>
                      <AlertDialogFooter>
                        <AlertDialogCancel>{t('cancel')}</AlertDialogCancel>
                        <AlertDialogAction onClick={() => unlock(dataYear)}>
                          {t('confirm')}
                        </AlertDialogAction>
                      </AlertDialogFooter>
                    </AlertDialogContent>
                  </AlertDialog>
                </TableCell>
              </TableRow>
            );
          })}
        </TableBody>
      </Table>
    </>
  );
}

export default function YearLocksPage() {
  return (
    <RequireAdmin>
      <YearLocks />
    </RequireAdmin>
  );
}
