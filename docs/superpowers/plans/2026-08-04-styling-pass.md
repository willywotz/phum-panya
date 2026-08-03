# Styling Pass Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give the phum-panya frontend a warm, calm, readable design with Tailwind CSS v4 + shadcn/ui, light + dark themes, a real landing page, and a vendored Thai font — while keeping the single offline binary and all tests green.

**Architecture:** Tailwind v4 compiles to one static CSS file. shadcn copies components into `web/components/ui/`, so nothing loads from a CDN. `next.config` keeps `output: 'export'`, so `web/out/` still feeds `//go:embed`. The shared `CrudForm`/`CrudTable`/`IngredientEditor` components change once, so every resource inherits the new controls. Radix `Select`/`Dialog` change the accessible structure, so the affected Playwright specs move from `selectOption`/inline-confirm to role-based drivers.

**Tech Stack:** Next.js 15 (static export), React 19, Tailwind CSS v4, shadcn/ui (Radix), next-themes, @fontsource/noto-sans-thai, Playwright, Go 1.25 (`//go:embed`).

## Global Constraints

- **Offline only.** No CDN for CSS, JS, or fonts. Vendor everything (shadcn copies files; fonts come from `@fontsource` in `node_modules`). Verify with a grep for `http` URLs in fonts/styles.
- **Keep the single cgo-free binary.** `next.config` keeps `output: 'export'`. The build must still write `web/out/` for `//go:embed`. Do not add a runtime Node dependency.
- **Keep all tests green.** 107 Go tests (server, unchanged) + 7 Playwright e2e specs. Every task must end with the full e2e suite green.
- **i18n.** All user-facing text goes through `useT()` / the `lib/i18n` dictionary. Do not hard-code strings. Thai is the default locale.
- **Accessibility.** Keep semantic landmarks and accessible names. Keep `print.css` and its `.no-print` rule (FR-PRINT-1): the nav must stay hidden on print.
- **API/data model are out of scope.** Do not touch Go code, API routes, or field-spec data shapes — only the frontend render and the e2e specs.
- **Path alias.** `@/*` maps to `web/*` (already in `tsconfig.json`). shadcn aliases: `components` → `@/components`, `ui` → `@/components/ui`, `utils` → `@/lib/utils`.
- **Docs rule.** When the whole pass is done, update `CONTEXT.md`, then commit it (project rule).

---

### Task 1: Foundation — Tailwind v4 + shadcn init + Thai font

Sets up the toolchain with no visual behavior change yet, so every existing e2e spec must stay green.

**Files:**
- Create: `web/postcss.config.mjs`
- Create: `web/app/globals.css`
- Create: `web/lib/utils.ts`
- Create: `web/components.json`
- Modify: `web/package.json` (deps added by the CLIs — do not hand-edit versions)
- Modify: `web/app/layout.tsx` (import `globals.css` + font; keep `print.css`)

**Interfaces:**
- Produces: `cn(...inputs)` from `@/lib/utils` — the shadcn class-merge helper used by every `components/ui/*` file.
- Produces: `web/app/globals.css` as the single Tailwind entry (`@import "tailwindcss";`), imported once in the root layout.

- [ ] **Step 1: Install Tailwind v4 + PostCSS**

```bash
cd web
npm install -D tailwindcss@^4 @tailwindcss/postcss@^4
```

- [ ] **Step 2: Create the PostCSS config**

Create `web/postcss.config.mjs`:

```js
const config = {
  plugins: {
    '@tailwindcss/postcss': {},
  },
};

export default config;
```

- [ ] **Step 3: Create the Tailwind entry stylesheet**

Create `web/app/globals.css` (theme tokens are added in Task 2; this is the minimal valid v4 entry):

```css
@import 'tailwindcss';
```

- [ ] **Step 4: Initialize shadcn/ui**

React 19 triggers a peer-dependency prompt; pass `--force`. Accept detected Next.js App Router + Tailwind v4.

```bash
cd web
npx shadcn@latest init --force
```

When prompted, choose: base color **neutral**, CSS file `app/globals.css`, CSS variables **yes**. This writes `web/components.json`, `web/lib/utils.ts` (with `cn()`), and injects the shadcn `:root`/`.dark` variable blocks and `@theme inline` mapping into `web/app/globals.css`. Confirm `components.json` has `"rsc": false` and `"aliases": { "utils": "@/lib/utils", "ui": "@/components/ui", "components": "@/components" }`.

- [ ] **Step 5: Install and wire the Thai font (offline, vendored)**

```bash
cd web
npm install @fontsource/noto-sans-thai
```

Modify `web/app/layout.tsx` to import the font CSS and the Tailwind entry, and keep `print.css`. Set the page language and a base font family:

```tsx
import type { Metadata } from 'next';

import '@fontsource/noto-sans-thai/400.css';
import '@fontsource/noto-sans-thai/500.css';
import '@fontsource/noto-sans-thai/700.css';
import './globals.css';
import '../print.css';
import { I18nProvider } from '@/lib/i18n';

export const metadata: Metadata = {
  title: 'phum-panya',
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="th" suppressHydrationWarning>
      <body id="app" className="min-h-dvh bg-background text-foreground antialiased">
        <I18nProvider>{children}</I18nProvider>
      </body>
    </html>
  );
}
```

