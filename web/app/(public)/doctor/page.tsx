'use client';

import { useSearchParams } from 'next/navigation';
import { Suspense, useEffect, useState } from 'react';

import { ApiError, api } from '@/lib/api';
import { useT } from '@/lib/i18n';

interface PublicCase {
  id: number;
  patient_gender: string;
  patient_age_range: string;
  condition: string;
  treatment: string;
  result: string;
  duration: string;
  photo: string;
}

interface PublicIngredient {
  herb_name: string;
  amount: string;
  unit: string;
  note: string;
}

interface PublicRecipe {
  id: number;
  name: string;
  indication: string;
  preparation: string;
  usage: string;
  caution: string;
  doctor_name: string;
  district_name: string;
  photo: string;
  ingredients: PublicIngredient[];
  cases: PublicCase[];
}

interface PublicDoctorDetail {
  id: number;
  full_name: string;
  known_as: string;
  specialty: string;
  district_id: number;
  lineage: string;
  years_experience: number;
  first_year: number;
  photo: string;
  recipes: PublicRecipe[];
}

// The district name is only carried on recipe attribution, not on the
// doctor projection itself; fall back to the numeric id when the doctor has
// no recipe to attribute it through.
function districtLabel(doctor: PublicDoctorDetail): string {
  return doctor.recipes[0]?.district_name ?? String(doctor.district_id);
}

export default function DoctorPage() {
  return (
    <Suspense fallback={null}>
      <DoctorDetail />
    </Suspense>
  );
}

function DoctorDetail() {
  const t = useT();
  const id = useSearchParams().get('id');
  const [doctor, setDoctor] = useState<PublicDoctorDetail | null>(null);
  const [notFound, setNotFound] = useState(false);

  useEffect(() => {
    if (!id) {
      return;
    }
    api
      .get<PublicDoctorDetail>(`/api/public/doctors/${id}`)
      .then(setDoctor)
      .catch((err) => {
        if (err instanceof ApiError && err.status === 404) {
          setNotFound(true);
        }
      });
  }, [id]);

  if (notFound) {
    return <p>{t('doctorNotFound')}</p>;
  }
  if (!doctor) {
    return null;
  }

  return (
    <article>
      <button type="button" className="no-print" onClick={() => window.print()}>
        {t('print')}
      </button>
      <h1>{doctor.full_name}</h1>
      {doctor.photo && (
        // eslint-disable-next-line @next/next/no-img-element
        <img src={`/media/${doctor.photo}`} alt={doctor.full_name} />
      )}
      {doctor.known_as && <p>{doctor.known_as}</p>}
      <dl>
        <dt>{t('specialty')}</dt>
        <dd>{doctor.specialty}</dd>
        <dt>{t('district')}</dt>
        <dd>{districtLabel(doctor)}</dd>
        <dt>{t('lineage')}</dt>
        <dd>{doctor.lineage}</dd>
        <dt>{t('yearsExperience')}</dt>
        <dd>{doctor.years_experience}</dd>
        <dt>{t('firstYear')}</dt>
        <dd>{doctor.first_year}</dd>
      </dl>
      <section>
        <h2>{t('recipes')}</h2>
        {doctor.recipes.map((recipe) => (
          <article key={recipe.id}>
            <h3>{recipe.name}</h3>
            <p>
              {t('indication')}: {recipe.indication}
            </p>
            <p>
              {t('preparation')}: {recipe.preparation}
            </p>
            <p>
              {t('usage')}: {recipe.usage}
            </p>
            {recipe.caution && (
              <p>
                {t('caution')}: {recipe.caution}
              </p>
            )}
            {recipe.photo && (
              // eslint-disable-next-line @next/next/no-img-element
              <img src={`/media/${recipe.photo}`} alt={recipe.name} />
            )}
            <section>
              <h4>{t('ingredients')}</h4>
              <ul>
                {recipe.ingredients.map((ing, index) => (
                  <li key={index}>
                    {ing.herb_name} {ing.amount} {ing.unit}
                    {ing.note && ` (${ing.note})`}
                  </li>
                ))}
              </ul>
            </section>
            <section>
              <h4>{t('cases')}</h4>
              <ul>
                {recipe.cases.map((c) => (
                  <li key={c.id}>
                    {c.condition} — {t('result')}: {c.result}
                    {c.photo && (
                      // eslint-disable-next-line @next/next/no-img-element
                      <img src={`/media/${c.photo}`} alt={c.condition} />
                    )}
                  </li>
                ))}
              </ul>
            </section>
          </article>
        ))}
      </section>
    </article>
  );
}
