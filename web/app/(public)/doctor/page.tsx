'use client';

import { useSearchParams } from 'next/navigation';
import { Suspense, useEffect, useState } from 'react';

import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
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
  photos: string[];
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
      <Button type="button" variant="outline" className="no-print mb-4" onClick={() => window.print()}>
        {t('print')}
      </Button>
      <h1 className="text-2xl font-bold text-primary">{doctor.full_name}</h1>
      {doctor.photo && (
        // eslint-disable-next-line @next/next/no-img-element
        <img
          src={`/media/${doctor.photo}`}
          alt={doctor.full_name}
          className="my-4 max-w-xs rounded-lg"
        />
      )}
      <Card className="mb-6">
        <CardContent>
          {doctor.known_as && <p className="mb-2 text-muted-foreground">{doctor.known_as}</p>}
          <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
            <dt className="font-medium">{t('specialty')}</dt>
            <dd>{doctor.specialty}</dd>
            <dt className="font-medium">{t('district')}</dt>
            <dd>{districtLabel(doctor)}</dd>
            <dt className="font-medium">{t('lineage')}</dt>
            <dd>{doctor.lineage}</dd>
            <dt className="font-medium">{t('yearsExperience')}</dt>
            <dd>{doctor.years_experience}</dd>
            <dt className="font-medium">{t('firstYear')}</dt>
            <dd>{doctor.first_year}</dd>
          </dl>
        </CardContent>
      </Card>
      <section>
        <h2 className="mb-3 text-xl font-semibold">{t('recipes')}</h2>
        <div className="grid gap-4">
          {doctor.recipes.map((recipe) => (
            <Card key={recipe.id}>
              <CardHeader>
                <CardTitle>{recipe.name}</CardTitle>
              </CardHeader>
              <CardContent className="grid gap-2 text-sm">
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
                {recipe.photos?.map((photo) => (
                  // eslint-disable-next-line @next/next/no-img-element
                  <img
                    key={photo}
                    src={`/media/${photo}`}
                    alt={recipe.name}
                    className="max-w-xs rounded-lg"
                  />
                ))}
                <section>
                  <h4 className="font-medium">{t('ingredients')}</h4>
                  <ul className="list-inside list-disc">
                    {recipe.ingredients.map((ing, index) => (
                      <li key={index}>
                        {ing.herb_name} {ing.amount} {ing.unit}
                        {ing.note && ` (${ing.note})`}
                      </li>
                    ))}
                  </ul>
                </section>
                <section>
                  <h4 className="font-medium">{t('cases')}</h4>
                  <ul className="list-inside list-disc">
                    {recipe.cases.map((c) => (
                      <li key={c.id}>
                        {c.condition} — {t('result')}: {c.result}
                        {c.photo && (
                          // eslint-disable-next-line @next/next/no-img-element
                          <img
                            src={`/media/${c.photo}`}
                            alt={c.condition}
                            className="mt-1 max-w-xs rounded-lg"
                          />
                        )}
                      </li>
                    ))}
                  </ul>
                </section>
              </CardContent>
            </Card>
          ))}
        </div>
      </section>
    </article>
  );
}