Add the font family to the `@theme inline` block in `globals.css` so Tailwind's `font-sans` uses it:

```css
@theme inline {
  --font-sans: 'Noto Sans Thai', ui-sans-serif, system-ui, sans-serif;
}
```

(`suppressHydrationWarning` is required for the Task 2 theme toggle; adding it now avoids a second edit.)

- [ ] **Step 6: Verify the build and static export still work**

Run: `cd web && npm run build`
Expected: build succeeds and writes `web/out/` (static export).

- [ ] **Step 7: Verify the existing e2e suite is still green**

Run: `cd web && npx playwright test`
Expected: all 7 specs PASS (no markup or accessible name changed yet).

- [ ] **Step 8: Commit**

```bash
rtk git add web/postcss.config.mjs web/app/globals.css web/lib/utils.ts web/components.json web/package.json web/package-lock.json web/app/layout.tsx web/components/ui
rtk git commit -m "chore(web): add Tailwind v4 + shadcn/ui + vendored Thai font"
```

---

### Task 2: Warm herbal theme + dark-mode toggle

Replaces the shadcn neutral tokens with the warm herbal palette and adds a working light/dark toggle in both navs.

**Files:**
- Modify: `web/app/globals.css` (override `:root` and `.dark` token values)
- Create: `web/lib/theme.tsx` (ThemeProvider wrapper + `ThemeToggle`)
- Modify: `web/app/layout.tsx` (wrap children in `ThemeProvider`)
- Modify: `web/lib/i18n.tsx` (add `theme`, `lightMode`, `darkMode` keys — Thai + English)
- Create: `web/e2e/theme-toggle.spec.ts`

**Interfaces:**
- Consumes: `cn` from `@/lib/utils`; the shadcn token blocks in `globals.css` from Task 1.
- Produces: `ThemeProvider` and `ThemeToggle` from `@/lib/theme`. `ThemeToggle` renders a `<button>` with accessible name from `t('theme')`; clicking it toggles the `dark` class on `<html>`.

- [ ] **Step 1: Install next-themes**

```bash
cd web && npm install next-themes
```

- [ ] **Step 2: Write the failing theme-toggle test**

Create `web/e2e/theme-toggle.spec.ts`:

```ts
import { test, expect } from '@playwright/test';

test('theme toggle switches between light and dark', async ({ page }) => {
  await page.goto('/');
  const html = page.locator('html');
  await expect(html).not.toHaveClass(/dark/);

  await page.getByRole('button', { name: 'ธีม' }).click();
  await expect(html).toHaveClass(/dark/);

  await page.getByRole('button', { name: 'ธีม' }).click();
  await expect(html).not.toHaveClass(/dark/);
});
```

- [ ] **Step 3: Run it to verify it fails**

Run: `cd web && npx playwright test e2e/theme-toggle.spec.ts`
Expected: FAIL — no button named `ธีม` exists yet.

- [ ] **Step 4: Add the i18n keys**

In `web/lib/i18n.tsx`, add to the `th` dictionary: `theme: 'ธีม'`. Add to the `en` dictionary: `theme: 'Theme'`. (Keep both dictionaries in sync — the file has parallel `th`/`en` objects.)

- [ ] **Step 5: Create the ThemeProvider and toggle**

Create `web/lib/theme.tsx`:

```tsx
'use client';

import { Moon, Sun } from 'lucide-react';
import { ThemeProvider as NextThemesProvider, useTheme } from 'next-themes';

import { Button } from '@/components/ui/button';
import { useT } from '@/lib/i18n';

export function ThemeProvider({ children }: { children: React.ReactNode }) {
  return (
    <NextThemesProvider attribute="class" defaultTheme="light" enableSystem={false}>
      {children}
    </NextThemesProvider>
  );
}

export function ThemeToggle() {
  const t = useT();
  const { resolvedTheme, setTheme } = useTheme();
  const next = resolvedTheme === 'dark' ? 'light' : 'dark';
  return (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      aria-label={t('theme')}
      onClick={() => setTheme(next)}
    >
      <Sun className="h-5 w-5 dark:hidden" />
      <Moon className="hidden h-5 w-5 dark:block" />
    </Button>
  );
}
```

- [ ] **Step 6: Add the shadcn Button used by the toggle**

```bash
cd web && npx shadcn@latest add button
```

- [ ] **Step 7: Wrap the app in ThemeProvider**

In `web/app/layout.tsx`, wrap the existing `I18nProvider` subtree:

```tsx
import { ThemeProvider } from '@/lib/theme';
// ...
<ThemeProvider>
  <I18nProvider>{children}</I18nProvider>
</ThemeProvider>
```

- [ ] **Step 8: Mount the toggle in both navs**

In `web/app/(public)/layout.tsx` `PublicNav` and `web/app/(staff)/staff/layout.tsx` `StaffNav`, import `ThemeToggle` from `@/lib/theme` and render `<ThemeToggle />` next to the existing `<LangToggle />`.

