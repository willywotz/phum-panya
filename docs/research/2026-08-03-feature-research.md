# Feature Research: Thai Folk-Medicine Knowledge Web App Completeness

**Date:** 2026-08-03  
**Status:** Research findings for scope evaluation  
**Target:** Identify missing features that comparable systems have, fit for a budget-limited Thai provincial health context.

---

## Executive Summary

Research into Thai Traditional Medicine standards (DTAM), herbal database systems (MANOSROI III, Tok Bidan app), ethnobotany preservation practices (Mukurtu CMS, TK Labels), and government health IT standards identifies **5 core missing feature areas** that would make the app more complete as a public health resource:

1. **Healer Consent & Knowledge Attribution** (Medium effort, MUST-HAVE for credibility)
2. **Audit & Edit History** (Small effort, should-have for accountability)  
3. **Print & PDF Export** (Small effort, should-have for accessibility)
4. **View Analytics & Popular Content Tracking** (Small effort, nice-to-have for engagement)
5. **Backup & Data Integrity** (Small effort at this scale — a cron dump + object storage; MUST-HAVE for government use)

Each can be implemented incrementally; none require the map/GPS or mobile-app features you rejected.

---

## Feature Recommendations Table

| Feature | Effort | Priority | Legal/Ethical | Rationale | Fits Budget? |
|---------|--------|----------|---------------|-----------|-------------|
| **Healer Consent & TK Attribution** | Medium | MUST | YES | DTAM + ethical healing practice; prevents unlicensed use | Yes, if built-in from start |
| **Edit History & Audit Trail** | Small | SHOULD | YES | Thai gov't data standards require audit; accountability | Yes |
| **Print/PDF Export** | Small | SHOULD | NO | Rural access, offline use, Thai gov't accessibility standards | Yes |
| **Analytics: View Counts & Popular Recipes** | Small | NICE | NO | Track engagement; inform content strategy | Yes |
| **Image Licensing Metadata** | Small | NICE | NO | Public domain/CC attribution compliance | Yes |
| **Data Backup & Integrity Checks** | Small | MUST | YES | Government-adjacent client needs continuity | Yes — a cron dump + object storage, a few USD/mo |
| **Bulk Export (JSON/CSV)** | Small | SHOULD | NO | Thai open-data standards; data portability | Yes |
| **Audio Pronunciation (Local Names)** | Medium | NICE | NO | Tok Bidan app feature; aid for non-readers | Deferred to later release |
| **Multi-District Comparison / Dashboard** | Medium | NICE | NO | Province-wide insight; not in scope yet | Deferred |
| **Contact/Feedback Form** | Small | SHOULD | NO | Thai gov't standard; user engagement | Yes |

---

## Area 1: Thai Traditional Medicine Standards & DTAM

### Findings

**DTAM Regulatory Context:**
The Department of Thai Traditional and Alternative Medicine (DTAM, กรมการแพทย์แผนไทยและการแพทย์ทางเลือก) is the official regulator under Thailand's Ministry of Public Health. DTAM is responsible for setting TTM standards, protecting TTM knowledge, and managing the "Thai Herbal Pharmacopoeia" (the official herb reference).

**Herb Standardization:**
DTAM uses DNA barcode authentication for herbal products and collaborates with the Thai FDA (TFDA) to enforce quality and safety standards for registered herbal products. The Thai Herbal Pharmacopoeia defines microbial limits and chemical fingerprints for herbal materials. While not a public online registry exposed directly, DTAM maintains an official list of recognized herbs and their monographs (botanical name, part used, properties, preparation guidance).

**Recommended Action:** Your herb catalog should align with DTAM's Thai Herbal Pharmacopoeia. Consider:
- Adding a `scientific_name` field and cross-check against DTAM's official list (your form already includes this — good).
- Optionally add `dtam_approved` flag to indicate herbs registered with DTAM/TFDA for credibility.
- In admin documentation, note that new herbs should be validated against DTAM standards before approval.

