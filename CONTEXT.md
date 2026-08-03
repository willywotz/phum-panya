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
- Price proposal (3 tiers): English draft + Thai version ready.
  See `docs/proposals/2026-08-03-price-proposal-en.md` and `-th.md`.
- Feature research: done. Data model has no missing fields; gaps are operational/ethical.
  See `docs/research/2026-08-03-feature-research.md`.
  Top adds: healer consent + attribution, edit history, print/PDF, bulk export, feedback form.
  Note: the report's USD 5k-15k/year backup cost does NOT fit this project; backup here is a
  small script + cheap object storage.
- Next step: pick a tier, then write the implementation plan (writing-plans skill).

## Data model (summary)

Six records: District, User, Doctor, Herb (shared catalog), Recipe, Case.

```
District ──< Doctor ──< Recipe ──< Case
                          └──< Ingredient >── Herb
```

- Case links to one Recipe. Patient is anonymous.
- Ingredient uses a decimal amount plus a unit, and links to the shared Herb catalog.
- Every Doctor/Recipe/Case has a `data_year`. Year locking is a later paid feature.

## Open items (deferred / paid)

- Approval-before-publish and edit history.
- Year locking.
- Bulk import of old data.
- Export to Excel/PDF (client said "maybe").
