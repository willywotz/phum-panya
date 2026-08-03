'use client';

import Link from 'next/link';
import { type FormEvent, useEffect, useState } from 'react';

import { Button } from '@/components/ui/button';
import { Card, CardHeader, CardTitle } from '@/components/ui/card';
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

interface PublicDoctor {
  id: number;
  full_name: string;
  known_as: string;
  district_id: number;
}

interface PublicDistrict {
  id: number;
  name: string;
  province: string;
}

export default function DoctorsPage() {
  const t = useT();
  const [districts, setDistricts] = useState<PublicDistrict[]>([]);
  const [doctors, setDoctors] = useState<PublicDoctor[]>([]);
  const [q, setQ] = useState('');
  const [districtId, setDistrictId] = useState('');

  useEffect(() => {
    api.get<PublicDistrict[]>('/api/public/districts').then(setDistricts);
  }, []);

  useEffect(() => {
    const params = new URLSearchParams();
    if (q) params.set('q', q);
    if (districtId) params.set('district_id', districtId);
    api.get<PublicDoctor[]>(`/api/public/doctors?${params}`).then(setDoctors);
  }, [q, districtId]);

  return (
    <section>
      <h1 className="mb-4 text-2xl font-bold text-primary">{t('doctors')}</h1>
      <form
        onSubmit={(event: FormEvent<HTMLFormElement>) => event.preventDefault()}
        className="mb-6 flex flex-wrap items-end gap-3"
      >
        <div className="grid gap-1.5">
          <Label htmlFor="doctor-search">{t('search')}</Label>
          <Input
            id="doctor-search"
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
            <SelectTrigger id="doctor-district-filter" aria-label={t('district')}>
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
        <Button type="submit">{t('search')}</Button>
      </form>
      <div className="grid gap-3">
        {doctors.map((doctor) => (
          <Link key={doctor.id} href={`/doctor?id=${doctor.id}`} className="block">
            <Card className="transition-colors hover:border-primary">
              <CardHeader>
                <CardTitle>
                  {doctor.full_name}
                  {doctor.known_as ? ` (${doctor.known_as})` : ''}
                </CardTitle>
              </CardHeader>
            </Card>
          </Link>
        ))}
      </div>
    </section>
  );
}
