'use client';

import { CrudTable } from '@/components/CrudTable';
import { districtSpec } from '@/lib/specs/district';

export default function DistrictsPage() {
  return <CrudTable spec={districtSpec} />;
}
