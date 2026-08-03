'use client';

import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
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
    <div className="grid gap-4">
      {rows.map((row, index) => (
        <fieldset key={index} className="grid gap-3 rounded-lg border p-3">
          <legend className="px-1 text-sm font-medium">{`${t('ingredient')} ${index + 1}`}</legend>
          <div className="grid gap-1.5">
            <Label htmlFor={`ingredient-${index}-herb`}>{t('herb')}</Label>
            <Select
              value={row.herbSelection}
              required
              onValueChange={(value) => updateRow(index, { herbSelection: value })}
            >
              <SelectTrigger id={`ingredient-${index}-herb`} aria-label={t('herb')}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={PENDING}>{t('otherHerb')}</SelectItem>
                {herbs.map((herb) => (
                  <SelectItem key={rowId(herb)} value={String(rowId(herb))}>
                    {String(rowValue(herb, 'thai_name'))}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          {row.herbSelection === PENDING && (
            <div className="grid gap-1.5">
              <Label htmlFor={`ingredient-${index}-pending-name`}>{t('herbName')}</Label>
              <Input
                id={`ingredient-${index}-pending-name`}
                type="text"
                required
                value={row.pendingName}
                onChange={(event) => updateRow(index, { pendingName: event.target.value })}
              />
            </div>
          )}
          <div className="grid gap-1.5">
            <Label htmlFor={`ingredient-${index}-amount`}>{t('amount')}</Label>
            <Input
              id={`ingredient-${index}-amount`}
              type="text"
              value={row.amount}
              onChange={(event) => updateRow(index, { amount: event.target.value })}
            />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor={`ingredient-${index}-unit`}>{t('unit')}</Label>
            <Input
              id={`ingredient-${index}-unit`}
              type="text"
              value={row.unit}
              onChange={(event) => updateRow(index, { unit: event.target.value })}
            />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor={`ingredient-${index}-note`}>{t('note')}</Label>
            <Input
              id={`ingredient-${index}-note`}
              type="text"
              value={row.note}
              onChange={(event) => updateRow(index, { note: event.target.value })}
            />
          </div>
          <Button
            type="button"
            variant="outline"
            onClick={() => onChange(rows.filter((_, i) => i !== index))}
          >
            {t('removeIngredient')}
          </Button>
        </fieldset>
      ))}
      <Button type="button" variant="outline" onClick={() => onChange([...rows, emptyIngredientRow()])}>
        {t('addIngredient')}
      </Button>
    </div>
  );
}