- [ ] **Step 9: Override the tokens with the warm herbal palette**

In `web/app/globals.css`, replace the `:root` and `.dark` token values that shadcn wrote with the warm herbal palette below (keep every variable name shadcn generated; only change values). Values are OKLCH.

```css
:root {
  --background: oklch(0.98 0.01 95);      /* warm off-white */
  --foreground: oklch(0.26 0.02 80);      /* warm near-black */
  --card: oklch(0.99 0.01 95);
  --card-foreground: var(--foreground);
  --popover: oklch(0.99 0.01 95);
  --popover-foreground: var(--foreground);
  --primary: oklch(0.52 0.09 150);        /* herbal green */
  --primary-foreground: oklch(0.98 0.01 95);
  --secondary: oklch(0.94 0.03 90);       /* warm sand */
  --secondary-foreground: oklch(0.30 0.03 80);
  --muted: oklch(0.95 0.02 90);
  --muted-foreground: oklch(0.50 0.02 80);
  --accent: oklch(0.75 0.11 70);          /* amber/clay */
  --accent-foreground: oklch(0.26 0.03 80);
  --destructive: oklch(0.55 0.20 25);
  --border: oklch(0.89 0.02 90);
  --input: oklch(0.89 0.02 90);
  --ring: oklch(0.52 0.09 150);
}

.dark {
  --background: oklch(0.22 0.01 80);
  --foreground: oklch(0.94 0.01 95);
  --card: oklch(0.26 0.01 80);
  --card-foreground: oklch(0.94 0.01 95);
  --popover: oklch(0.26 0.01 80);
  --popover-foreground: oklch(0.94 0.01 95);
  --primary: oklch(0.68 0.10 150);
  --primary-foreground: oklch(0.20 0.02 80);
  --secondary: oklch(0.32 0.02 80);
  --secondary-foreground: oklch(0.94 0.01 95);
  --muted: oklch(0.32 0.02 80);
  --muted-foreground: oklch(0.70 0.02 85);
  --accent: oklch(0.70 0.10 70);
  --accent-foreground: oklch(0.20 0.02 80);
  --destructive: oklch(0.62 0.19 25);
  --border: oklch(0.36 0.02 80);
  --input: oklch(0.36 0.02 80);
  --ring: oklch(0.68 0.10 150);
}
```

- [ ] **Step 10: Run the theme-toggle test to verify it passes**

Run: `cd web && npx playwright test e2e/theme-toggle.spec.ts`
Expected: PASS.

- [ ] **Step 11: Run the full e2e suite**

Run: `cd web && npx playwright test`
Expected: all 8 specs PASS.

- [ ] **Step 12: Commit**

```bash
rtk git add web/app/globals.css web/lib/theme.tsx web/app/layout.tsx web/lib/i18n.tsx web/app/'(public)'/layout.tsx web/app/'(staff)'/staff/layout.tsx web/components/ui web/e2e/theme-toggle.spec.ts web/package.json web/package-lock.json
rtk git commit -m "feat(web): warm herbal theme + light/dark toggle"
```

---

### Task 3: Landing page

Replaces the bare `welcome` heading with a hero and three navigation cards — the QR visitor's entry point.

**Files:**
- Modify: `web/app/(public)/page.tsx`
- Modify: `web/lib/i18n.tsx` (add `tagline`, `browseDoctors`, `browseRecipes`, `browseHerbs`)
- Create: `web/e2e/landing.spec.ts`

**Interfaces:**
- Consumes: shadcn `Card`, `Button`; `useT()`.
- Produces: a home page (`/`) with an `<h1>` and three links whose `href`s are `/doctors`, `/recipes`, `/herbs`.

- [ ] **Step 1: Write the failing landing test**

Create `web/e2e/landing.spec.ts`:

```ts
import { test, expect } from '@playwright/test';

test('landing page shows hero and three browse links', async ({ page }) => {
  await page.goto('/');
  await expect(page.getByRole('heading', { level: 1 })).toBeVisible();

  await expect(page.getByRole('link', { name: 'หมอพื้นบ้าน' })).toHaveAttribute('href', '/doctors');
  await expect(page.getByRole('link', { name: 'ตำรับยา' })).toHaveAttribute('href', '/recipes');
  await expect(page.getByRole('link', { name: 'สมุนไพร' })).toHaveAttribute('href', '/herbs');
});
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd web && npx playwright test e2e/landing.spec.ts`
Expected: FAIL — no such links on `/`.

- [ ] **Step 3: Add the shadcn Card**

```bash
cd web && npx shadcn@latest add card
```

- [ ] **Step 4: Add the i18n keys**

In `web/lib/i18n.tsx`, add to `th`: `tagline: 'ทะเบียนตำรายาหมอพื้นบ้าน'`; add to `en`: `tagline: 'Folk-medicine registry'`. (`doctors`, `recipes`, `herbs` keys already exist and are reused as the card titles/link names.)

- [ ] **Step 5: Build the landing page**

