// Config-driven CRUD types shared by CrudTable and CrudForm.

export type FieldType =
  | 'text'
  | 'number'
  | 'select'
  | 'multiselect'
  | 'checkbox'
  | 'textarea';

export interface FieldOption {
  value: string;
  labelKey: string;
}

export interface FieldSpec {
  name: string; // maps to the API JSON key
  labelKey: string; // i18n key
  type: FieldType;
  options?: FieldOption[]; // for select/multiselect
  required?: boolean;
}

export interface ResourceSpec {
  basePath: string; // e.g. '/api/districts'
  titleKey: string;
  fields: FieldSpec[]; // fields shown in the form; a subset also used as table columns
  listColumns?: string[]; // field names to show as table columns (default: all text/number fields)
}

export type CrudRow = Record<string, unknown>;

// The Go JSON API capitalizes struct fields without a json tag (e.g. "ID",
// "Name"), while request bodies use each field's own lowercase json tag.
// Look up a row's id/value tolerantly across both casings.
function capitalize(name: string): string {
  return name.charAt(0).toUpperCase() + name.slice(1);
}

export function rowId(row: CrudRow): number {
  return Number(row.id ?? row.ID);
}

export function rowValue(row: CrudRow, name: string): unknown {
  return name in row ? row[name] : row[capitalize(name)];
}
