// Helpers shared by specs that need a district picker.

import { type CrudRow, type FieldOption, rowId, rowValue } from '@/lib/crud';

export function districtOptions(districts: CrudRow[]): FieldOption[] {
  return districts.map((district) => ({
    value: String(rowId(district)),
    label: String(rowValue(district, 'name')),
  }));
}