Replace `web/app/(public)/page.tsx`:

```tsx
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
```

- [ ] **Step 6: Run the landing test to verify it passes**

Run: `cd web && npx playwright test e2e/landing.spec.ts`
Expected: PASS.

- [ ] **Step 7: Run the full e2e suite**

Run: `cd web && npx playwright test`
Expected: all 9 specs PASS.

- [ ] **Step 8: Commit**

```bash
rtk git add web/app/'(public)'/page.tsx web/lib/i18n.tsx web/components/ui web/e2e/landing.spec.ts
rtk git commit -m "feat(web): landing page with hero and browse cards"
```

---

### Task 4: Base layout, nav bars, and login styling

Styles the shared chrome (nav bars, page container) and the login page. No accessible name changes, so all existing specs stay green.

**Files:**
- Modify: `web/app/(public)/layout.tsx`
- Modify: `web/app/(staff)/staff/layout.tsx`
- Modify: `web/app/(staff)/login/page.tsx`
- Modify: `web/components/PhotoUpload.tsx` (Input/Label/Button styling only)

**Interfaces:**
- Consumes: shadcn `Button`, `Input`, `Label`; `cn`.
- Produces: no new exported symbols. Nav still renders the same links and buttons with the same accessible names.

- [ ] **Step 1: Add shadcn Input and Label**

```bash
cd web && npx shadcn@latest add input label
```

- [ ] **Step 2: Style the public nav**

In `web/app/(public)/layout.tsx`, wrap `<nav>` content in a styled bar. Keep `className="no-print"`, keep `aria-label={t('publicNav')}`, keep each `<Link>` text and the `signIn` button text unchanged:

```tsx
<nav
  className="no-print flex items-center gap-4 border-b bg-card px-4 py-3"
  aria-label={t('publicNav')}
>
  <div className="mr-auto flex items-center gap-4">
    {navLinks.map(({ href, key }) => (
      <Link key={href} href={href} className="text-sm font-medium hover:text-primary">
        {t(key)}
      </Link>
    ))}
  </div>
  <Button type="button" variant="outline" size="sm" onClick={() => router.push('/login')}>
    {t('signIn')}
  </Button>
  <LangToggle />
  <ThemeToggle />
</nav>
```

- [ ] **Step 3: Style the staff nav**

In `web/app/(staff)/staff/layout.tsx`, apply the same bar treatment to `StaffNav` (keep `aria-label={t('staffNav')}`, keep all link texts, keep `<LangToggle />` and `<ThemeToggle />`). Wrap the authenticated content in a page container:

```tsx
return (
  <RequireStaff>
    <StaffNav />
    <div className="mx-auto max-w-5xl px-4 py-6">{children}</div>
  </RequireStaff>
);
```

Apply the same container wrapper around `{children}` in the public layout.

- [ ] **Step 4: Style the login page**

In `web/app/(staff)/login/page.tsx`, swap the raw `<input>`/`<label>`/`<button>` for shadcn `Input`, `Label`, `Button`. Keep `htmlFor`/`id` pairing (`email`, `password`), keep the button text `t('signIn')`, keep `<p role="alert">`. Center it:

```tsx
<main className="mx-auto flex min-h-dvh max-w-sm flex-col justify-center gap-4 px-4">
  <div className="flex justify-end gap-2">
    <LangToggle />
    <ThemeToggle />
  </div>
  <h1 className="text-2xl font-bold">{t('signIn')}</h1>
  <form onSubmit={handleSubmit} className="flex flex-col gap-4">
    <div className="grid gap-1.5">
      <Label htmlFor="email">{t('email')}</Label>
      <Input id="email" type="email" value={email} required
        onChange={(event) => setEmail(event.target.value)} />
    </div>
    <div className="grid gap-1.5">
      <Label htmlFor="password">{t('password')}</Label>
      <Input id="password" type="password" value={password} required
        onChange={(event) => setPassword(event.target.value)} />
    </div>
    {error && <p role="alert" className="text-sm text-destructive">{t('loginError')}</p>}
    <Button type="submit" disabled={submitting}>{t('signIn')}</Button>
  </form>
</main>
```

Import `ThemeToggle` from `@/lib/theme`. `getByLabel('อีเมล')` / `getByLabel('รหัสผ่าน')` still resolve because `Input`/`Label` keep the `id`/`htmlFor` pairing.

- [ ] **Step 5: Style PhotoUpload controls**

In `web/components/PhotoUpload.tsx`, swap the `<label>`/`<input type="file">`/upload `<button>` for shadcn `Label`/`Input`/`Button`. Keep the `htmlFor="photo-upload"`/`id="photo-upload"` pairing, keep the `<progress>` element, and add `aria-label={t('storage')}` to `<progress>` (fixes the HANDOFF §8 missing aria-label). Do not change any accessible names the specs rely on.

- [ ] **Step 6: Run the full e2e suite**

Run: `cd web && npx playwright test`
Expected: all 9 specs PASS (login, staff-flow, uat, export-a11y unaffected — names unchanged).

- [ ] **Step 7: Commit**

