'use client';

import { CrudTable } from '@/components/CrudTable';
import { RequireAdmin } from '@/lib/auth';
import { districtSpec } from '@/lib/specs/district';

export default function DistrictsPage() {
  return (
    <RequireAdmin>
      <CrudTable spec={districtSpec} />
    </RequireAdmin>
  );
}
