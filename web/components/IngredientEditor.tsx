'use client';

import { type CrudRow, rowId, rowValue } from '@/lib/crud';
import { useT } from '@/lib/i18n';

// A pending-herb row picks this sentinel instead of a catalog herb id.
const PENDING = '__pending__';

export interface IngredientRow {
  herbSelection: string; // catalog herb id, PENDING, or '' (unset)
  pendingName: string;
  amount: string;
  unit: string;
  note: string;
}

export function emptyIngredientRow(): IngredientRow {
  return { herbSelection: '', pendingName: '', amount: '', unit: '', note: '' };
}

// Reads an existing ingredient (from a recipe's GET response) into row state.
export function toIngredientRow(ingredient: CrudRow): IngredientRow {
  const herbId = rowValue(ingredient, 'herb_id');
  const pendingName = rowValue(ingredient, 'pending_herb_name');
  return {
    herbSelection: herbId != null ? String(herbId) : PENDING,
    pendingName: typeof pendingName === 'string' ? pendingName : '',
    amount: String(rowValue(ingredient, 'amount') ?? ''),
    unit: String(rowValue(ingredient, 'unit') ?? ''),
    note: String(rowValue(ingredient, 'note') ?? ''),
  };
}

// Builds the ingredients array for POST/PUT /api/recipes: exactly one of
// herb_id / pending_herb_name per row.
export function toIngredientPayload(rows: IngredientRow[]) {
  return rows.map((row) => ({
    herb_id: row.herbSelection && row.herbSelection !== PENDING ? Number(row.herbSelection) : undefined,
    pending_herb_name: row.herbSelection === PENDING ? row.pendingName : undefined,
    amount: row.amount,
    unit: row.unit,
    note: row.note,
  }));
}

export interface IngredientEditorProps {
  rows: IngredientRow[];
  herbs: CrudRow[];
  onChange: (rows: IngredientRow[]) => void;
}

export function IngredientEditor({ rows, herbs, onChange }: IngredientEditorProps) {
  const t = useT();

  const updateRow = (index: number, patch: Partial<IngredientRow>) => {
    onChange(rows.map((row, i) => (i === index ? { ...row, ...patch } : row)));
  };

  return (
    <div>
      {rows.map((row, index) => (
        <fieldset key={index}>
          <legend>{`${t('ingredient')} ${index + 1}`}</legend>
          <label htmlFor={`ingredient-${index}-herb`}>{t('herb')}</label>
          <select
            id={`ingredient-${index}-herb`}
            value={row.herbSelection}
            required
            onChange={(event) => updateRow(index, { herbSelection: event.target.value })}
          >
            <option value="" />
            <option value={PENDING}>{t('otherHerb')}</option>
            {herbs.map((herb) => (
              <option key={rowId(herb)} value={rowId(herb)}>
                {String(rowValue(herb, 'thai_name'))}
              </option>
            ))}
          </select>
          {row.herbSelection === PENDING && (
            <>
              <label htmlFor={`ingredient-${index}-pending-name`}>{t('herbName')}</label>
              <input
                id={`ingredient-${index}-pending-name`}
                type="text"
                required
                value={row.pendingName}
                onChange={(event) => updateRow(index, { pendingName: event.target.value })}
              />
            </>
          )}
          <label htmlFor={`ingredient-${index}-amount`}>{t('amount')}</label>
          <input
            id={`ingredient-${index}-amount`}
            type="text"
            value={row.amount}
            onChange={(event) => updateRow(index, { amount: event.target.value })}
          />
          <label htmlFor={`ingredient-${index}-unit`}>{t('unit')}</label>
          <input
            id={`ingredient-${index}-unit`}
            type="text"
            value={row.unit}
            onChange={(event) => updateRow(index, { unit: event.target.value })}
          />
          <label htmlFor={`ingredient-${index}-note`}>{t('note')}</label>
          <input
            id={`ingredient-${index}-note`}
            type="text"
            value={row.note}
            onChange={(event) => updateRow(index, { note: event.target.value })}
          />
          <button type="button" onClick={() => onChange(rows.filter((_, i) => i !== index))}>
            {t('removeIngredient')}
          </button>
        </fieldset>
      ))}
      <button type="button" onClick={() => onChange([...rows, emptyIngredientRow()])}>
        {t('addIngredient')}
      </button>
    </div>
  );
}
