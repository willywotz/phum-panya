'use client';

import { type FormEvent, useCallback, useEffect, useState } from 'react';

import { PhotoUpload } from '@/components/PhotoUpload';
import { api } from '@/lib/api';
import { useMe } from '@/lib/auth';
import { type CrudRow, rowId, rowValue } from '@/lib/crud';
import { useT } from '@/lib/i18n';

interface CaseFormValues {
  patient_gender: string;
  patient_age_range: string;
  condition: string;
  treatment: string;
  result: string;
  duration: string;
  data_year: string;
}

function emptyCaseValues(): CaseFormValues {
  return {
    patient_gender: '',
    patient_age_range: '',
    condition: '',
    treatment: '',
    result: '',
    duration: '',
    data_year: '',
  };
}

function caseValuesFrom(row: CrudRow): CaseFormValues {
  return {
    patient_gender: String(rowValue(row, 'patient_gender') ?? ''),
    patient_age_range: String(rowValue(row, 'patient_age_range') ?? ''),
    condition: String(rowValue(row, 'condition') ?? ''),
    treatment: String(rowValue(row, 'treatment') ?? ''),
    result: String(rowValue(row, 'result') ?? ''),
    duration: String(rowValue(row, 'duration') ?? ''),
    data_year: String(rowValue(row, 'data_year') ?? ''),
  };
}

function CaseForm({
  recipeId,
  editing,
  onDone,
  onCancel,
}: {
  recipeId: number;
  editing: CrudRow | null;
  onDone: () => void;
  onCancel: () => void;
}) {
  const t = useT();
  const [values, setValues] = useState<CaseFormValues>(() =>
    editing ? caseValuesFrom(editing) : emptyCaseValues(),
  );
  const [submitting, setSubmitting] = useState(false);

  const setField = (name: keyof CaseFormValues, value: string) => {
    setValues((prev) => ({ ...prev, [name]: value }));
  };

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setSubmitting(true);
    try {
      const body = {
        recipe_id: recipeId,
        patient_gender: values.patient_gender,
        patient_age_range: values.patient_age_range,
        condition: values.condition,
        treatment: values.treatment,
        result: values.result,
        duration: values.duration,
        data_year: Number(values.data_year),
      };
      if (editing) {
        await api.send('PUT', `/api/cases/${rowId(editing)}`, body);
      } else {
        await api.send('POST', '/api/cases', body);
      }
      onDone();
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <form onSubmit={handleSubmit}>
      <label htmlFor="case-patient-gender">{t('patientGender')}</label>
      <select
        id="case-patient-gender"
        value={values.patient_gender}
        onChange={(event) => setField('patient_gender', event.target.value)}
      >
        <option value="" />
        <option value="male">{t('genderMale')}</option>
        <option value="female">{t('genderFemale')}</option>
        <option value="other">{t('genderOther')}</option>
      </select>
      <label htmlFor="case-patient-age-range">{t('patientAgeRange')}</label>
      <select
        id="case-patient-age-range"
        value={values.patient_age_range}
        onChange={(event) => setField('patient_age_range', event.target.value)}
      >
        <option value="" />
        <option value="0-12">{t('ageRange0to12')}</option>
        <option value="13-19">{t('ageRange13to19')}</option>
        <option value="20-39">{t('ageRange20to39')}</option>
        <option value="40-59">{t('ageRange40to59')}</option>
        <option value="60+">{t('ageRange60plus')}</option>
      </select>
      <label htmlFor="case-condition">{t('condition')}</label>
      <input
        id="case-condition"
        type="text"
        required
        value={values.condition}
        onChange={(event) => setField('condition', event.target.value)}
      />
      <label htmlFor="case-treatment">{t('treatment')}</label>
      <textarea
        id="case-treatment"
        value={values.treatment}
        onChange={(event) => setField('treatment', event.target.value)}
      />
      <label htmlFor="case-result">{t('result')}</label>
      <select
        id="case-result"
        required
        value={values.result}
        onChange={(event) => setField('result', event.target.value)}
      >
        <option value="" />
        <option value="cured">{t('resultCured')}</option>
        <option value="better">{t('resultBetter')}</option>
        <option value="no_change">{t('resultNoChange')}</option>
      </select>
      <label htmlFor="case-duration">{t('duration')}</label>
      <input
        id="case-duration"
        type="text"
        value={values.duration}
        onChange={(event) => setField('duration', event.target.value)}
      />
      <label htmlFor="case-data-year">{t('dataYear')}</label>
      <input
        id="case-data-year"
        type="number"
        value={values.data_year}
        onChange={(event) => setField('data_year', event.target.value)}
      />
      {editing && <PhotoUpload uploadPath={`/api/cases/${rowId(editing)}/photo`} />}
      <button type="submit" disabled={submitting}>
        {t('save')}
      </button>
      <button type="button" onClick={onCancel}>
        {t('cancel')}
      </button>
    </form>
  );
}

