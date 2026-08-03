'use client';

import Link from 'next/link';
import { type FormEvent, useEffect, useMemo, useState } from 'react';

import { api } from '@/lib/api';
import { useT } from '@/lib/i18n';

interface PublicHerb {
  id: number;
  thai_name: string;
}

interface PublicDoctor {
  district_id: number;
}

interface PublicRecipe {
  id: number;
  name: string;
  doctor_id: number;
  doctor_name: string;
  district_name: string;
}

export default function RecipesPage() {
  const t = useT();
  const [herbs, setHerbs] = useState<PublicHerb[]>([]);
  const [allDoctors, setAllDoctors] = useState<PublicDoctor[]>([]);
  const [recipes, setRecipes] = useState<PublicRecipe[]>([]);
  const [q, setQ] = useState('');
  const [districtId, setDistrictId] = useState('');
  const [herbId, setHerbId] = useState('');

  useEffect(() => {
    api.get<PublicHerb[]>('/api/public/herbs').then(setHerbs);
    api.get<PublicDoctor[]>('/api/public/doctors').then(setAllDoctors);
  }, []);

  useEffect(() => {
    const params = new URLSearchParams();
    if (q) params.set('q', q);
    if (districtId) params.set('district_id', districtId);
    if (herbId) params.set('herb_id', herbId);
    api.get<PublicRecipe[]>(`/api/public/recipes?${params}`).then(setRecipes);
  }, [q, districtId, herbId]);

  const districtIds = useMemo(
    () => Array.from(new Set(allDoctors.map((d) => d.district_id))).sort((a, b) => a - b),
    [allDoctors],
  );

  return (
    <section>
      <h1>{t('recipes')}</h1>
      <form onSubmit={(event: FormEvent<HTMLFormElement>) => event.preventDefault()}>
        <label htmlFor="recipe-search">{t('search')}</label>
        <input
          id="recipe-search"
          type="text"
          value={q}
          onChange={(event) => setQ(event.target.value)}
        />
        <label htmlFor="recipe-district-filter">{t('district')}</label>
        <select
          id="recipe-district-filter"
          value={districtId}
          onChange={(event) => setDistrictId(event.target.value)}
        >
          <option value="">{t('allDistricts')}</option>
          {districtIds.map((id) => (
            <option key={id} value={id}>
              {id}
            </option>
          ))}
        </select>
        <label htmlFor="recipe-herb-filter">{t('herb')}</label>
        <select
          id="recipe-herb-filter"
          value={herbId}
          onChange={(event) => setHerbId(event.target.value)}
        >
          <option value="">{t('allHerbs')}</option>
          {herbs.map((herb) => (
            <option key={herb.id} value={herb.id}>
              {herb.thai_name}
            </option>
          ))}
        </select>
        <button type="submit">{t('search')}</button>
      </form>
      <ul>
        {recipes.map((recipe) => (
          <li key={recipe.id}>
            <Link href={`/doctor?id=${recipe.doctor_id}`}>{recipe.name}</Link>
            {' — '}
            {recipe.doctor_name}, {recipe.district_name}
          </li>
        ))}
      </ul>
    </section>
  );
}
