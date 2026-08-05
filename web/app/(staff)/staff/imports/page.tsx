'use client';

import { useState } from 'react';

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
import { api } from '@/lib/api';
import { RequireAdmin } from '@/lib/auth';
import { useT } from '@/lib/i18n';

interface Report {
  dryRun: boolean;
  doctorsNew: number;
  recipesNew: number;
  casesNew: number;
  skipped: Array<{ sheet: string; code: string; reason: string }>;
  errors: Array<{ sheet: string; ref: string; message: string }>;
  batchId?: number;
}

function Imports() {
  const t = useT();
  const [file, setFile] = useState<File | null>(null);
  const [report, setReport] = useState<Report | null>(null);
  const [batchId, setBatchId] = useState<number | null>(null);
  const [undone, setUndone] = useState(false);

  const run = async (dryRun: boolean) => {
    if (!file) {
      return;
    }
    const form = new FormData();
    form.append('file', file);
    const result = await api.upload<Report>(`/api/imports?dryRun=${dryRun}`, form);
    setReport(result);
    if (!dryRun && result.batchId) {
      setBatchId(result.batchId);
      setUndone(false);
    }
  };

  const undo = async () => {
    if (batchId == null) {
      return;
    }
    await api.send('POST', `/api/imports/${batchId}/undo`);
    setUndone(true);
    setBatchId(null);
  };

  return (
    <div className="space-y-6">
      <div>
        <Label htmlFor="import-file">{t('excelFile')}</Label>
        <Input
          id="import-file"
          type="file"
          accept=".xlsx"
          onChange={(e) => setFile(e.target.files?.[0] ?? null)}
        />
      </div>
      <div className="flex gap-3">
        <Button disabled={!file} onClick={() => run(true)}>
          {t('dryRun')}
        </Button>
        <Button disabled={!report} variant="secondary" onClick={() => run(false)}>
          {t('commitImport')}
        </Button>
      </div>

      {report && (
        <div className="space-y-2">
          <p>
            {t('doctorsNew')}: {report.doctorsNew} · {t('recipesNew')}: {report.recipesNew}{' '}
            · {t('casesNew')}: {report.casesNew}
          </p>
          {report.skipped.length > 0 && (
            <p>
              {t('skipped')}:{' '}
              {report.skipped.map((s) => `${s.sheet}/${s.code} (${s.reason})`).join(', ')}
            </p>
          )}
          {report.errors.length > 0 && (
            <p role="alert" className="text-destructive">
              {report.errors.map((e) => `${e.sheet}/${e.ref}: ${e.message}`).join(', ')}
            </p>
          )}
        </div>
      )}

      {batchId != null && (
        <AlertDialog>
          <AlertDialogTrigger asChild>
            <Button variant="destructive">{t('undoBatch')}</Button>
          </AlertDialogTrigger>
          <AlertDialogContent>
            <AlertDialogTitle>{t('confirmUndo')}</AlertDialogTitle>
            <AlertDialogFooter>
              <AlertDialogCancel>{t('cancel')}</AlertDialogCancel>
              <AlertDialogAction onClick={undo}>{t('confirm')}</AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      )}
      {undone && <p>{t('batchUndone')}</p>}
    </div>
  );
}

export default function ImportsPage() {
  return (
    <RequireAdmin>
      <Imports />
    </RequireAdmin>
  );
}