```bash
rtk git add web/app/'(public)'/layout.tsx web/app/'(staff)'/staff/layout.tsx web/app/'(staff)'/login/page.tsx web/components/PhotoUpload.tsx web/components/ui
rtk git commit -m "feat(web): style nav bars, page container, login, photo upload"
```

---

### Task 5: CrudForm — shadcn controls + Radix Select

Migrates the shared form used by every staff resource. Native `<select>` becomes Radix `Select`, which breaks `selectOption` in `staff-flow` and `uat`; those specs move to a role-based helper in the same task so the suite stays green.

**Files:**
- Modify: `web/components/CrudForm.tsx`
- Create: `web/e2e/fixtures/select.ts` (Radix select helper)
- Modify: `web/e2e/staff-flow.spec.ts`
- Modify: `web/e2e/uat.spec.ts`

**Interfaces:**
- Consumes: shadcn `Select`, `Checkbox`, `Input`, `Label`, `Textarea`, `Button`; `cn`.
- Produces: `selectByName(page, name, optionLabel)` from `@/e2e/fixtures/select` — clicks a Radix `combobox` by accessible name, then clicks the `option` by visible label. Radix `SelectTrigger` in `CrudForm` carries `aria-label={t(field.labelKey)}`, so `getByRole('combobox', { name })` resolves. The `multiselect` field type stays a native styled `<select multiple>` (Radix has no multiselect primitive), so `getByLabel(...).selectOption(...)` still works for it.

- [ ] **Step 1: Add the shadcn Select, Checkbox, and Textarea**

```bash
cd web && npx shadcn@latest add select checkbox textarea
```

- [ ] **Step 2: Write the Radix select test helper**

Create `web/e2e/fixtures/select.ts`:

```ts
import { type Page, expect } from '@playwright/test';

// Drives a shadcn/Radix Select: open the combobox by its accessible name,
// then pick the option by its visible label.
export async function selectByName(page: Page, name: string, optionLabel: string) {
  await page.getByRole('combobox', { name }).click();
  await page.getByRole('option', { name: optionLabel }).click();
  await expect(page.getByRole('combobox', { name })).toContainText(optionLabel);
}
```

- [ ] **Step 3: Update staff-flow.spec select calls (write the failing form)**

In `web/e2e/staff-flow.spec.ts`, import the helper and replace each `getByLabel(...).selectOption({ label })` that targets a `CrudForm` select with `selectByName(page, name, label)`. Map by accessible name:
- `getByLabel('บทบาท').selectOption({ label: 'ผู้แก้ไขข้อมูลอำเภอ' })` → `selectByName(page, 'บทบาท', 'ผู้แก้ไขข้อมูลอำเภอ')`
- the user-district `getByLabel('อำเภอ').selectOption({ label: districtName })` → `selectByName(page, 'อำเภอ', districtName)`
- `getByLabel('สถานะ').selectOption({ label: 'ใช้งาน' })` → `selectByName(page, 'สถานะ', 'ใช้งาน')`
- `getByLabel('ผลการรักษา').selectOption({ label: 'หายขาด' })` → `selectByName(page, 'ผลการรักษา', 'หายขาด')`

Leave the IngredientEditor selects (`herbSelect`, `ingredientRow2` สมุนไพร, `pendingSelect`, `จับคู่กับสมุนไพร`) unchanged for now — they are migrated in Task 7.

- [ ] **Step 4: Run staff-flow to verify it fails**

Run: `cd web && npx playwright test e2e/staff-flow.spec.ts`
Expected: FAIL — `getByRole('combobox', { name })` finds nothing (form still renders native `<select>`).

- [ ] **Step 5: Migrate CrudForm's renderInput**

In `web/components/CrudForm.tsx`, replace the raw controls in `renderInput` with shadcn equivalents. Keep the `id`/label wiring for text-like inputs. For `select`, render Radix `Select` with `aria-label`. Keep `multiselect` as a native `<select multiple>` with Tailwind classes. Replace the field wrapper and submit/cancel buttons with shadcn `Button`.

```tsx
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Checkbox } from '@/components/ui/checkbox';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { Button } from '@/components/ui/button';
```

`select` case:

```tsx
case 'select':
  return (
    <Select
      value={String(values[name] ?? '')}
      required={required}
      onValueChange={(value) => setField(name, value)}
    >
      <SelectTrigger id={name} aria-label={t(field.labelKey)}>
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {options?.map((option) => (
          <SelectItem key={option.value} value={option.value}>
            {optionLabel(option, t)}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
```

`checkbox` case:

```tsx
case 'checkbox':
  return (
    <Checkbox
      id={name}
      checked={Boolean(values[name])}
      onCheckedChange={(checked) => setField(name, checked === true)}
    />
  );
```

`textarea` case → shadcn `Textarea` with the same `id`/`value`/`onChange`. `text`/`number`/`date`/`password` cases → shadcn `Input` with the matching `type` and the same `id`/`value`/`required`/`onChange`. Keep `multiselect` as the existing native `<select multiple>` but add `className="w-full rounded-md border bg-background p-2"`.