export default function CasesPage() {
  const t = useT();
  const { me } = useMe();
  const [districts, setDistricts] = useState<CrudRow[]>([]);
  const [doctors, setDoctors] = useState<CrudRow[]>([]);
  const [recipes, setRecipes] = useState<CrudRow[]>([]);
  const [cases, setCases] = useState<CrudRow[]>([]);
  const [districtId, setDistrictId] = useState<number | null>(null);
  const [doctorId, setDoctorId] = useState<number | null>(null);
  const [recipeId, setRecipeId] = useState<number | null>(null);
  const [editing, setEditing] = useState<CrudRow | 'new' | null>(null);
  const [confirmingId, setConfirmingId] = useState<number | null>(null);

  useEffect(() => {
    api.get<CrudRow[]>('/api/districts').then(setDistricts);
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

  useEffect(() => {
    if (doctorId === null) {
      setRecipes([]);
      setRecipeId(null);
      return;
    }
    api.get<CrudRow[]>(`/api/recipes?doctor_id=${doctorId}`).then((rows) => {
      setRecipes(rows);
      setRecipeId(rows.length > 0 ? rowId(rows[0]) : null);
    });
  }, [doctorId]);

  const refreshCases = useCallback(async () => {
    if (recipeId === null) {
      setCases([]);
      return;
    }
    setCases(await api.get<CrudRow[]>(`/api/cases?recipe_id=${recipeId}`));
  }, [recipeId]);

  useEffect(() => {
    refreshCases();
  }, [refreshCases]);

  const handleDelete = async (row: CrudRow) => {
    await api.send('DELETE', `/api/cases/${rowId(row)}`);
    setConfirmingId(null);
    await refreshCases();
  };

  return (
    <section>
      <h1>{t('cases')}</h1>
      <label htmlFor="case-district-filter">{t('district')}</label>
      <select
        id="case-district-filter"
        value={districtId ?? ''}
        onChange={(event) => setDistrictId(Number(event.target.value))}
      >
        {districts.map((district) => (
          <option key={rowId(district)} value={rowId(district)}>
            {String(rowValue(district, 'name'))}
          </option>
        ))}
      </select>
      <label htmlFor="case-doctor-filter">{t('doctor')}</label>
      <select
        id="case-doctor-filter"
        value={doctorId ?? ''}
        onChange={(event) => setDoctorId(Number(event.target.value))}
      >
        {doctors.map((doctor) => (
          <option key={rowId(doctor)} value={rowId(doctor)}>
            {String(rowValue(doctor, 'full_name'))}
          </option>
        ))}
      </select>
      <label htmlFor="case-recipe-filter">{t('recipe')}</label>
      <select
        id="case-recipe-filter"
        value={recipeId ?? ''}
        onChange={(event) => setRecipeId(Number(event.target.value))}
      >
        {recipes.map((recipe) => (
          <option key={rowId(recipe)} value={rowId(recipe)}>
            {String(rowValue(recipe, 'name'))}
          </option>
        ))}
      </select>
      {recipeId !== null && (
        <button type="button" onClick={() => setEditing('new')}>
          {t('add')}
        </button>
      )}
      {editing !== null && recipeId !== null && (
        <CaseForm
          recipeId={recipeId}
          editing={editing === 'new' ? null : editing}
          onDone={async () => {
            setEditing(null);
            await refreshCases();
          }}
          onCancel={() => setEditing(null)}
        />
      )}
      <table>
        <thead>
          <tr>
            <th>{t('condition')}</th>
            <th>{t('result')}</th>
            <th>{t('actions')}</th>
          </tr>
        </thead>
        <tbody>
          {cases.map((row) => {
            const id = rowId(row);
            return (
              <tr key={id}>
                <td>{String(rowValue(row, 'condition') ?? '')}</td>
                <td>{String(rowValue(row, 'result') ?? '')}</td>
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
