'use client';

import { createContext, useContext, useEffect, useState } from 'react';

export type Locale = 'th' | 'en';

const STORAGE_KEY = 'locale';
const DEFAULT_LOCALE: Locale = 'th';

type Dictionary = Record<string, string>;

const th: Dictionary = {
  signIn: 'เข้าสู่ระบบ',
};

const en: Dictionary = {
  signIn: 'Sign in',
};

const dictionaries: Record<Locale, Dictionary> = { th, en };

interface I18nContextValue {
  locale: Locale;
  setLocale: (locale: Locale) => void;
}

const I18nContext = createContext<I18nContextValue | null>(null);

export function I18nProvider({ children }: { children: React.ReactNode }) {
  const [locale, setLocaleState] = useState<Locale>(DEFAULT_LOCALE);

  useEffect(() => {
    const stored = window.localStorage.getItem(STORAGE_KEY);
    if (stored === 'th' || stored === 'en') {
      setLocaleState(stored);
    }
  }, []);

  const setLocale = (next: Locale) => {
    setLocaleState(next);
    window.localStorage.setItem(STORAGE_KEY, next);
  };

  return (
    <I18nContext.Provider value={{ locale, setLocale }}>
      {children}
    </I18nContext.Provider>
  );
}

function useI18n(): I18nContextValue {
  const ctx = useContext(I18nContext);
  if (!ctx) {
    throw new Error('useI18n must be used within I18nProvider');
  }
  return ctx;
}

export function useLocale(): I18nContextValue {
  return useI18n();
}

export function useT(): (key: string) => string {
  const { locale } = useI18n();
  return (key: string) => dictionaries[locale][key] ?? key;
}

export function LangToggle() {
  const { locale, setLocale } = useI18n();
  const next = locale === 'th' ? 'en' : 'th';
  const label = locale === 'th' ? 'EN' : 'TH';
  return (
    <button type="button" onClick={() => setLocale(next)}>
      {label}
    </button>
  );
}
