'use client';

import { useCallback, useEffect, useRef, useState } from 'react';

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '@/components/ui/alert-dialog';
import { Button } from '@/components/ui/button';
import { CrudForm } from '@/components/CrudForm';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { api } from '@/lib/api';
import {
  type CrudRow,
  type FieldSpec,
  type ResourceSpec,
  resourceUrl,
  rowId,
  rowValue,
} from '@/lib/crud';
import { useT } from '@/lib/i18n';

function defaultColumns(fields: FieldSpec[]): string[] {
  return fields
    .filter((field) => field.type === 'text' || field.type === 'number')
    .map((field) => field.name);
}

function labelKeyFor(fields: FieldSpec[], name: string): string {
  return fields.find((field) => field.name === name)?.labelKey ?? name;
}

export interface CrudTableProps {
  spec: ResourceSpec;
  // Prefilled values for a new row (e.g. the currently selected district).
  newDefaults?: CrudRow;
  // Extra content (e.g. a photo upload widget) shown once a row exists.
  formExtra?: (id: number) => React.ReactNode;
}

export function CrudTable({ spec, newDefaults, formExtra }: CrudTableProps) {
  const t = useT();
  const [rows, setRows] = useState<CrudRow[]>([]);
  const [editing, setEditing] = useState<CrudRow | 'new' | null>(null);
  const [mismatch, setMismatch] = useState(false);
  // The button that opened the form (Add, or a row's Edit), so focus can
  // return to it once the form closes.
  const triggerRef = useRef<HTMLButtonElement | null>(null);

  const columns = spec.listColumns ?? defaultColumns(spec.fields);

  const refresh = useCallback(async () => {
    setRows(await api.get<CrudRow[]>(spec.basePath));
  }, [spec.basePath]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const openForm = (row: CrudRow | 'new', trigger: HTMLButtonElement) => {
    triggerRef.current = trigger;
    setEditing(row);
  };

  const closeForm = () => {
    setEditing(null);
    triggerRef.current?.focus();
  };

  const handleDone = async (didMismatch: boolean) => {
    closeForm();
    setMismatch(didMismatch);
    await refresh();
  };

  const handleDelete = async (row: CrudRow) => {
    await api.send('DELETE', resourceUrl(spec.basePath, rowId(row)));
    await refresh();
  };

  return (
    <section>
      <h2>{t(spec.titleKey)}</h2>
      {mismatch && <p role="alert">{t('mismatchWarning')}</p>}
      <Button
        type="button"
        onClick={(event) => openForm('new', event.currentTarget)}
      >
        {t('add')}
      </Button>
      <Dialog
        open={editing !== null}
        onOpenChange={(open) => {
          if (!open) closeForm();
        }}
      >
        <DialogContent className="max-h-[85vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>{t(editing === 'new' ? 'add' : 'edit')}</DialogTitle>
          </DialogHeader>
          {editing !== null && (
            <CrudForm
              spec={spec}
              id={editing === 'new' ? undefined : rowId(editing)}
              initial={editing === 'new' ? newDefaults : editing}
              onDone={handleDone}
              onCancel={closeForm}
              formExtra={formExtra}
            />
          )}
        </DialogContent>
      </Dialog>
      <Table>
        <TableHeader>
          <TableRow>
            {columns.map((name) => (
              <TableHead key={name}>{t(labelKeyFor(spec.fields, name))}</TableHead>
            ))}
            <TableHead>{t('actions')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.map((row) => {
            const id = rowId(row);
            return (
              <TableRow key={id}>
                {columns.map((name) => (
                  <TableCell key={name}>{String(rowValue(row, name) ?? '')}</TableCell>
                ))}
                <TableCell>
                  <Button
                    type="button"
                    variant="outline"
                    onClick={(event) => openForm(row, event.currentTarget)}
                  >
                    {t('edit')}
                  </Button>
                  <AlertDialog>
                    <AlertDialogTrigger asChild>
                      <Button type="button" variant="destructive">
                        {t('delete')}
                      </Button>
                    </AlertDialogTrigger>
                    <AlertDialogContent>
                      <AlertDialogTitle>{t('delete')}</AlertDialogTitle>
                      <AlertDialogDescription>
                        {t('confirmDelete')}
                      </AlertDialogDescription>
                      <AlertDialogFooter>
                        <AlertDialogCancel>{t('no')}</AlertDialogCancel>
                        <AlertDialogAction onClick={() => handleDelete(row)}>
                          {t('yes')}
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
    </section>
  );
}
