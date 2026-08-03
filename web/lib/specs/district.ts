import type { ResourceSpec } from '@/lib/crud';

export const districtSpec: ResourceSpec = {
  basePath: '/api/districts',
  titleKey: 'districts',
  fields: [
    { name: 'name', labelKey: 'name', type: 'text', required: true },
    { name: 'province', labelKey: 'province', type: 'text', required: true },
  ],
};
