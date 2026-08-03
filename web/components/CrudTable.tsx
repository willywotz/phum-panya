'use client';

import { useCallback, useEffect, useRef, useState } from 'react';

import { CrudForm } from '@/components/CrudForm';
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
  const [confirmingId, setConfirmingId] = useState<number | null>(null);
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
    setConfirmingId(null);
    await refresh();
  };

  return (
    <section>
      <h2>{t(spec.titleKey)}</h2>
      {mismatch && <p role="alert">{t('mismatchWarning')}</p>}
      <button
        type="button"
        onClick={(event) => openForm('new', event.currentTarget)}
      >
        {t('add')}
      </button>
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
      <table>
        <thead>
          <tr>
            {columns.map((name) => (
              <th key={name}>{t(labelKeyFor(spec.fields, name))}</th>
            ))}
            <th>{t('actions')}</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => {
            const id = rowId(row);
            return (
              <tr key={id}>
                {columns.map((name) => (
                  <td key={name}>{String(rowValue(row, name) ?? '')}</td>
                ))}
                <td>
                  <button
                    type="button"
                    onClick={(event) => openForm(row, event.currentTarget)}
                  >
                    {t('edit')}
                  </button>
                  {confirmingId === id ? (
                    <span>
                      {t('confirmDelete')}
                      <button type="button" onClick={() => handleDelete(row)}>
                        {t('yes')}
                      </button>
                      <button type="button" onClick={() => setConfirmingId(null)}>
                        {t('no')}
                      </button>
                    </span>
                  ) : (
                    <button type="button" onClick={() => setConfirmingId(id)}>
                      {t('delete')}
                    </button>
                  )}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </section>
  );
}
