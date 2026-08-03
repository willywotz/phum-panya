'use client';

import { type FormEvent, useEffect, useRef, useState } from 'react';

import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Textarea } from '@/components/ui/textarea';
import { api } from '@/lib/api';
import {
  type CrudRow,
  type FieldOption,
  type FieldSpec,
  type ResourceSpec,
  resourceUrl,
  rowValue,
} from '@/lib/crud';
import { useT } from '@/lib/i18n';

export interface CrudFormProps {
  spec: ResourceSpec;
  id?: number;
  initial?: CrudRow;
  onDone: (mismatch: boolean) => void;
  onCancel: () => void;
  // Extra content (e.g. a photo upload widget) shown below the fields, only
  // once a row exists to attach it to.
  formExtra?: (id: number) => React.ReactNode;
}

// Fields hidden once a row already exists (e.g. a create-only password).
function visibleFields(fields: FieldSpec[], editing: boolean): FieldSpec[] {
  return fields.filter((field) => !(field.createOnly && editing));
}

function toDateInputValue(raw: unknown): string {
  return typeof raw === 'string' ? raw.slice(0, 10) : '';
}

function initialValues(fields: FieldSpec[], initial?: CrudRow): CrudRow {
  const values: CrudRow = {};
  for (const field of fields) {
    const raw = initial ? rowValue(initial, field.name) : undefined;
    if (field.type === 'checkbox') {
      values[field.name] = Boolean(raw);
    } else if (field.type === 'multiselect') {
      values[field.name] = Array.isArray(raw) ? raw : [];
    } else if (field.type === 'date') {
      values[field.name] = toDateInputValue(raw);
    } else {
      values[field.name] = raw ?? '';
    }
  }
  return values;
}

// Builds the JSON request body: numbers and numeric-tagged selects become
// Number (blank optional ones are omitted), and date inputs (YYYY-MM-DD)
// become full timestamps or null.
function toRequestBody(fields: FieldSpec[], values: CrudRow): CrudRow {
  const body: CrudRow = {};
  for (const field of fields) {
    const raw = values[field.name];
    if (field.type === 'number' || field.numeric) {
      body[field.name] = raw === '' ? undefined : Number(raw);
    } else if (field.type === 'date') {
      body[field.name] = raw ? new Date(String(raw)).toISOString() : null;
    } else {
      body[field.name] = raw;
    }
  }
  return body;
}

export function CrudForm({ spec, id, initial, onDone, onCancel, formExtra }: CrudFormProps) {
  const t = useT();
  const fields = visibleFields(spec.fields, id !== undefined);
  const [values, setValues] = useState<CrudRow>(() => initialValues(fields, initial));
  const [submitting, setSubmitting] = useState(false);
  const firstFieldRef = useRef<HTMLElement | null>(null);

  // Move focus into the form as soon as it opens, so keyboard/screen-reader
  // users land on the first field instead of staying on the trigger button.
  useEffect(() => {
    firstFieldRef.current?.focus();
  }, []);

  const setField = (name: string, value: unknown) => {
    setValues((prev) => ({ ...prev, [name]: value }));
  };

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setSubmitting(true);
    try {
      const body = toRequestBody(fields, values);
      const response =
        id === undefined
          ? await api.send<CrudRow>('POST', spec.basePath, body)
          : await api.send<CrudRow>('PUT', resourceUrl(spec.basePath, id), body);
      onDone(response?.mismatch === true);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <form onSubmit={handleSubmit}>
      {fields.map((field, index) => (
        <div key={field.name} className="grid gap-1.5">
          <Label htmlFor={field.name}>{t(field.labelKey)}</Label>
          {renderInput(
            field,
            values,
            setField,
            t,
            index === 0 && autoFocusable(field.type)
              ? (el) => (firstFieldRef.current = el)
              : undefined,
          )}
        </div>
      ))}
      {id !== undefined && formExtra?.(id)}
      <Button type="submit" disabled={submitting}>
        {t('save')}
      </Button>
      <Button type="button" variant="outline" onClick={onCancel}>
        {t('cancel')}
      </Button>
    </form>
  );
}

// Radix SelectTrigger and shadcn Checkbox don't accept the HTMLElement focus
// ref used to autofocus the form's first field, so only text-like inputs
// (including native multiselect) are eligible.
function autoFocusable(type: FieldSpec['type']): boolean {
  return type !== 'select' && type !== 'checkbox';
}

function optionLabel(option: FieldOption, t: (key: string) => string): string {
  return option.label ?? (option.labelKey ? t(option.labelKey) : option.value);
}

function renderInput(
  field: FieldSpec,
  values: CrudRow,
  setField: (name: string, value: unknown) => void,
  t: (key: string) => string,
  autoFocusRef?: (el: HTMLElement | null) => void,
) {
  const { name, type, required, options } = field;

  switch (type) {
    case 'textarea':
      return (
        <Textarea
          id={name}
          ref={autoFocusRef}
          value={String(values[name] ?? '')}
          required={required}
          onChange={(event) => setField(name, event.target.value)}
        />
      );
    case 'checkbox':
      return (
        <Checkbox
          id={name}
          checked={Boolean(values[name])}
          onCheckedChange={(checked) => setField(name, checked === true)}
        />
      );
    case 'select':
      return (
        <Select
          value={String(values[name] ?? '')}
          required={required}
          onValueChange={(value) => setField(name, value)}
        >
          <SelectTrigger id={name} aria-label={t(field.labelKey)}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {options?.map((option) => (
              <SelectItem key={option.value} value={option.value}>
                {optionLabel(option, t)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      );
    case 'multiselect':
      return (
        <select
          id={name}
          multiple
          ref={autoFocusRef}
          className="w-full rounded-md border bg-background p-2"
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
              {optionLabel(option, t)}
            </option>
          ))}
        </select>
      );
    case 'date':
    case 'password':
    case 'number':
      return (
        <Input
          id={name}
          type={type}
          ref={autoFocusRef}
          value={String(values[name] ?? '')}
          required={required}
          onChange={(event) => setField(name, event.target.value)}
        />
      );
    default:
      return (
        <Input
          id={name}
          type="text"
          ref={autoFocusRef}
          value={String(values[name] ?? '')}
          required={required}
          onChange={(event) => setField(name, event.target.value)}
        />
      );
  }
}
