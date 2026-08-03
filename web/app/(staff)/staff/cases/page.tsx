'use client';

import { type FormEvent, useCallback, useEffect, useState } from 'react';

import { ExportLinks } from '@/components/ExportLinks';
import { PhotoUpload } from '@/components/PhotoUpload';
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
import { Textarea } from '@/components/ui/textarea';
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
      <Label htmlFor="case-patient-gender">{t('patientGender')}</Label>
      <Select
        value={values.patient_gender || undefined}
        onValueChange={(value) => setField('patient_gender', value)}
      >
        <SelectTrigger id="case-patient-gender" aria-label={t('patientGender')}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="male">{t('genderMale')}</SelectItem>
          <SelectItem value="female">{t('genderFemale')}</SelectItem>
          <SelectItem value="other">{t('genderOther')}</SelectItem>
        </SelectContent>
      </Select>
      <Label htmlFor="case-patient-age-range">{t('patientAgeRange')}</Label>
      <Select
        value={values.patient_age_range || undefined}
        onValueChange={(value) => setField('patient_age_range', value)}
      >
        <SelectTrigger id="case-patient-age-range" aria-label={t('patientAgeRange')}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="0-12">{t('ageRange0to12')}</SelectItem>
          <SelectItem value="13-19">{t('ageRange13to19')}</SelectItem>
          <SelectItem value="20-39">{t('ageRange20to39')}</SelectItem>
          <SelectItem value="40-59">{t('ageRange40to59')}</SelectItem>
          <SelectItem value="60+">{t('ageRange60plus')}</SelectItem>
        </SelectContent>
      </Select>
      <Label htmlFor="case-condition">{t('condition')}</Label>
      <Input
        id="case-condition"
        type="text"
        required
        value={values.condition}
        onChange={(event) => setField('condition', event.target.value)}
      />
      <Label htmlFor="case-treatment">{t('treatment')}</Label>
      <Textarea
        id="case-treatment"
        value={values.treatment}
        onChange={(event) => setField('treatment', event.target.value)}
      />
      <Label htmlFor="case-result">{t('result')}</Label>
      <Select
        value={values.result || undefined}
        required
        onValueChange={(value) => setField('result', value)}
      >
        <SelectTrigger id="case-result" aria-label={t('result')}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="cured">{t('resultCured')}</SelectItem>
          <SelectItem value="better">{t('resultBetter')}</SelectItem>
          <SelectItem value="no_change">{t('resultNoChange')}</SelectItem>
        </SelectContent>
      </Select>
      <Label htmlFor="case-duration">{t('duration')}</Label>
      <Input
        id="case-duration"
        type="text"
        value={values.duration}
        onChange={(event) => setField('duration', event.target.value)}
      />
      <Label htmlFor="case-data-year">{t('dataYear')}</Label>
      <Input
        id="case-data-year"
        type="number"
        value={values.data_year}
        onChange={(event) => setField('data_year', event.target.value)}
      />
      {editing && <PhotoUpload uploadPath={`/api/cases/${rowId(editing)}/photo`} />}
      <Button type="submit" disabled={submitting}>
        {t('save')}
      </Button>
      <Button type="button" variant="outline" onClick={onCancel}>
        {t('cancel')}
      </Button>
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
      <Label htmlFor="case-district-filter">{t('district')}</Label>
      <Select
        value={districtId != null ? String(districtId) : ''}
        onValueChange={(value) => setDistrictId(Number(value))}
      >
        <SelectTrigger id="case-district-filter" aria-label={t('district')}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {districts.map((district) => (
            <SelectItem key={rowId(district)} value={String(rowId(district))}>
              {String(rowValue(district, 'name'))}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Label htmlFor="case-doctor-filter">{t('doctor')}</Label>
      <Select
        value={doctorId != null ? String(doctorId) : ''}
        onValueChange={(value) => setDoctorId(Number(value))}
      >
        <SelectTrigger id="case-doctor-filter" aria-label={t('doctor')}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {doctors.map((doctor) => (
            <SelectItem key={rowId(doctor)} value={String(rowId(doctor))}>
              {String(rowValue(doctor, 'full_name'))}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Label htmlFor="case-recipe-filter">{t('recipe')}</Label>
      <Select
        value={recipeId != null ? String(recipeId) : ''}
        onValueChange={(value) => setRecipeId(Number(value))}
      >
        <SelectTrigger id="case-recipe-filter" aria-label={t('recipe')}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {recipes.map((recipe) => (
            <SelectItem key={rowId(recipe)} value={String(rowId(recipe))}>
              {String(rowValue(recipe, 'name'))}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <ExportLinks resource="cases" districtId={districtId} />
      {recipeId !== null && (
        <Button type="button" onClick={() => setEditing('new')}>
          {t('add')}
        </Button>
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
                  <Button type="button" variant="outline" onClick={() => setEditing(row)}>
                    {t('edit')}
                  </Button>
                  {confirmingId === id ? (
                    <span>
                      {t('confirmDelete')}
                      <Button type="button" onClick={() => handleDelete(row)}>
                        {t('yes')}
                      </Button>
                      <Button type="button" variant="outline" onClick={() => setConfirmingId(null)}>
                        {t('no')}
                      </Button>
                    </span>
                  ) : (
                    <Button type="button" variant="outline" onClick={() => setConfirmingId(id)}>
                      {t('delete')}
                    </Button>
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