**Effort:** Small (metadata enhancement). **Priority:** SHOULD-HAVE (for credibility with Ministry).

### Sources
- [DTAM Official Website](https://www.dtam.moph.go.th/)
- [DNA Barcode Library of Thai Herbal Pharmacopoeia](https://www.nature.com/articles/s41598-022-13287-x) — Nature Scientific Reports
- [Thai Traditional Medicine Regulation in Thailand](https://lcm.amegroups.org/article/view/7154/html) — Longhua Chinese Medicine

---

## Area 2: Herbal Database Systems & Ethnobotany Standards

### Findings

**Field Structure from MANOSROI III & Ethnobotany Literature:**
The MANOSROI III database (Thailand's largest medicinal plant recipe database with 200,000+ recipes) and ethnobotanical literature define a proven field structure:

| Core Field | Type | Note |
|------------|------|------|
| **Plant Identification** | Scientific name, Thai name, local name | Your Herb model covers this ✓ |
| **Plant Part Used** | Text (leaf, root, bark, seed, etc.) | Your Herb.part_used covers this ✓ |
| **Preparation Method** | Text (fresh, dried, decoction, paste) | Your Recipe.preparation + Ingredient.note covers this ✓ |
| **Dosage & Administration** | Amount + Unit + frequency | Your Ingredient (amount, unit) + Recipe.usage covers this ✓ |
| **Indication (What it Treats)** | Text | Your Recipe.indication covers this ✓ |
| **Contraindication & Caution** | Text | Your Recipe.caution covers this ✓ |

**Your data model is already aligned** with ethnobotanical standards. The MANOSROI database includes cosmetic and wellness recipes (87 hair, 79 skin, 42 acne recipes), so your optional `care_stage` field is appropriate.

**Additional Field from Tok Bidan App:**
The reference app (Tok Bidan article) mentions audio pronunciation for local herb names. This is valuable for rural, non-literate, or dialect-speaking users. Consider deferring this to a v2 (Medium effort).

**Recommended Action:** None needed for MVP. Your fields are complete. For a future release:
- Add optional `audio_url` field to Herb for local pronunciation.
- Optionally add `contraindication` field if not already implicit in Recipe.caution.

**Effort:** Small (deferred). **Priority:** NICE-TO-HAVE (v2 feature).

### Sources
- MANOSROI III medicinal-plant recipe database — the figures cited here come from a
  commercial portfolio page ([manose.co](https://www.manose.co/portfolio-item/work-2/)),
  NOT a primary source. Treat the recipe counts as indicative only. The field-structure
  comparison stands on the peer-reviewed ethnobotany sources below.
- [Ethnobotanical Documentation Framework](https://link.springer.com/article/10.1007/BF02859302) — Springer Ethnobotany
- [Quantitative Ethnobotany Methods](https://frontiersin.org/articles/10.3389/fphar.2018.00040/full) — Frontiers in Pharmacology

---

## Area 3: Community Knowledge, Consent & Attribution (TK Labels)

### Findings

**Traditional Knowledge (TK) Labels & Ethical Issues:**
Indigenous knowledge (including folk medicine) has historically been collected without consent, published without attribution, and used for commercial gain without community benefit. The Local Contexts Initiative developed "TK Labels" — metadata tags that communities can attach to digital materials to express:
- Who owns the knowledge (healer, community, lineage).
- Who may access it (restricted, open, commercial use blocked, etc.).
- How it must be attributed / cited.
- Consent and permission status.

**Mukurtu CMS Model:**
Mukurtu (named by the Warumungu people, meaning "safe-keeping place for sacred items") is a free CMS designed for Indigenous heritage. It provides:
- **Cultural Protocols:** Each record can have different access rules (e.g., "viewable only by initiated members" or "public").
- **TK Labels:** Metadata tags that travel with the data on export.
- **Attribution Tracking:** Clear ownership and authorship fields.
- **Roundtrip Integrity:** Export/import preserves all cultural metadata.

**Thai Folk-Medicine Context:**
Your app already has the right foundation:
- Each Doctor is named and credited (good for attribution).
- Each Recipe links to its Doctor (good for healer attribution).
- Your Case records are anonymous (good for privacy).
- You have `updated_by` audit field (good for tracking edits).

**What's Missing:**
- **Explicit consent checkbox** at data entry: "The healer/district has given permission to publish this knowledge."
- **Healer agreement/contract** (legal): before publishing a Doctor's recipes, the client should have a simple agreement with the healer affirming consent and use rights.
- **TK Label metadata**: A note field indicating "Knowledge belongs to [healer name] of [lineage/tradition]" — this doesn't require new tech, just a content field.
- **Data export includes attribution**: When the public exports a Recipe, it should include "Created by [Doctor name] — [District] Province" and the data year.

**Recommended Action:**
1. **Add consent tracking** (Small effort):
   - Add optional `healer_consent_obtained` (yes/no) to Doctor, defaulting to no.
   - Add optional `knowledge_licensing` (text field) for free-text license terms, e.g., "CC-BY-4.0 with healer attribution" or "Thai Traditional Knowledge Label - Private Community Use."
   
2. **Create a "Healer Knowledge Agreement" template** (Small effort):
   - A PDF/form the central admin can print and have the healer sign.
   - Affirms the healer consents to digital documentation and public (or restricted) sharing.
   - Document scanned and filed (not in the app, but noted in audit).

3. **Export includes attribution** (Small effort):
   - When a Recipe is exported to PDF or JSON, include: "By [Doctor name], [District]. Data year: [year]. Licensed: [knowledge_licensing field]."

**Effort:** Small-Medium (fields + export logic). **Priority:** MUST-HAVE (ethical, credible public health resource).

### Sources
- [Traditional Knowledge Labels and Licensing](https://www.sfu.ca/ipinch/project-components/community-based-initiatives/special-initiative-traditional-knowledge-licensing-an/) — Simon Fraser University
- [Mukurtu CMS: Supporting Digital Sovereignty](https://www.interaccess.org/tf-case-studies/mukurtu) — InterAccess
- [Mukurtu About Page](https://mukurtu.org/about/) — Mukurtu CMS
- [Traditional Knowledge and Creative Commons](https://medium.com/creative-commons-we-like-to-share/traditional-knowledge-and-copyright-intersections-d8cb78375e40) — Medium / Creative Commons
- [Beyond Copyright: Traditional Knowledge and Biocultural Labels](https://www.lib.sfu.ca/help/publish/scholarly-publishing/radical-access/beyondcopyright-traditionalknowledge-bioculturallabels) — SFU Library

---

## Area 4: Practical Web-App Completeness for Government Use

### 4.1 Audit & Edit History

**Findings:**
Thai government data standards (via the Thai Health Information Standards Development Center, Ministry of Public Health) require audit trails for health data. Medical SOAP notes (clinical records) in Thailand must log: who edited, when, what changed.

Your app already has `updated_by` and `updated_at` on all records. **Improvement:** Add a simple **edit history log** that users can view (staff only, not public). This shows:
- Who edited this Doctor/Recipe/Case?
- When?
- What field changed (optional: before/after values)?

**Effort:** Small (add history table + read-only API endpoint). **Priority:** SHOULD-HAVE (government compliance).

### 4.2 Print & PDF Export

**Findings:**
Thai government accessibility standards (TWCAG 2025, based on WCAG 2.1) require that public health resources be accessible in multiple formats, including print and offline. Rural users may have unreliable internet. PDF export is a proven pattern for health resources.

**Recommended Action:**
- Add a "Print" or "Export as PDF" button on public Doctor, Recipe, and Case detail pages.
- Use a library like `html2pdf.js` or a server-side tool (e.g., wkhtmltopdf, Puppeteer) to generate clean, A4-formatted PDFs.
- Include: healer name, recipe name, ingredients, preparation, usage, caution, case results, data year, and a "Licensed: [knowledge_licensing]" footer.

**Effort:** Small (PDF generation library + templates). **Priority:** SHOULD-HAVE (accessibility, offline use).

### 4.3 Analytics: View Counts & Popular Content

**Findings:**
U.S. and European government health databases (CDC, NHS) track resource popularity to inform content curation. Simple view counts are non-invasive (no personal tracking) and help editors know which recipes/doctors are most trusted.

**Recommended Action:**
- Add a `view_count` integer to Doctor, Recipe, and Case, increment on each public read.
- Optionally, add `last_viewed_at` timestamp.
- In the central admin dashboard, show "Top 10 Recipes by View Count" and "Top 10 Doctors by View Count" — helps editors prioritize curation.
- Do NOT track individual IPs or identities (privacy-first).

**Effort:** Small. **Priority:** NICE-TO-HAVE (useful for engagement, not essential).

### 4.4 Image Licensing Metadata

**Findings:**
Public health resources often include images (herb photos, case photos) from public domain, Creative Commons, or original sources. Proper attribution is both a legal requirement and an ethical best practice.

**Recommended Action:**
- Add optional fields to Herb and Case (images):
  - `image_source`: URL or "Original" or "Public Domain"
  - `image_license`: CC-BY-4.0, CC-BY-SA, Public Domain, Proprietary (with permission text)
  - `image_credit`: "Photo by [Name]" or "CDC PHIL"
- In public view, display: "Photo: [image_credit]. License: [image_license]."

**Effort:** Small (metadata + template update). **Priority:** NICE-TO-HAVE (ethical practice).

### 4.5 Bulk Export (CSV, JSON)

**Findings:**
Thai government open-data standards (via the Digital Government Development Agency, DGA) recommend that public datasets be exportable in JSON and CSV format. This allows researchers, other government agencies, and NGOs to integrate the data into their own tools.

**Recommended Action:**
- Add "Export as JSON" and "Export as CSV" buttons on the public Doctor, Recipe, Herb, and Case list pages.
- Include all public fields + `data_year`, `updated_at`, and a URL back to the original record.
- JSON export should follow a simple schema: `{ "type": "doctor", "id": 1, "name": "...", "recipes": [...] }`.

**Effort:** Small (API endpoint + serialization). **Priority:** SHOULD-HAVE (aligns with Thai gov open-data direction).

### 4.6 Contact & Feedback Form

**Findings:**
A feedback mechanism on public sites is a widely-recommended e-government pattern (see the
US Section 508 guidance below as an analogy; this is not a Thai-specific mandate). It helps
with user engagement and, more important here, lets a viewer report a wrong or unsafe herbal
entry — valuable for a public health resource.

**Recommended Action:**
- Add a simple "Feedback" or "Report an Error" button on the public site footer.
- Opens a form: "Name (optional), Email (optional), Message."
- Submits to the central admin's email.
- Do NOT require login (accessibility).
- Optional: add a "Type" dropdown: Bug Report, Suggestion, Missing Data, etc.

**Effort:** Small. **Priority:** SHOULD-HAVE (engagement, user trust).

### Sources
- [Thai Health Information Standards](https://thaidj.org/index.php/JHS/article/view/17653) — Journal of Health Science of Thailand
- Thai Web Content Accessibility (TWCAG) is published by Thailand's Electronic Transactions
  Development Agency (ETDA) and is based on W3C WCAG 2.1. Primary source: [WCAG 2.1](https://www.w3.org/TR/WCAG21/) — W3C.
  (The earlier India-government link was mislabeled as Thai and is removed.)
- [Thai Open Government Data Standards](https://www.dga.or.th/en/our-services/digital-platform-services/open-government-data/) — Digital Government Development Agency
- [Print-Friendly Design for Health Resources](https://odphp.health.gov/healthliteracyonline/2010/Web_Guide_Health_Lit_Online.pdf) — U.S. ODPHP Health Literacy Online
- [Public Feedback Mechanisms in Government](https://www.section508.gov/manage/laws-and-policies/implementing-public-feedback-mechanism/) — U.S. Section 508 Guidance (cited as an analogy, not a Thai standard)
- [Image Attribution & Licensing](https://hslguides.med.nyu.edu/medicalimages) — NYU Health Sciences Library

---

## Area 5: Data Integrity & Backup (Critical for Government)

### Findings

**Government Data Protection Standards:**
Thailand's Personal Data Protection Act (PDPA) and the Digitalization of Public Administration & Services Delivery Act (2019) require government agencies to protect data integrity, confidentiality, and availability. Small government databases are at risk of loss due to hardware failure, accidental deletion, or security breach.

**Cost-effective backup for THIS app (corrected):**
The enterprise DRaaS figures often quoted (USD 5,000–50,000/year) do NOT fit this project.
This app runs on a ~350 THB/month (~USD 10/month) server with ~30 GB. A right-sized backup
here is a small script, not a DRaaS contract:
- A daily `pg_dump` (or equivalent) plus a daily copy of the photo folder to cheap object
  storage (e.g. S3/GCS/Backblaze, or a Thai provider). Storage for tens of GB with 30-day
  retention costs roughly a few USD per month, not thousands per year.
- A cron job runs the dump and the upload. No paid DR platform is needed at this size.
- Estimated real cost: a few USD/month of storage, plus one-time setup. This is a Small task.

**Recommended Action:**
1. **Daily automated backups** to a cloud provider (AWS, Google Cloud, or Thai-local provider like NECTEC).
   - Database snapshots (daily).
   - File storage snapshots (photos, uploads).
   - Retention: keep 30 days of backups (rolling window).

2. **Data integrity checks** (Small effort):
   - Monthly checksums on all records to detect corruption.
   - Run a script: hash all data, log it, compare monthly.

3. **Recovery testing** (Operational):
   - Test recovery once per quarter (restore to a staging environment, verify data).
   - Document recovery time objective (RTO): how fast can data be restored? (Target: 4 hours.)
   - Document recovery point objective (RPO): how much data loss is acceptable? (Target: 24 hours = 1 day.)

4. **Audit trail for deletions** (Medium effort):
   - Soft-delete records (mark as `deleted_at`, do not hard-delete).
   - Keep audit log of who deleted what and when.
   - Admin can restore a soft-deleted record within 90 days.

**Effort:** Medium (one-time setup + ongoing ops). **Priority:** MUST-HAVE (government mandate, data protection).

### Sources
- [Thai PDPA & Data Protection](https://pmc.ncbi.nlm.nih.gov/articles/PMC12703692/) — PMC NIH
- [Disaster Recovery Planning for Government](https://www.ready.gov/business/emergency-plans/recovery-plan) — Ready.gov
- [DRaaS for Government Agencies](https://www.govpilot.com/blog/government-it-disaster-recovery) — GovPilot
- [NIST Contingency Planning Standards](https://n-able.com/blog/backup-and-disaster-recovery-plan) — N-able Blog

---

## Legal & Ethical Must-Dos

### Consent & Knowledge Ownership (MUST-HAVE)
1. **Healer Consent Form**: Before publishing any Doctor's recipes, obtain signed consent from the healer (or district authority on their behalf). Template recommended.
2. **Knowledge Attribution**: Every published recipe must credit the Doctor by name and district. This is both legal (IP protection in TK context) and ethical (respect for knowledge holder).
3. **Opt-Out Rights**: Healers should be able to request removal of their knowledge from the public site (though rare, this respects autonomy).

**Reference:** [TK Labels & Local Contexts](https://www.sfu.ca/ipinch/project-components/community-based-initiatives/special-initiative-traditional-knowledge-licensing-an/), [Thai PDPA](https://pmc.ncbi.nlm.nih.gov/articles/PMC12703692/)

### Patient Privacy (Already Compliant)
- Your Case records are already anonymous (no patient name, ID, or full birth date). ✓
- Patient age is a range (0–12, 13–19, etc.), not exact. ✓
- Photo must not show patient face. ✓
- This aligns with HIPAA-equivalent privacy (Thai PDPA). ✓

**No action needed.**

### Image Copyright (SHOULD-HAVE)
- All images used (herb photos, case photos) must be licensed.
- If using a photo not your own, add attribution metadata (creator, source, license).
- Prefer public domain (CDC PHIL, Wikimedia) or Creative Commons licensed images.

**Effort:** Small (add metadata fields + terms of use doc).

### Data Retention & Deletion (SHOULD-HAVE)
- Define a data retention policy: "Case records are kept for 5 years, then anonymized or archived."
- Implement soft-delete (records marked `deleted_at`, not hard-deleted).
- Log all deletions in audit trail.
- This complies with Thai PDPA data minimization principles.

**Effort:** Small (policy doc + soft-delete implementation).

---

## Open Questions

1. **Healer Licensing & DTAM**: Should the app track whether a Doctor is officially licensed by DTAM, or is it solely a folk-medicine knowledge base? (Recommend: optional `dtam_licensed` flag for future enhancement.)

2. **Commercial Use**: Should the client restrict commercial use of recipes (e.g., herbal companies cannot use the data to make products without permission)? If yes, add a `commercial_use_allowed` flag.

3. **Multi-Language**: The research mentions "Thai + local dialect names" for herbs. Should Recipe instructions also support Thai + English? Recommend deferring to v2.

4. **Audio Pronunciation**: Tok Bidan app includes audio. Should this app? (Recommend: v2 feature, Medium effort, budget TBD.)

5. **Data Versioning**: Should old versions of a Recipe be publicly viewable with a changelog? Or only the latest version? (Recommend: audit trail for staff only; public sees latest only.)

6. **Community Review**: Should district editors have a peer-review step before publishing new Doctor/Recipe data? (Recommend: deferred to paid "Approval-before-publish" feature already listed in CONTEXT.md.)

---

## Summary: Prioritized Implementation Roadmap

### MVP (Ready Now, No-Scope-Creep)
- ✓ Your current data model (Doctor, Recipe, Case, Herb).
- ✓ Public read, district edit, central admin.
- ✓ Keyword search and district filter.

### v1.1 (Small Features, 1–2 Weeks Dev)
1. **Healer Consent Field** + **Knowledge Licensing Field** (Herb + Doctor).
2. **Edit History Log** (staff-only view).
3. **Print/PDF Export** (on detail pages).
4. **Contact/Feedback Form**.
5. **View Count Analytics** (simple counter).

### v1.2 (Small Features, 1–2 Weeks Dev)
1. **Bulk Export** (CSV, JSON for public lists).
2. **Image Licensing Metadata** (Herb, Case).
3. **Thai Open Data Alignment** (JSON schema + DGA compatibility).

### v2 (Medium Features, Deferred)
1. **Audio Pronunciation** (Herb local names).
2. **Approval-Before-Publish** (already in CONTEXT.md paid list).
3. **Year Locking** (already in CONTEXT.md paid list).
4. **Multi-District Dashboard** (province-wide insights).

### Ongoing (Operational)
1. **Daily Automated Backups** (cron dump + object storage).
2. **Data Integrity Checks** (monthly).
3. **Recovery Testing** (quarterly).
4. **Healer Consent Agreements** (legal/admin process).

---

## Conclusion

Your app's data model and access structure are already well-aligned with ethnobotanical best practices and Thai health IT standards. The main gaps are **not in data structure but in operational and ethical practices**:

1. **Consent & Attribution** (ethical + legal MUST).
2. **Audit & Versioning** (compliance SHOULD).
3. **Export & Accessibility** (public benefit SHOULD).
4. **Backup & Disaster Recovery** (government mandate MUST).

All of these can be added incrementally without scope creep, and most are Small or Medium effort. Recommend prioritizing items 1–5 from the v1.1 roadmap before launch to ensure credibility with the Ministry and user trust.

---

**Report compiled:** 2026-08-03  
**Researcher:** Claude  
**Sources:** DTAM official docs, MANOSROI III database, Mukurtu CMS, Thai government open data standards, WHO/CDC health IT best practices, TK Labels initiative.
