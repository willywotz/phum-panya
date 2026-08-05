'use client';

import { useCallback, useEffect, useState } from 'react';

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
import { Textarea } from '@/components/ui/textarea';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { RequireAdmin } from '@/lib/auth';
import { api } from '@/lib/api';
import { useT } from '@/lib/i18n';

interface QueueItem {
  entityType: string;
  entityId: number;
  action: string;
  reviewState: string;
  districtId: number;
}

interface Detail {
  entityType: string;
  entityId: number;
  action: string;
  identity: string;
  doctorId: number;
  current: unknown;
  proposed: unknown;
}

function changedFields(current: unknown, proposed: unknown): string[] {
  if (!proposed || typeof proposed !== 'object') {
    return [];
  }
  const cur = (current ?? {}) as Record<string, unknown>;
  return Object.entries(proposed as Record<string, unknown>)
    .filter(([key, value]) => JSON.stringify(cur[key]) !== JSON.stringify(value))
    .map(([key, value]) => `${key}: ${JSON.stringify(value)}`);
}

// RejectDialog owns its reason text so it cannot leak into another row's dialog.
function RejectDialog({ onReject }: { onReject: (reason: string) => void }) {
  const t = useT();
  const [reason, setReason] = useState('');

  const submit = () => {
    onReject(reason);
    setReason('');
  };

  return (
    <AlertDialog>
      <AlertDialogTrigger asChild>
        <Button size="sm" variant="outline">
          {t('reject')}
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogTitle>{t('rejectReason')}</AlertDialogTitle>
        <Textarea
          aria-label={t('rejectReason')}
          value={reason}
          onChange={(e) => setReason(e.target.value)}
        />
        <AlertDialogFooter>
          <AlertDialogCancel>{t('cancel')}</AlertDialogCancel>
          <AlertDialogAction disabled={!reason.trim()} onClick={submit}>
            {t('reject')}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

function ReviewQueue() {
  const t = useT();
  const [rows, setRows] = useState<Detail[]>([]);
  const [loading, setLoading] = useState(true);

  const refresh = useCallback(async () => {
    const items = await api.get<QueueItem[]>('/api/review/queue');
    const results = await Promise.allSettled(
      items.map((it) =>
        api.get<Detail>(`/api/review/entry/${it.entityType}/${it.entityId}`),
      ),
    );
    setRows(
      results
        .filter((r): r is PromiseFulfilledResult<Detail> => r.status === 'fulfilled')
        .map((r) => r.value),
    );
    setLoading(false);
  }, []);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const approve = async (d: Detail) => {
    await api.send('POST', `/api/review/entry/${d.entityType}/${d.entityId}/approve`);
    await refresh();
  };
  const approveTree = async (doctorId: number) => {
    await api.send('POST', `/api/review/doctor/${doctorId}/approve-all`);
    await refresh();
  };
  const reject = async (d: Detail, reason: string) => {
    await api.send('POST', `/api/review/entry/${d.entityType}/${d.entityId}/reject`, {
      reason,
    });
    await refresh();
  };

  if (loading) {
    return null;
  }

  if (rows.length === 0) {
    return <p>{t('queueEmpty')}</p>;
  }

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{t('entityType')}</TableHead>
          <TableHead>{t('name')}</TableHead>
          <TableHead>{t('action')}</TableHead>
          <TableHead>{t('proposedChange')}</TableHead>
          <TableHead>{t('actions')}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.map((d) => (
          <TableRow key={`${d.entityType}-${d.entityId}`} aria-label={d.identity}>
            <TableCell>{t(d.entityType)}</TableCell>
            <TableCell>{d.identity}</TableCell>
            <TableCell>{t(`action_${d.action}`)}</TableCell>
            <TableCell>
              {d.action === 'update'
                ? changedFields(d.current, d.proposed).join('; ')
                : t(`action_${d.action}`)}
            </TableCell>
            <TableCell className="flex gap-2">
              <Button size="sm" onClick={() => approve(d)}>
                {t('approve')}
              </Button>
              <RejectDialog onReject={(reason) => reject(d, reason)} />
              <Button size="sm" variant="ghost" onClick={() => approveTree(d.doctorId)}>
                {t('approveAll')}
              </Button>
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}

export default function ReviewPage() {
  return (
    <RequireAdmin>
      <ReviewQueue />
    </RequireAdmin>
  );
}
