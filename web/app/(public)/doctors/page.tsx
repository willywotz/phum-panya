'use client';

import Link from 'next/link';
import { type FormEvent, useEffect, useState } from 'react';

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
      <h1>{t('doctors')}</h1>
      <form onSubmit={(event: FormEvent<HTMLFormElement>) => event.preventDefault()}>
        <label htmlFor="doctor-search">{t('search')}</label>
        <input
          id="doctor-search"
          type="text"
          value={q}
          onChange={(event) => setQ(event.target.value)}
        />
        <label htmlFor="doctor-district-filter">{t('district')}</label>
        <select
          id="doctor-district-filter"
          value={districtId}
          onChange={(event) => setDistrictId(event.target.value)}
        >
          <option value="">{t('allDistricts')}</option>
          {districts.map((d) => (
            <option key={d.id} value={d.id}>
              {d.name}
            </option>
          ))}
        </select>
        <button type="submit">{t('search')}</button>
      </form>
      <ul>
        {doctors.map((doctor) => (
          <li key={doctor.id}>
            <Link href={`/doctor?id=${doctor.id}`}>
              {doctor.full_name}
              {doctor.known_as ? ` (${doctor.known_as})` : ''}
            </Link>
          </li>
        ))}
      </ul>
    </section>
  );
}
