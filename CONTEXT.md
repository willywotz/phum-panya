# CONTEXT

## Project

A web app to collect folk-medicine records (ตำรายาหมอพื้นบ้าน) for one province,
grouped by district (อำเภอ). The public reads all data. One editor per district plus a
central admin write the data. No mobile-store app; users open the site by QR scan. No map.

Client chat: `messages.txt`.
Reference app (client-forwarded): a Thai "Tok Bidan" herbal app, but without the map.

## Status

- Design phase.
- Data model + standard fill-in form: designed and approved.
  See `docs/superpowers/specs/2026-08-03-data-model-and-form-design.md`.
- Feature research: done. Data model has no missing fields; gaps are operational/ethical.
  See `docs/research/2026-08-03-feature-research.md`.
  Top adds: healer consent + attribution, edit history, print/PDF, bulk export, feedback form.
- Paranoid review (fresh-context verifier): done. Fixes applied to model, form, and research:
  - Consent/PDPA: added `consent_obtained`/`consent_date` on Doctor, opt-out rule, form
    consent box, and a "must-do before launch" section (spec §3.1).
  - Record linking: added `code` on Doctor and Recipe; the form links by code, not name.
  - Year rule: one Doctor row per healer with `first_year`; Recipe/Case keep `data_year` (§3.2).
  - Herb fallback: a pending-herb name so district entry is never blocked.
  - Research file: corrected the backup cost to real scale; fixed 3 mislabeled citations.
- Next step: write the implementation plan (writing-plans skill).

## Data model (summary)

Six records: District, User, Doctor, Herb (shared catalog), Recipe, Case.

```
District ──< Doctor ──< Recipe ──< Case
                          └──< Ingredient >── Herb
```

- Case links to one Recipe. Patient is anonymous.
- Ingredient uses a decimal amount plus a unit, and links to the shared Herb catalog
  (or a pending-herb name when the herb is not in the catalog yet).
- Doctor and Recipe carry a short `code` for reliable linking on the paper form.
- Doctor is one row per healer (`first_year`); Recipe/Case carry `data_year`.
- Doctor needs consent before it goes public. Year locking is a later paid feature.

