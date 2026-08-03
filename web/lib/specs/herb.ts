import type { ResourceSpec } from '@/lib/crud';

export const herbSpec: ResourceSpec = {
  basePath: '/api/herbs',
  titleKey: 'herbs',
  fields: [
    { name: 'thai_name', labelKey: 'thaiName', type: 'text', required: true },
    { name: 'local_name', labelKey: 'localName', type: 'text' },
    { name: 'scientific_name', labelKey: 'scientificName', type: 'text' },
    { name: 'part_used', labelKey: 'partUsed', type: 'text' },
    { name: 'properties', labelKey: 'properties', type: 'text' },
  ],
};
