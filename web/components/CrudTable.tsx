'use client';

import { useCallback, useEffect, useState } from 'react';

import { CrudForm } from '@/components/CrudForm';
import { api } from '@/lib/api';
import {
  type CrudRow,
  type FieldSpec,
  type ResourceSpec,
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

export function CrudTable({ spec }: { spec: ResourceSpec }) {
  const t = useT();
  const [rows, setRows] = useState<CrudRow[]>([]);
  const [editing, setEditing] = useState<CrudRow | 'new' | null>(null);
  const [mismatch, setMismatch] = useState(false);

  const columns = spec.listColumns ?? defaultColumns(spec.fields);

  const refresh = useCallback(async () => {
    setRows(await api.get<CrudRow[]>(spec.basePath));
  }, [spec.basePath]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const handleDone = async (didMismatch: boolean) => {
    setEditing(null);
    setMismatch(didMismatch);
    await refresh();
  };

  const handleDelete = async (row: CrudRow) => {
    await api.send('DELETE', `${spec.basePath}/${rowId(row)}`);
    await refresh();
  };

  return (
    <section>
      <h2>{t(spec.titleKey)}</h2>
      {mismatch && <p role="alert">{t('mismatchWarning')}</p>}
      <button type="button" onClick={() => setEditing('new')}>
        {t('add')}
      </button>
      {editing !== null && (
        <CrudForm
          spec={spec}
          id={editing === 'new' ? undefined : rowId(editing)}
          initial={editing === 'new' ? undefined : editing}
          onDone={handleDone}
          onCancel={() => setEditing(null)}
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
          {rows.map((row) => (
            <tr key={rowId(row)}>
              {columns.map((name) => (
                <td key={name}>{String(rowValue(row, name) ?? '')}</td>
              ))}
              <td>
                <button type="button" onClick={() => setEditing(row)}>
                  {t('edit')}
                </button>
                <button type="button" onClick={() => handleDelete(row)}>
                  {t('delete')}
                </button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  );
}
