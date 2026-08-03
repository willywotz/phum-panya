'use client';

import { useRouter } from 'next/navigation';
import { type FormEvent, useState } from 'react';

import { api, ApiError } from '@/lib/api';
import { LangToggle, useT } from '@/lib/i18n';

export default function LoginPage() {
  const router = useRouter();
  const t = useT();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState(false);
  const [submitting, setSubmitting] = useState(false);

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setError(false);
    setSubmitting(true);
    try {
      await api.send('POST', '/api/login', { email, password });
      router.replace('/staff');
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setError(true);
      } else {
        throw err;
      }
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <main>
      <LangToggle />
      <h1>{t('signIn')}</h1>
      <form onSubmit={handleSubmit}>
        <label htmlFor="email">{t('email')}</label>
        <input
          id="email"
          type="email"
          value={email}
          onChange={(event) => setEmail(event.target.value)}
          required
        />
        <label htmlFor="password">{t('password')}</label>
        <input
          id="password"
          type="password"
          value={password}
          onChange={(event) => setPassword(event.target.value)}
          required
        />
        {error && <p role="alert">{t('loginError')}</p>}
        <button type="submit" disabled={submitting}>
          {t('signIn')}
        </button>
      </form>
    </main>
  );
}
