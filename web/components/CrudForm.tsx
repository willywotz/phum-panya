'use client';

import { type FormEvent, useState } from 'react';

import { api } from '@/lib/api';
import {
  type CrudRow,
  type FieldSpec,
  type ResourceSpec,
  rowValue,
} from '@/lib/crud';
import { useT } from '@/lib/i18n';

export interface CrudFormProps {
  spec: ResourceSpec;
  id?: number;
  initial?: CrudRow;
  onDone: (mismatch: boolean) => void;
  onCancel: () => void;
}

function initialValues(fields: FieldSpec[], initial?: CrudRow): CrudRow {
  const values: CrudRow = {};
  for (const field of fields) {
    const raw = initial ? rowValue(initial, field.name) : undefined;
    if (field.type === 'checkbox') {
      values[field.name] = Boolean(raw);
    } else if (field.type === 'multiselect') {
      values[field.name] = Array.isArray(raw) ? raw : [];
    } else {
      values[field.name] = raw ?? '';
    }
  }
  return values;
}

function toRequestBody(fields: FieldSpec[], values: CrudRow): CrudRow {
  const body: CrudRow = {};
  for (const field of fields) {
    body[field.name] =
      field.type === 'number' ? Number(values[field.name]) : values[field.name];
  }
  return body;
}

export function CrudForm({ spec, id, initial, onDone, onCancel }: CrudFormProps) {
  const t = useT();
  const [values, setValues] = useState<CrudRow>(() =>
    initialValues(spec.fields, initial),
  );
  const [submitting, setSubmitting] = useState(false);

  const setField = (name: string, value: unknown) => {
    setValues((prev) => ({ ...prev, [name]: value }));
  };

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setSubmitting(true);
    try {
      const body = toRequestBody(spec.fields, values);
      const response =
        id === undefined
          ? await api.send<CrudRow>('POST', spec.basePath, body)
          : await api.send<CrudRow>('PUT', `${spec.basePath}/${id}`, body);
      onDone(response?.mismatch === true);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <form onSubmit={handleSubmit}>
      {spec.fields.map((field) => (
        <div key={field.name}>
          <label htmlFor={field.name}>{t(field.labelKey)}</label>
          {renderInput(field, values, setField, t)}
        </div>
      ))}
      <button type="submit" disabled={submitting}>
        {t('save')}
      </button>
      <button type="button" onClick={onCancel}>
        {t('cancel')}
      </button>
    </form>
  );
}

function renderInput(
  field: FieldSpec,
  values: CrudRow,
  setField: (name: string, value: unknown) => void,
  t: (key: string) => string,
) {
  const { name, type, required, options } = field;

  switch (type) {
    case 'textarea':
      return (
        <textarea
          id={name}
          value={String(values[name] ?? '')}
          required={required}
          onChange={(event) => setField(name, event.target.value)}
        />
      );
    case 'checkbox':
      return (
        <input
          id={name}
          type="checkbox"
          checked={Boolean(values[name])}
          onChange={(event) => setField(name, event.target.checked)}
        />
      );
    case 'select':
      return (
        <select
          id={name}
          value={String(values[name] ?? '')}
          required={required}
          onChange={(event) => setField(name, event.target.value)}
        >
          <option value="" />
          {options?.map((option) => (
            <option key={option.value} value={option.value}>
              {t(option.labelKey)}
            </option>
          ))}
        </select>
      );
    case 'multiselect':
      return (
        <select
          id={name}
          multiple
          value={(values[name] as string[]) ?? []}
          required={required}
          onChange={(event) =>
            setField(
              name,
              Array.from(event.target.selectedOptions, (option) => option.value),
            )
          }
        >
          {options?.map((option) => (
            <option key={option.value} value={option.value}>
              {t(option.labelKey)}
            </option>
          ))}
        </select>
      );
    case 'number':
      return (
        <input
          id={name}
          type="number"
          value={String(values[name] ?? '')}
          required={required}
          onChange={(event) => setField(name, event.target.value)}
        />
      );
    default:
      return (
        <input
          id={name}
          type="text"
          value={String(values[name] ?? '')}
          required={required}
          onChange={(event) => setField(name, event.target.value)}
        />
      );
  }
}
