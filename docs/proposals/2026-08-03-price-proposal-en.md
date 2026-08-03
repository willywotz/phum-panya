# Price Proposal — Folk-Medicine Web App (English draft)

Date: 2026-08-03
Status: Draft for internal review. Translate to Thai after approval.
All prices in Thai baht (THB). Numbers are a starting point — adjust before you send.

## 1. What this app is

One web app for the whole province. It collects folk-medicine records, grouped by
district. The public reads all data. One editor per district, plus a central admin,
write the data. Users open the site by QR scan. No mobile-store app. No map.

The app has six record types: District, User, Doctor, Herb, Recipe, and Case.
See the data model: `docs/superpowers/specs/2026-08-03-data-model-and-form-design.md`.

## 2. Important: price depends on features, not record count

You asked "how many people or pages" each price gives. A web app does not charge per
record. The number of doctors, recipes, and cases is effectively unlimited. The only
limit is server storage (about 30 GB), which holds a few thousand records with photos.

So the price does **not** grow with the number of doctors. The price grows with the
**features** you turn on. The three tiers below show this.

## 3. Tier comparison

| Feature | Basic | Standard | Complete |
|---|:--:|:--:|:--:|
| Responsive web app (phone + computer), QR access | ✓ | ✓ | ✓ |
| Public view: Doctor / Recipe / Case | ✓ | ✓ | ✓ |
| Keyword search + district filter | ✓ | ✓ | ✓ |
| Roles: central admin + 1 editor per district | ✓ | ✓ | ✓ |
| Add / edit / delete: Doctor, Recipe, Case | ✓ | ✓ | ✓ |
| Photo upload (doctor, recipe) | ✓ | ✓ | ✓ |
| Ingredient amount + unit | ✓ | ✓ | ✓ |
| Herb storage | free text | shared catalog + herb filter | shared catalog + herb filter |
| Excel / PDF export | — | ✓ | ✓ |
| Approval before publish + edit history | — | — | ✓ |
| Year locking (freeze old years) | — | — | ✓ |
| Bulk import help for old data | — | — | up to 500 records |
| Revision rounds | 2 | 2 | 3 |
| Free bug warranty | 90 days | 90 days | 120 days |
| Install / deploy | 1 | 1 | 1 |
| **One-time price (THB)** | **12,000** | **22,000** | **35,000** |

## 4. Tier notes

**Basic — 12,000 THB.** The core app. It keeps the important part: per-district
editing, public view, and search. It trims the extras. Herbs are a plain text list
inside each recipe, so the public cannot filter recipes by one herb. No export.

**Standard — 22,000 THB (recommended).** Adds the shared herb catalog. Each herb is a
record with a Thai name, local name, and photo. The public can filter recipes by one
herb. Adds Excel and PDF export. This tier sits under the 25,000 ceiling you mentioned.

**Complete — 35,000 THB.** Adds control and history: approval before the public sees a
record, a full edit history, and year locking to freeze old years. Includes help to
import up to 500 old paper or Excel records. One extra revision round and a longer
warranty.

## 5. In every tier

- Design that fits phone and computer.
- One deploy to your server.
- Two revision rounds (Complete has three).
- Free bug fixing for 90 days after delivery (Complete: 120 days).
- A short guide on how to use the admin pages.

## 6. Recurring cost (separate from the one-time price)

- Server and storage: about **350 THB per month** for roughly 30 GB.
- If the data grows past 30 GB, we raise the plan then.

## 7. Not in the price (add-ons, priced on request)

| Add-on | Suggested price |
|---|---|
| Excel / PDF export (if you take Basic) | +3,000 THB |
| Shared herb catalog + herb filter (if you take Basic) | +5,000 THB |
| Approval before publish + edit history | +6,000 THB |
| Year locking | +4,000 THB |
| Bulk import of old data | +5,000 THB per 500 records |
| Server migration later | quote when needed |

Data entry itself is not in the price. Each district enters its own doctors, recipes,
and cases with the standard form.

## 8. Timeline (after the data structure is signed off)

- Basic: 3–4 weeks.
- Standard: 4–6 weeks.
- Complete: 6–8 weeks.

The data structure and standard form are already designed. So the districts can start
to collect data now, in parallel with the build.

## 9. Payment terms (suggested)

- 50% deposit to start.
- 50% on delivery and sign-off.

## 10. Next step

1. You pick a tier (or a tier plus add-ons).
2. You approve the budget with your office.
3. We sign off the scope and start. The districts collect data with the standard form
   at the same time.
