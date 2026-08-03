import type { CrudRow, FieldOption, ResourceSpec } from '@/lib/crud';
import { districtOptions } from '@/lib/specs/shared';

const genderOptions: FieldOption[] = [
  { value: 'male', labelKey: 'genderMale' },
  { value: 'female', labelKey: 'genderFemale' },
  { value: 'other', labelKey: 'genderOther' },
];

const specialtyOptions: FieldOption[] = [
  { value: 'herbal', labelKey: 'specialtyHerbal' },
  { value: 'postpartum', labelKey: 'specialtyPostpartum' },
  { value: 'bone', labelKey: 'specialtyBone' },
  { value: 'massage', labelKey: 'specialtyMassage' },
  { value: 'other', labelKey: 'specialtyOther' },
];

const statusOptions: FieldOption[] = [
  { value: 'active', labelKey: 'statusActive' },
  { value: 'inactive', labelKey: 'statusInactive' },
  { value: 'deceased', labelKey: 'statusDeceased' },
];

// district_id has no fixed options: build the spec once districts are
// fetched. basePath carries the district filter (?district_id=) so
// re-mounting CrudTable under a new key refetches the right list.
export function doctorSpec(districts: CrudRow[], basePath: string): ResourceSpec {
  return {
    basePath,
    titleKey: 'doctors',
    fields: [
      { name: 'code', labelKey: 'code', type: 'text', required: true },
      { name: 'full_name', labelKey: 'fullName', type: 'text', required: true },
      { name: 'known_as', labelKey: 'knownAs', type: 'text' },
      { name: 'gender', labelKey: 'gender', type: 'select', options: genderOptions },
      { name: 'birth_year', labelKey: 'birthYear', type: 'number' },
      {
        name: 'district_id',
        labelKey: 'district',
        type: 'select',
        required: true,
        numeric: true,
        options: districtOptions(districts),
      },
      { name: 'address', labelKey: 'address', type: 'text' },
      { name: 'phone', labelKey: 'phone', type: 'text' },
      {
        name: 'specialty',
        labelKey: 'specialty',
        type: 'multiselect',
        options: specialtyOptions,
      },
      { name: 'years_experience', labelKey: 'yearsExperience', type: 'number' },
      { name: 'lineage', labelKey: 'lineage', type: 'text' },
      { name: 'consent_obtained', labelKey: 'consentObtained', type: 'checkbox' },
      { name: 'consent_date', labelKey: 'consentDate', type: 'date' },
      {
        name: 'status',
        labelKey: 'status',
        type: 'select',
        required: true,
        options: statusOptions,
      },
      { name: 'first_year', labelKey: 'firstYear', type: 'number' },
    ],
    listColumns: ['code', 'full_name', 'status'],
  };
}
