'use client';

import { type ChangeEvent, useState } from 'react';

import { useT } from '@/lib/i18n';

// Multipart upload; unlike lib/api.ts this posts a FormData body directly
// (same-origin, credentialed via cookie — no CSRF header needed either).
export function PhotoUpload({ uploadPath }: { uploadPath: string }) {
  const t = useT();
  const [photo, setPhoto] = useState<string | null>(null);
  const [uploading, setUploading] = useState(false);

  const handleChange = async (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    if (!file) {
      return;
    }
    setUploading(true);
    try {
      const formData = new FormData();
      formData.append('photo', file);
      const res = await fetch(uploadPath, {
        method: 'POST',
        credentials: 'include',
        body: formData,
      });
      if (res.ok) {
        const body = (await res.json()) as { photo: string };
        setPhoto(body.photo);
      }
    } finally {
      setUploading(false);
    }
  };

  return (
    <div>
      <label htmlFor="photo-upload">{t('photo')}</label>
      <input
        id="photo-upload"
        type="file"
        accept="image/*"
        disabled={uploading}
        onChange={handleChange}
      />
      {photo && <p>{photo}</p>}
    </div>
  );
}