Note on focus: Radix `SelectTrigger` and `Checkbox` are not `HTMLElement`-focus-compatible with the existing `firstFieldRef` typed `HTMLElement | null`. Keep the `autoFocusRef` on the `text`/`textarea`/`input` cases; when field index 0 is a `select`/`checkbox`, drop the ref (the district/herb create forms all start with a text field, so first-field focus behavior — asserted by `crud.spec` on the district name — is preserved). Verify `crud.spec`'s `toBeFocused()` assertion still passes in Step 7.

Wrap each field:

```tsx
<div key={field.name} className="grid gap-1.5">
  <Label htmlFor={field.name}>{t(field.labelKey)}</Label>
  {renderInput(...)}
</div>
```

- [ ] **Step 6: Run staff-flow to verify it passes**

Run: `cd web && npx playwright test e2e/staff-flow.spec.ts`
Expected: PASS.

- [ ] **Step 7: Update and run uat.spec, then the full suite**

Apply the same `selectOption` → `selectByName` replacements in `web/e2e/uat.spec.ts` for the `CrudForm` selects (`บทบาท`, user/doctor `อำเภอ` in forms, `สถานะ`, `ผลการรักษา`). Leave the public-filter `อำเภอ` (line ~191) and the IngredientEditor/`จับคู่` selects for Task 7.

Run: `cd web && npx playwright test`
Expected: all 9 specs PASS.

- [ ] **Step 8: Commit**

```bash
rtk git add web/components/CrudForm.tsx web/components/ui web/e2e/fixtures/select.ts web/e2e/staff-flow.spec.ts web/e2e/uat.spec.ts
rtk git commit -m "feat(web): CrudForm uses shadcn inputs and Radix Select"
```

---

### Task 6: CrudTable — shadcn Table + Dialog form + AlertDialog delete

Styles the resource table and moves the form into a modal `Dialog` and the delete confirm into an `AlertDialog`. This changes the delete flow that `crud.spec` drives.

**Files:**
- Modify: `web/components/CrudTable.tsx`
- Modify: `web/lib/i18n.tsx` (reuse existing `confirmDelete`/`yes`/`no`; no new keys required)
- Modify: `web/e2e/crud.spec.ts`

**Interfaces:**
- Consumes: shadcn `Table`, `Dialog`, `AlertDialog`, `Button`; `CrudForm` (unchanged interface).
- Produces: the delete confirm as a Radix `alertdialog` containing a confirm button named `t('yes')` and a cancel button named `t('no')`. The add/edit form renders inside a `Dialog` whose open state replaces the old `editing !== null` inline block.

- [ ] **Step 1: Add shadcn Table, Dialog, AlertDialog**

```bash
cd web && npx shadcn@latest add table dialog alert-dialog
```

- [ ] **Step 2: Update crud.spec delete flow (write the failing form)**

In `web/e2e/crud.spec.ts`, replace the inline two-step delete with the alertdialog flow. The row's `ลบ` button opens the dialog; the confirm `ใช่` lives in the dialog:

```ts
// Delete, via the confirmation dialog.
await updatedRow.getByRole('button', { name: 'ลบ' }).click();
const dialog = page.getByRole('alertdialog');
await dialog.getByRole('button', { name: 'ใช่' }).click();
await expect(page.getByRole('row', { name: new RegExp(updatedName) })).toHaveCount(0);
```

The add/edit assertions do not change: the form fields are still reachable by label because `CrudForm` renders inside the dialog. Keep the `toBeFocused()` check on `ชื่อ`.

- [ ] **Step 3: Run crud.spec to verify it fails**

Run: `cd web && npx playwright test e2e/crud.spec.ts`
Expected: FAIL — no `alertdialog` role yet (delete still inline).

- [ ] **Step 4: Migrate CrudTable**

In `web/components/CrudTable.tsx`:
- Replace the `<table>` block with shadcn `Table`/`TableHeader`/`TableRow`/`TableHead`/`TableBody`/`TableCell`. Keep the header cells rendering `t(labelKeyFor(...))` and `t('actions')`, and keep each data row as a `TableRow` so `getByRole('row', { name })` / `getByRole('cell', { name })` still match.
- Wrap the add/edit `CrudForm` in a `Dialog` controlled by `editing !== null`; render `CrudForm` inside `DialogContent`. On `onCancel`/`onDone`, close the dialog (`setEditing(null)`) — keep the existing `triggerRef` focus-return logic via `onOpenChange`.
- Replace the inline delete confirm span with an `AlertDialog` per row (or one shared dialog keyed by `confirmingId`). The trigger is the row `ลบ` button; the dialog body shows `t('confirmDelete')` with `t('yes')` (calls `handleDelete(row)`) and `t('no')` (closes). Keep the `role="alert"` mismatch `<p>` as-is.

Keep `<section>`/`<h2>` and the top-level `เพิ่ม` (add) button outside the dialog.

- [ ] **Step 5: Run crud.spec to verify it passes**

Run: `cd web && npx playwright test e2e/crud.spec.ts`
Expected: PASS.

