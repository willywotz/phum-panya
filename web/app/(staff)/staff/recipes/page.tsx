'use client';

import { type FormEvent, useCallback, useEffect, useState } from 'react';

import { ExportLinks } from '@/components/ExportLinks';
import {
  IngredientEditor,
  type IngredientRow,
  emptyIngredientRow,
  toIngredientPayload,
  toIngredientRow,
} from '@/components/IngredientEditor';
import { api } from '@/lib/api';
import { useMe } from '@/lib/auth';
import { type CrudRow, rowId, rowValue } from '@/lib/crud';
import { useT } from '@/lib/i18n';

interface RecipeFormValues {
  code: string;
  name: string;
  indication: string;
  preparation: string;
  usage: string;
  caution: string;
  care_stage: string;
  data_year: string;
}

function emptyRecipeValues(): RecipeFormValues {
  return {
    code: '',
    name: '',
    indication: '',
    preparation: '',
    usage: '',
    caution: '',
    care_stage: '',
    data_year: '',
  };
}

function recipeValuesFrom(row: CrudRow): RecipeFormValues {
  return {
    code: String(rowValue(row, 'code') ?? ''),
    name: String(rowValue(row, 'name') ?? ''),
    indication: String(rowValue(row, 'indication') ?? ''),
    preparation: String(rowValue(row, 'preparation') ?? ''),
    usage: String(rowValue(row, 'usage') ?? ''),
    caution: String(rowValue(row, 'caution') ?? ''),
    care_stage: String(rowValue(row, 'care_stage') ?? ''),
    data_year: String(rowValue(row, 'data_year') ?? ''),
  };
}

function RecipeForm({
  doctorId,
  herbs,
  editing,
  onDone,
  onCancel,
}: {
  doctorId: number;
  herbs: CrudRow[];
  editing: CrudRow | null;
  onDone: () => void;
  onCancel: () => void;
}) {
  const t = useT();
  const [values, setValues] = useState<RecipeFormValues>(() =>
    editing ? recipeValuesFrom(editing) : emptyRecipeValues(),
  );
  const [ingredients, setIngredients] = useState<IngredientRow[]>(() => {
    const existing = editing ? rowValue(editing, 'ingredients') : undefined;
    return Array.isArray(existing) && existing.length > 0
      ? (existing as CrudRow[]).map(toIngredientRow)
      : [emptyIngredientRow()];
  });
  const [submitting, setSubmitting] = useState(false);

  const setField = (name: keyof RecipeFormValues, value: string) => {
    setValues((prev) => ({ ...prev, [name]: value }));
  };

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setSubmitting(true);
    try {
      const body = {
        code: values.code,
        name: values.name,
        doctor_id: doctorId,
        indication: values.indication,
        preparation: values.preparation,
        usage: values.usage,
        caution: values.caution,
        care_stage: values.care_stage,
        data_year: Number(values.data_year),
        ingredients: toIngredientPayload(ingredients),
      };
      if (editing) {
        await api.send('PUT', `/api/recipes/${rowId(editing)}`, body);
      } else {
        await api.send('POST', '/api/recipes', body);
      }
      onDone();
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <form onSubmit={handleSubmit}>
      <label htmlFor="recipe-code">{t('code')}</label>
      <input
        id="recipe-code"
        type="text"
        required
        value={values.code}
        onChange={(event) => setField('code', event.target.value)}
      />
      <label htmlFor="recipe-name">{t('name')}</label>
      <input
        id="recipe-name"
        type="text"
        required
        value={values.name}
        onChange={(event) => setField('name', event.target.value)}
      />
      <label htmlFor="recipe-indication">{t('indication')}</label>
      <input
        id="recipe-indication"
        type="text"
        value={values.indication}
        onChange={(event) => setField('indication', event.target.value)}
      />
      <label htmlFor="recipe-preparation">{t('preparation')}</label>
      <textarea
        id="recipe-preparation"
        value={values.preparation}
        onChange={(event) => setField('preparation', event.target.value)}
      />
      <label htmlFor="recipe-usage">{t('usage')}</label>
      <textarea
        id="recipe-usage"
        value={values.usage}
        onChange={(event) => setField('usage', event.target.value)}
      />
      <label htmlFor="recipe-caution">{t('caution')}</label>
      <textarea
        id="recipe-caution"
        value={values.caution}
        onChange={(event) => setField('caution', event.target.value)}
      />
      <label htmlFor="recipe-care-stage">{t('careStage')}</label>
      <input
        id="recipe-care-stage"
        type="text"
        value={values.care_stage}
        onChange={(event) => setField('care_stage', event.target.value)}
      />
      <label htmlFor="recipe-data-year">{t('dataYear')}</label>
      <input
        id="recipe-data-year"
        type="number"
        value={values.data_year}
        onChange={(event) => setField('data_year', event.target.value)}
      />
      <IngredientEditor rows={ingredients} herbs={herbs} onChange={setIngredients} />
      <button type="submit" disabled={submitting}>
        {t('save')}
      </button>
      <button type="button" onClick={onCancel}>
        {t('cancel')}
      </button>
    </form>
  );
}

