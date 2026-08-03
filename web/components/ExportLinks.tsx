'use client';

import { useT } from '@/lib/i18n';

// Anchor links (not fetch) so the browser handles the file save; the
// session cookie rides along automatically. A district_editor's export is
// scoped server-side regardless of districtId; a central_admin narrows the
// export to the district currently selected on the page.
export function ExportLinks({
  resource,
  districtId,
}: {
  resource: string;
  districtId?: number | null;
}) {
  const t = useT();
  const query = districtId != null ? `?district_id=${districtId}` : '';
  return (
    <p>
      <a href={`/api/export/${resource}.csv${query}`} download>
        {t('exportCsv')}
      </a>{' '}
      <a href={`/api/export/${resource}.xlsx${query}`} download>
        {t('exportExcel')}
      </a>
    </p>
  );
}