- [ ] **Step 6: Run the full suite**

Run: `cd web && npx playwright test`
Expected: all 9 specs PASS. If `staff-flow`/`uat` open the form and now expect a dialog, confirm their `เพิ่ม`→fill→`บันทึก` sequences still pass (fields are reachable by label inside the dialog). Fix any dialog-scoping assertion inline.

- [ ] **Step 7: Commit**

```bash
rtk git add web/components/CrudTable.tsx web/e2e/crud.spec.ts web/components/ui
rtk git commit -m "feat(web): CrudTable uses shadcn Table, Dialog form, AlertDialog delete"
```

---

### Task 7: IngredientEditor + public filters → Radix Select and Card lists

Migrates the remaining native selects (recipe ingredient rows, public doctor/recipe filters) and styles the public list pages as cards.

**Files:**
- Modify: `web/components/IngredientEditor.tsx`
- Modify: `web/app/(public)/doctors/page.tsx`
- Modify: `web/app/(public)/recipes/page.tsx`
- Modify: `web/app/(public)/herbs/page.tsx`
- Modify: `web/app/(public)/doctor/page.tsx`
- Modify: `web/e2e/public.spec.ts`
- Modify: `web/e2e/staff-flow.spec.ts` (ingredient selects)
- Modify: `web/e2e/uat.spec.ts` (ingredient + public-filter selects)

**Interfaces:**
- Consumes: shadcn `Select`, `Card`, `Input`, `Button`; `selectByName` from `@/e2e/fixtures/select`.
- Produces: every remaining Radix `SelectTrigger` carries `aria-label` equal to its visible label text, so `selectByName` resolves it. Public filters select by option label (district/herb name), not by numeric value.

- [ ] **Step 1: Update public.spec + remaining staff-flow/uat select calls (write the failing form)**

