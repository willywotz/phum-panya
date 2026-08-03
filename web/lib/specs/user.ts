import type { CrudRow, ResourceSpec } from '@/lib/crud';
import { districtOptions } from '@/lib/specs/shared';

// district_id has no fixed options: build the spec once districts are
// fetched.
export function userSpec(districts: CrudRow[]): ResourceSpec {
  return {
    basePath: '/api/users',
    titleKey: 'users',
    fields: [
      { name: 'full_name', labelKey: 'fullName', type: 'text', required: true },
      { name: 'email', labelKey: 'email', type: 'text', required: true },
      {
        name: 'password',
        labelKey: 'password',
        type: 'password',
        required: true,
        createOnly: true,
      },
      {
        name: 'role',
        labelKey: 'role',
        type: 'select',
        required: true,
        options: [
          { value: 'central_admin', labelKey: 'roleCentralAdmin' },
          { value: 'district_editor', labelKey: 'roleDistrictEditor' },
        ],
      },
      {
        name: 'district_id',
        labelKey: 'district',
        type: 'select',
        numeric: true,
        options: districtOptions(districts),
      },
    ],
  };
}
