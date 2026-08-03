'use client';

import Link from 'next/link';

import { Card, CardHeader, CardTitle } from '@/components/ui/card';
import { useT } from '@/lib/i18n';

const cards = [
  { href: '/doctors', key: 'doctors' },
  { href: '/recipes', key: 'recipes' },
  { href: '/herbs', key: 'herbs' },
] as const;

export default function HomePage() {
  const t = useT();
  return (
    <main className="mx-auto max-w-3xl px-4 py-10">
      <section className="mb-10 text-center">
        <h1 className="text-3xl font-bold text-primary">{t('welcome')}</h1>
        <p className="mt-2 text-muted-foreground">{t('tagline')}</p>
      </section>
      <div className="grid gap-4 sm:grid-cols-3">
        {cards.map(({ href, key }) => (
          <Link key={href} href={href} className="block">
            <Card className="h-full transition-colors hover:border-primary">
              <CardHeader>
                <CardTitle>{t(key)}</CardTitle>
              </CardHeader>
            </Card>
          </Link>
        ))}
      </div>
    </main>
  );
}
