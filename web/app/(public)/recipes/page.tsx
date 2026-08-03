'use client';

import Link from 'next/link';
import { type FormEvent, useEffect, useState } from 'react';

import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { api } from '@/lib/api';
import { useT } from '@/lib/i18n';

interface PublicHerb {
  id: number;
  thai_name: string;
}

interface PublicDistrict {
  id: number;
  name: string;
  province: string;
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
  doctor_id: number;
  doctor_name: string;
  district_name: string;
  ingredients: PublicIngredient[];
}

export default function RecipesPage() {
  const t = useT();
  const [herbs, setHerbs] = useState<PublicHerb[]>([]);
  const [districts, setDistricts] = useState<PublicDistrict[]>([]);
  const [recipes, setRecipes] = useState<PublicRecipe[]>([]);
  const [q, setQ] = useState('');
  const [districtId, setDistrictId] = useState('');
  const [herbId, setHerbId] = useState('');

  useEffect(() => {
    api.get<PublicHerb[]>('/api/public/herbs').then(setHerbs);
    api.get<PublicDistrict[]>('/api/public/districts').then(setDistricts);
  }, []);

  useEffect(() => {
    const params = new URLSearchParams();
    if (q) params.set('q', q);
    if (districtId) params.set('district_id', districtId);
    if (herbId) params.set('herb_id', herbId);
    api.get<PublicRecipe[]>(`/api/public/recipes?${params}`).then(setRecipes);
  }, [q, districtId, herbId]);

  return (
    <section>
      <h1 className="mb-4 text-2xl font-bold text-primary">{t('recipes')}</h1>
      <form
        onSubmit={(event: FormEvent<HTMLFormElement>) => event.preventDefault()}
        className="mb-6 flex flex-wrap items-end gap-3"
      >
        <div className="grid gap-1.5">
          <Label htmlFor="recipe-search">{t('search')}</Label>
          <Input
            id="recipe-search"
            type="text"
            value={q}
            onChange={(event) => setQ(event.target.value)}
          />
        </div>
        <div className="grid gap-1.5">
          <Label>{t('district')}</Label>
          <Select
            value={districtId || 'all'}
            onValueChange={(value) => setDistrictId(value === 'all' ? '' : value)}
          >
            <SelectTrigger id="recipe-district-filter" aria-label={t('district')}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">{t('allDistricts')}</SelectItem>
              {districts.map((d) => (
                <SelectItem key={d.id} value={String(d.id)}>
                  {d.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="grid gap-1.5">
          <Label>{t('herb')}</Label>
          <Select
            value={herbId || 'all'}
            onValueChange={(value) => setHerbId(value === 'all' ? '' : value)}
          >
            <SelectTrigger id="recipe-herb-filter" aria-label={t('herb')}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">{t('allHerbs')}</SelectItem>
              {herbs.map((herb) => (
                <SelectItem key={herb.id} value={String(herb.id)}>
                  {herb.thai_name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <Button type="submit">{t('search')}</Button>
      </form>
      <div className="grid gap-3">
        {recipes.map((recipe) => (
          <Card key={recipe.id}>
            <CardHeader>
              <CardTitle>
                <Link href={`/doctor?id=${recipe.doctor_id}`}>{recipe.name}</Link>
              </CardTitle>
            </CardHeader>
            <CardContent className="text-muted-foreground">
              {recipe.doctor_name}, {recipe.district_name}
              {recipe.ingredients.length > 0 && (
                <p>{recipe.ingredients.map((ing) => ing.herb_name).join(', ')}</p>
              )}
            </CardContent>
          </Card>
        ))}
      </div>
    </section>
  );
}
