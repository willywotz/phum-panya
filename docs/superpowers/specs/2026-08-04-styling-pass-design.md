# Styling Pass — Design

Date: 2026-08-04 · Branch: `feat/styling-pass` (off `feat/p1-launch`)

Written in ASD-STE100 Simplified Technical English.

## 1. Purpose

The frontend is clean semantic HTML with almost no CSS. Only `print.css` is present.
The app is functional and accessible, but it looks bare. This pass gives the app a warm,
calm, readable look. It keeps the app as one offline binary.

## 2. Goal and non-goal

Goal:
- Add a visual design to all frontend pages, public and staff.
- Keep the app self-hosted and offline. Do not use a CDN.
- Keep the single cgo-free binary and the `//go:embed` build.
- Keep all tests green: 107 Go tests and 7 Playwright e2e specs.

Non-goal:
- Do not change the backend, the API routes, or the data model.
- Do not do the deferred minor items in `docs/HANDOFF.md` §8.
  Exception: fix an item only when a component swap fixes it (example: the consent
  Checkbox).

## 3. Decisions (agreed with the owner)

| Topic | Decision |
|---|---|
| CSS approach | Tailwind CSS v4 + shadcn/ui |
| Component depth | Full shadcn. Use Radix `Select`, `Dialog`, `Checkbox`. Update the e2e specs. |
| Theme | Warm herbal / earthy. Herbal-green primary, amber/clay accent, warm off-white background. |
| Dark mode | Light and dark. Add a toggle. |
| Landing page | Build a real landing page. |
| Thai font | Add a Thai webfont (Noto Sans Thai, and Sarabun as an option). Self-host it. |

## 4. Stack and offline rule

- **Tailwind v4.** Use `@import "tailwindcss";` in `app/globals.css`. Use
  `@tailwindcss/postcss` in `postcss.config.mjs`. Tailwind v4 has no `tailwind.config.*` file.
  All theme tokens live in CSS.
- **shadcn/ui.** Run `npx shadcn init`. React 19 shows a peer-dependency prompt; select
  `--force`. shadcn **copies** each component into `web/components/ui/`. No component loads
  from a CDN.
- **Build output.** Tailwind compiles to one static CSS file. `next.config` keeps
  `output: 'export'`. The build still writes `web/out/` for `//go:embed`. The binary stays
  cgo-free and unchanged.
- New files from init: `web/lib/utils.ts` (the `cn()` helper), `web/components.json`
  (`rsc: false`, `baseColor: neutral`, css → `app/globals.css`, alias `@/*`). The `@/*` alias
  is already in `tsconfig.json`.

## 5. Theme

- `app/globals.css` holds the CSS variables. It defines a **warm herbal light** palette and a
  **dark** palette under the `.dark` class. `@theme inline` maps the variables to the shadcn
  tokens.
- **Fonts.** Self-host Noto Sans Thai (UI) with `next/font/local`. Sarabun is an option for
  body text. Do not use the Google Fonts CDN. Use a larger base text size for readability.
- **Dark toggle.** Use `next-themes`. It runs on the client and works with static export. Add
  `suppressHydrationWarning` to the `<html>` element. Put a sun/moon toggle in both navs, next
  to the existing `LangToggle`.

## 6. Component migration

Add these shadcn components. Swap the raw markup once in the shared components, so every
resource gets the change.

| Now | Becomes |
|---|---|
| `<button>` | `Button` (renders a native `<button>`) |
| `<input>` and `<label>` | `Input` and `Label` |
| native `<select>` (doctor filter, `CrudForm`) | shadcn `Select` (Radix) |
| inline delete-confirm span and the `CrudForm` block | `AlertDialog` (delete) and `Dialog` (form) |
| the consent boolean field | `Checkbox` |
| `<table>` in `CrudTable` | shadcn `Table` |
| doctor / recipe / herb lists | `Card` |
| nav | styled bar, `Button`-as-link, plus the two toggles |

`CrudForm` and `CrudTable` build from field specs. The `Select`, `Checkbox`, and `Dialog`
swaps happen one time in these shared components. Each resource inherits them.

## 7. Landing page

Replace the bare `welcome` heading on `app/(public)/page.tsx` with:
- A hero. Show the project name (ตำรายาหมอพื้นบ้าน) and a one-line purpose.
- Three `Card` links: Doctors, Recipes, Herbs. This is the entry point for a QR visitor.

Keep all text in the i18n system. Do not hard-code strings.

## 8. e2e test updates

Full shadcn changes the accessible structure of some controls:
- Radix `Select` is a `combobox` with a `listbox`. It is not a native `<select>`.
- The delete confirm becomes an `alertdialog`.

Update the affected specs to drive the controls by role:
- `getByRole('combobox')`, then `getByRole('option')`.
- `getByRole('alertdialog')` for delete confirm.

Keep the accessible names the same, so the change stays small. The Go tests do not change,
because they test the server.

## 9. Responsive and accessibility

- Design for a small screen first. Most visitors arrive by QR on a phone.
- Use a readable container width on large screens.
- Keep semantic landmarks and labels. Radix components are accessible by default.
- Keep `print.css`. The `.no-print` rule must still hide the nav on print (FR-PRINT-1).

## 10. Verification gate

Run these steps in order. Each step must pass before the work is complete:
1. `cd web && npm run build` — static export succeeds; `web/out/` is written.
2. `go build ./...` — the binary embeds `web/out/`.
3. `go test ./...` — all Go tests are green.
4. `cd web && npx playwright test` — all 7 e2e specs are green.
5. Check that no file references a CDN URL for CSS, JS, or fonts.

## 11. Out of scope

- Backend, API routes, data model.
- The deferred minor items in `docs/HANDOFF.md` §8.
- The later paid phases P2–P5 in the SRS.