export default function RecipesPage() {
  const t = useT();
  const { me } = useMe();
  const [districts, setDistricts] = useState<CrudRow[]>([]);
  const [doctors, setDoctors] = useState<CrudRow[]>([]);
  const [herbs, setHerbs] = useState<CrudRow[]>([]);
  const [recipes, setRecipes] = useState<CrudRow[]>([]);
  const [districtId, setDistrictId] = useState<number | null>(null);
  const [doctorId, setDoctorId] = useState<number | null>(null);
  const [editing, setEditing] = useState<CrudRow | 'new' | null>(null);
  const [confirmingId, setConfirmingId] = useState<number | null>(null);

  useEffect(() => {
    api.get<CrudRow[]>('/api/districts').then(setDistricts);
    api.get<CrudRow[]>('/api/public/herbs').then(setHerbs);
  }, []);

  useEffect(() => {
    if (districtId !== null || districts.length === 0) {
      return;
    }
    setDistrictId(me?.district_id ?? rowId(districts[0]));
  }, [districts, me, districtId]);

  useEffect(() => {
    if (districtId === null) {
      return;
    }
    api.get<CrudRow[]>(`/api/doctors?district_id=${districtId}`).then((rows) => {
      setDoctors(rows);
      setDoctorId(rows.length > 0 ? rowId(rows[0]) : null);
    });
  }, [districtId]);

  const refreshRecipes = useCallback(async () => {
    if (doctorId === null) {
      setRecipes([]);
      return;
    }
    setRecipes(await api.get<CrudRow[]>(`/api/recipes?doctor_id=${doctorId}`));
  }, [doctorId]);

  useEffect(() => {
    refreshRecipes();
  }, [refreshRecipes]);

  const handleDelete = async (row: CrudRow) => {
    await api.send('DELETE', `/api/recipes/${rowId(row)}`);
    setConfirmingId(null);
    await refreshRecipes();
  };

  return (
    <section>
      <h1>{t('recipes')}</h1>
      <label htmlFor="recipe-district-filter">{t('district')}</label>
      <select
        id="recipe-district-filter"
        value={districtId ?? ''}
        onChange={(event) => {
          setDistrictId(Number(event.target.value));
          setDoctorId(null);
        }}
      >
        {districts.map((district) => (
          <option key={rowId(district)} value={rowId(district)}>
            {String(rowValue(district, 'name'))}
          </option>
        ))}
      </select>
      <label htmlFor="recipe-doctor-filter">{t('doctor')}</label>
      <select
        id="recipe-doctor-filter"
        value={doctorId ?? ''}
        onChange={(event) => setDoctorId(Number(event.target.value))}
      >
        {doctors.map((doctor) => (
          <option key={rowId(doctor)} value={rowId(doctor)}>
            {String(rowValue(doctor, 'full_name'))}
          </option>
        ))}
      </select>
      <ExportLinks resource="recipes" districtId={districtId} />
      {doctorId !== null && (
        <button type="button" onClick={() => setEditing('new')}>
          {t('add')}
        </button>
      )}
      {editing !== null && doctorId !== null && (
        <RecipeForm
          doctorId={doctorId}
          herbs={herbs}
          editing={editing === 'new' ? null : editing}
          onDone={async () => {
            setEditing(null);
            await refreshRecipes();
          }}
          onCancel={() => setEditing(null)}
        />
      )}
      <table>
        <thead>
          <tr>
            <th>{t('code')}</th>
            <th>{t('name')}</th>
            <th>{t('actions')}</th>
          </tr>
        </thead>
        <tbody>
          {recipes.map((row) => {
            const id = rowId(row);
            return (
              <tr key={id}>
                <td>{String(rowValue(row, 'code') ?? '')}</td>
                <td>{String(rowValue(row, 'name') ?? '')}</td>
                <td>
                  <button type="button" onClick={() => setEditing(row)}>
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
