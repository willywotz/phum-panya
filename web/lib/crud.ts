// Config-driven CRUD types shared by CrudTable and CrudForm.

export type FieldType =
  | 'text'
  | 'password'
  | 'number'
  | 'date'
  | 'select'
  | 'multiselect'
  | 'checkbox'
  | 'textarea';

export interface FieldOption {
  value: string;
  labelKey?: string; // i18n key; used when label is not set
  label?: string; // literal label (e.g. a district name); takes priority over labelKey
}

export interface FieldSpec {
  name: string; // maps to the API JSON key
  labelKey: string; // i18n key
  type: FieldType;
  options?: FieldOption[]; // for select/multiselect
  required?: boolean;
  createOnly?: boolean; // shown only when creating a new row (e.g. a password)
  numeric?: boolean; // coerce this select's string value to a Number on submit
}

export interface ResourceSpec {
  basePath: string; // e.g. '/api/districts'
  titleKey: string;
  fields: FieldSpec[]; // fields shown in the form; a subset also used as table columns
  listColumns?: string[]; // field names to show as table columns (default: all text/number fields)
}

export type CrudRow = Record<string, unknown>;

// The Go JSON API marshals model structs with no json tag using each
// field's exact Go name: PascalCase, with a standalone "id" segment
// upper-cased (e.g. "district_id" -> "DistrictID"). Request bodies instead
// use each field's own lowercase json tag. Look up a row's id/value
// tolerantly across both casings.
function pascalCase(name: string): string {
  return name
    .split('_')
    .map((part) =>
      part.toLowerCase() === 'id' ? 'ID' : part.charAt(0).toUpperCase() + part.slice(1),
    )
    .join('');
}

export function rowId(row: CrudRow): number {
  return Number(row.id ?? row.ID);
}

export function rowValue(row: CrudRow, name: string): unknown {
  return name in row ? row[name] : row[pascalCase(name)];
}

// resourceUrl builds an id-suffixed URL (for PUT/DELETE) from basePath.
// basePath may carry a list-scoping query string (e.g. a district filter
// used for GET); that query string must not leak into the id-suffixed URL,
// so it is stripped before appending /id.
export function resourceUrl(basePath: string, id: number): string {
  return `${basePath.split('?')[0]}/${id}`;
}