In `web/e2e/public.spec.ts`:
- `getByLabel('อำเภอ').selectOption({ value: String(districtId) })` → `selectByName(page, 'อำเภอ', districtName)` (select by the district's visible name; the spec already creates/knows it).
- `getByLabel('Herb').selectOption({ value: String(herbId) })` → `selectByName(page, 'Herb', herbName)`.

In `web/e2e/staff-flow.spec.ts` and `web/e2e/uat.spec.ts`, convert the IngredientEditor and reconcile selects to `selectByName`:
- herb select → `selectByName(page, 'สมุนไพร', herbName)` (scope per row if two rows exist — see Step 4 note)
- `ingredientRow2` สมุนไพร → `selectByName` scoped to the second row
- `pendingSelect` → `selectByName(page, 'สมุนไพร', pendingHerbName)`
- `จับคู่กับสมุนไพร` → `selectByName(page, 'จับคู่กับสมุนไพร', herbName)`
- the public-filter `อำเภอ` in `uat.spec` (~line 191) and herb filter (~198) → `selectByName` by name.

Import `selectByName` in each spec.

- [ ] **Step 2: Run the affected specs to verify they fail**

Run: `cd web && npx playwright test e2e/public.spec.ts`
Expected: FAIL — filters still render native `<select>`.

- [ ] **Step 3: Migrate the public filter pages**

In `web/app/(public)/doctors/page.tsx`, replace the search `<input>`/`<button>` with shadcn `Input`/`Button`, and the district `<select>` with Radix `Select` (`SelectTrigger` `aria-label={t('district')}`, options = districts by name, plus an "all districts" item using `t('allDistricts')` with a sentinel value like `'all'` mapped to `''` in state — Radix `SelectItem` cannot use an empty-string value). Render the doctor list as `Card`s. Do the same filter treatment in `web/app/(public)/recipes/page.tsx` (herb filter). Style `herbs` and `doctor` (detail) pages with the container + `Card`/typography; they have no selects.

Radix empty-value note: keep local state `''` for "all", but map to/from the sentinel:

```tsx
<Select value={districtId || 'all'} onValueChange={(v) => setDistrictId(v === 'all' ? '' : v)}>
  <SelectTrigger aria-label={t('district')}><SelectValue /></SelectTrigger>
  <SelectContent>
    <SelectItem value="all">{t('allDistricts')}</SelectItem>
    {districts.map((d) => <SelectItem key={d.id} value={String(d.id)}>{d.name}</SelectItem>)}
  </SelectContent>
</Select>
```

- [ ] **Step 4: Migrate IngredientEditor selects**

In `web/components/IngredientEditor.tsx`, replace each row's herb `<select>` and the amount/unit/note `<input>`s with Radix `Select` and shadcn `Input`. Give the herb `SelectTrigger` `aria-label={t('herb')}` (`สมุนไพร`). Because multiple ingredient rows each have a `สมุนไพร` select, the spec scopes by row; keep each row in a container the test can target (the specs already locate `ingredientRow2`). Preserve the `PENDING` sentinel option and the `emptyIngredientRow`/`toIngredientPayload` logic unchanged.

For the per-row helper, add a row-scoped variant usage in the specs:

```ts
// within a located row element `row`:
await row.getByRole('combobox', { name: 'สมุนไพร' }).click();
await page.getByRole('option', { name: herbName }).click();
```

(Radix renders the listbox in a portal at the page root, so the option click is page-scoped, not row-scoped — reflect this in the spec edits.)

- [ ] **Step 5: Run public.spec to verify it passes**

Run: `cd web && npx playwright test e2e/public.spec.ts`
Expected: PASS.

- [ ] **Step 6: Run the full suite**

Run: `cd web && npx playwright test`
Expected: all 9 specs PASS.

- [ ] **Step 7: Commit**

```bash
rtk git add web/components/IngredientEditor.tsx web/app/'(public)' web/e2e/public.spec.ts web/e2e/staff-flow.spec.ts web/e2e/uat.spec.ts web/components/ui
rtk git commit -m "feat(web): Radix Select for ingredients + public filters, card lists"
```

---

### Task 8: Final polish, verification gate, and docs

Sweeps spacing/typography, confirms the print rule and offline constraint, runs the full gate across web + Go, and updates project docs.

**Files:**
- Modify: `web/app/globals.css` (optional base typography: heading sizes, table spacing)
- Modify: `web/print.css` (confirm `.no-print` still hides the nav; add nav `.no-print` class if a nav lost it)
- Modify: `CONTEXT.md`
- Modify: `docs/HANDOFF.md` (update §8 — the "no CSS" gap is now closed)

**Interfaces:**
- Consumes: everything from Tasks 1–7.
- Produces: a fully styled app, all gates green, docs updated.

- [ ] **Step 1: Confirm the print rule**

Check that the public `<nav>` still has `className="no-print"` (Task 4 kept it). If the staff nav should also hide on print, add `no-print`. 

Run: `cd web && npx playwright test e2e/export-a11y.spec.ts`
Expected: PASS (axe accessibility checks on export pages).

- [ ] **Step 2: Base typography polish**

In `web/app/globals.css`, add minimal base rules under a `@layer base` block for `h1/h2` sizes and `table` cell padding if any page looks cramped. Keep it small; shadcn components already carry most styling.

- [ ] **Step 3: Verify no CDN references**

Run: `cd web && grep -rIn "https://" app components lib print.css | grep -iE "font|css|cdn|googleapis"`
Expected: no matches (fonts come from `@fontsource` in `node_modules`; shadcn files are local).

- [ ] **Step 4: Full web build + static export**

Run: `cd web && npm run build`
Expected: build succeeds; `web/out/` is written.

- [ ] **Step 5: Go build embeds the UI**

Run: `go build ./...`
Expected: success (the binary embeds `web/out/`).

- [ ] **Step 6: Go tests**

Run: `rtk go test ./...`
Expected: all 107 Go tests PASS.

- [ ] **Step 7: Full e2e suite**

Run: `cd web && npx playwright test`
Expected: all 9 specs PASS.

- [ ] **Step 8: Update docs**

- `CONTEXT.md`: record that the styling pass is done (Tailwind v4 + shadcn, warm herbal theme, dark toggle, landing page, vendored Thai font).
- `docs/HANDOFF.md` §8: the "NO visual styling / CSS" gap is closed; note the new stack row (Tailwind + shadcn) and that dark mode + a landing page now exist.

- [ ] **Step 9: Commit**

```bash
rtk git add web/app/globals.css web/print.css CONTEXT.md docs/HANDOFF.md
rtk git commit -m "docs: styling pass complete; polish + verification"
```

---

## Self-Review

**Spec coverage** (against `docs/superpowers/specs/2026-08-04-styling-pass-design.md`):
- §4 stack (Tailwind v4, shadcn, offline, cgo-free) → Task 1 + Global Constraints.
- §5 theme (warm herbal light/dark, vendored font, next-themes toggle) → Tasks 1–2.
- §6 component migration (Button/Input/Label/Select/Dialog/Checkbox/Table/Card/nav) → Tasks 2–7.
- §7 landing page → Task 3.
- §8 e2e updates (combobox/option, alertdialog) → Tasks 5–7 (helper in 5).
- §9 responsive + a11y + keep print.css → Tasks 3/4/8.
- §10 verification gate (npm build, go build, go test, playwright, no-CDN grep) → Task 8.
- §11 out of scope (backend/API/data model, HANDOFF minors) → respected; only the consent Checkbox and the `<progress>` aria-label are touched, both as natural side effects (allowed by the spec).

**Placeholder scan:** No "TBD/TODO". Where a step lists mechanical repetition across many `selectOption` call sites (Tasks 5/7), the exact old→new mapping and the shared `selectByName` helper are given, so the engineer has the full pattern.

**Type consistency:** `selectByName(page, name, optionLabel)` is defined in Task 5 and consumed unchanged in Tasks 5/6/7. `cn` from `@/lib/utils` (Task 1). `ThemeProvider`/`ThemeToggle` from `@/lib/theme` (Task 2) mounted in Tasks 2/4. shadcn component imports use the `@/components/ui/*` alias fixed in Task 1's `components.json`. The `multiselect` field type is explicitly kept native in Task 5 and never routed through `selectByName`.
