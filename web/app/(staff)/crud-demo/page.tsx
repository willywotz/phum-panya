'use client';

// Throwaway route to e2e-test CrudTable/CrudForm ahead of the real staff
// resource screens (Task 30). Remove once those screens exist.

import { CrudTable } from '@/components/CrudTable';
import { type ResourceSpec } from '@/lib/crud';
import { RequireStaff } from '@/lib/auth';

const districtSpec: ResourceSpec = {
  basePath: '/api/districts',
  titleKey: 'districts',
  fields: [
    { name: 'name', labelKey: 'name', type: 'text', required: true },
    { name: 'province', labelKey: 'province', type: 'text', required: true },
  ],
};

export default function CrudDemoPage() {
  return (
    <RequireStaff>
      <CrudTable spec={districtSpec} />
    </RequireStaff>
  );
}
