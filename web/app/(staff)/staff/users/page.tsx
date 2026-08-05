'use client';

import { useEffect, useMemo, useState } from 'react';

import { CrudTable } from '@/components/CrudTable';
import { api } from '@/lib/api';
import { RequireAdmin } from '@/lib/auth';
import { type CrudRow } from '@/lib/crud';
import { userSpec } from '@/lib/specs/user';

export default function UsersPage() {
  const [districts, setDistricts] = useState<CrudRow[]>([]);

  useEffect(() => {
    api.get<CrudRow[]>('/api/districts').then(setDistricts);
  }, []);

  const spec = useMemo(() => userSpec(districts), [districts]);

  return (
    <RequireAdmin>
      <CrudTable spec={spec} />
    </RequireAdmin>
  );
}
