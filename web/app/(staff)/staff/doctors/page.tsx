'use client';

import { useEffect, useMemo, useState } from 'react';

import { CrudTable } from '@/components/CrudTable';
import { ExportLinks } from '@/components/ExportLinks';
import { PhotoUpload } from '@/components/PhotoUpload';
import { api } from '@/lib/api';
import { useMe } from '@/lib/auth';
import { type CrudRow, rowId, rowValue } from '@/lib/crud';
import { useT } from '@/lib/i18n';
import { doctorSpec } from '@/lib/specs/doctor';

export default function DoctorsPage() {
  const t = useT();
  const { me } = useMe();
  const [districts, setDistricts] = useState<CrudRow[]>([]);
  const [districtId, setDistrictId] = useState<number | null>(null);

  useEffect(() => {
    api.get<CrudRow[]>('/api/districts').then(setDistricts);
  }, []);

  useEffect(() => {
    if (districtId !== null || districts.length === 0) {
      return;
    }
    setDistrictId(me?.district_id ?? rowId(districts[0]));
  }, [districts, me, districtId]);

  const spec = useMemo(
    () => doctorSpec(districts, `/api/doctors?district_id=${districtId}`),
    [districts, districtId],
  );

  if (districtId === null) {
    return null;
  }

  return (
    <section>
      <h1>{t('doctors')}</h1>
      <label htmlFor="doctor-district-filter">{t('district')}</label>
      <select
        id="doctor-district-filter"
        value={districtId}
        onChange={(event) => setDistrictId(Number(event.target.value))}
      >
        {districts.map((district) => (
          <option key={rowId(district)} value={rowId(district)}>
            {String(rowValue(district, 'name'))}
          </option>
        ))}
      </select>
      <ExportLinks resource="doctors" districtId={districtId} />
      <CrudTable
        key={districtId}
        spec={spec}
        newDefaults={{ district_id: String(districtId) }}
        formExtra={(id) => <PhotoUpload uploadPath={`/api/doctors/${id}/photo`} />}
      />
    </section>
  );
}
