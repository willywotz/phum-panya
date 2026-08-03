# Data Model and Standard Form — Design

Date: 2026-08-03
Status: Approved (design phase)

## 1. Purpose

The client collects folk-medicine records (ตำรายาหมอพื้นบ้าน) from every district
(อำเภอ) in one province. The client has no standard form yet, and the data is on
paper plus some Google Forms. This document gives two things:

1. The **data structure** for the future web app.
2. The **standard fill-in form** that every district uses now, before the app is built.

The form matches the data structure one-to-one. So later data entry is a direct copy.

Reference: the client forwarded a Thai "Tok Bidan" herbal app article
(so18.tci-thaijo.org/index.php/IAJ_FMS/article/download/831/567). The client wants a
similar app, but **without the map / GPS feature**.

## 2. Entities and relationships

Six record types:

| Record | Thai | Purpose |
|---|---|---|
| District | อำเภอ | The unit that groups all data. A fixed list. |
| User | ผู้ใช้งาน | A staff login. One editor per district, plus central admin. |
| Doctor | หมอพื้นบ้าน | The healer profile. The ID-card-like page. |
| Herb | สมุนไพร | One shared catalog for the whole province. |
| Recipe | ตำรับยา | A formula. It belongs to one doctor. |
| Case | เคส / ผลการรักษา | An anonymous treatment result. It links to one recipe. |

Relationships:

```
District ──< Doctor ──< Recipe ──< Case
                          │
                          └──< Ingredient >── Herb   (shared catalog)
```

- One District has many Doctors.
- One Doctor has many Recipes.
- One Recipe has many Cases. Each Case links to exactly one Recipe.
- One Recipe has many Ingredients. Each Ingredient points to one Herb, with an amount and unit.

## 3. Access rules

- **Public** — reads everything. No login.
- **District editor** — edits only the Doctors, Recipes, and Cases of their own district.
- **Central admin** — edits all districts, manages Users, and owns the shared Herb catalog
  and the District list.

Three rules hold across all records:

1. A district editor may write only rows where `district` is their own district.
2. The public never sees `phone`, `password`, `consent_*`, or audit fields.
3. Year fields group data by year now. Locking old years is a later paid step (§3.2).

Search and filter: the public list of Doctors and the list of Recipes+Cases both support
a keyword search and a district filter. The shared Herb catalog also lets the public filter
recipes by one herb.

### 3.1 Consent and privacy (must-do before launch)

The site names real healers and shows their knowledge in public. So consent is required,
not optional. Thai PDPA applies.

- A Doctor may go public only when `consent_obtained` is yes. The healer, or the district
  on their behalf, signs a short paper "Healer Knowledge Agreement". The central admin
  keeps the signed copy. The app records `consent_obtained` and `consent_date`.
- A healer may ask to remove their data. The central admin can unpublish or delete the
  Doctor and its Recipes and Cases (opt-out right).
- Every public Recipe credits its Doctor by name and district (attribution).
- Cases stay anonymous (see §4.7). A case photo must not show the patient's face.

### 3.2 Year rule (how a healer maps across years)

The client wants data that many users update each year. This is the rule:

- **One Doctor row per healer.** It does not duplicate each year. `first_year` marks the
  year the healer was first documented.
- **Recipes and Cases carry their own `data_year`** = the year that record was collected.
  So a healer can gain new recipes in later years without a duplicate profile.
- Full year snapshots and year locking (freeze a whole year) stay a later paid feature.

## 4. Fields

Legend: **R** = required, **O** = optional. "Public?" = the public sees it.

### 4.1 District (อำเภอ)

| Field | Type | R/O | Public? | Note |
|---|---|---|---|---|
| id | number | R | – | System key. |
| name | text | R | Yes | District name. |
| province | text | R | Yes | One province. Same for all rows. |

### 4.2 User (ผู้ใช้งาน)

| Field | Type | R/O | Note |
|---|---|---|---|
| id | number | R | System key. |
| full_name | text | R | Staff name. |
| email | text | R | The login name. |
| password | hash | R | Stored hashed. Never shown. |
| role | choice | R | `central_admin` or `district_editor`. |
| district | link → District | R* | Required only for a district editor. |
| active | yes/no | R | Turn off to block a login. |

### 4.3 Doctor (หมอพื้นบ้าน)

| Field | Type | R/O | Public? | Note |
|---|---|---|---|---|
| id | number | R | – | System key. |
| code | text | R | Yes | Short human code for linking, e.g. `MUE-01` (district + running number). |
| photo | image | R | Yes | The card-like portrait. |
| full_name | text | R | Yes | |
| known_as | text | O | Yes | Nickname or title. |
| gender | choice | O | Yes | |
| birth_year | number | O | Yes | Year, not full birth date. |
| district | link → District | R | Yes | Which district owns this doctor. |
| address | text | O | Yes | Sub-district, village. No map. |
| phone | text | O | No | Staff-only. Privacy. |
| specialty | choice (multi) | R | Yes | herbal, postpartum, bone, massage, other. |
| years_experience | number | O | Yes | |
| lineage | text | O | Yes | How they learned the skill. |
| consent_obtained | yes/no | R | No | The healer agreed to publish. Default no. See §3.1. |
| consent_date | date | O | No | When the signed agreement was dated. |
| status | choice | R | Yes | active / inactive / deceased. |
| first_year | number | R | Yes | The year the healer was first documented. See §3.2. |
| updated_by, updated_at | audit | R | No | System fills. |

### 4.4 Herb (สมุนไพร)

Central admin owns this catalog. District editors pick from it. If a herb is not in the
catalog yet, the district editor types the herb name as a **pending herb** (see 4.6). The
recipe saves at once. The central admin later matches the pending name to a catalog herb, or
adds a new herb. So data entry is never blocked.

| Field | Type | R/O | Public? | Note |
|---|---|---|---|---|
| id | number | R | – | System key. |
| thai_name | text | R | Yes | Common Thai name. |
| local_name | text | O | Yes | Local or dialect name. |
| scientific_name | text | O | Yes | e.g. *Abelmoschus esculentus*. |
| photo | image | O | Yes | |
| part_used | text | O | Yes | leaf, root, bark. |
| properties | text | O | Yes | Short use note. |

### 4.5 Recipe (ตำรับยา)

| Field | Type | R/O | Public? | Note |
|---|---|---|---|---|
| id | number | R | – | System key. |
| code | text | R | Yes | Short human code for linking, e.g. `MUE-01-R3` (doctor code + recipe number). |
| name | text | R | Yes | Formula name. |
| doctor | link → Doctor | R | Yes | The owner. District comes from the doctor. |
| indication | text | R | Yes | What it treats. |
| ingredients | list | R | Yes | See 4.6. |
| preparation | text | R | Yes | How to make it. |
| usage | text | R | Yes | How to take it. Dose. |
| caution | text | O | Yes | Warnings. |
| care_stage | choice | O | Yes | Optional tag, e.g. postpartum. |
| photo | image (many) | O | Yes | |
| data_year | number | R | Yes | The year this recipe was collected. |
| updated_by, updated_at | audit | R | No | System fills. |

### 4.6 Ingredient (a row inside a Recipe)

| Field | Type | R/O | Note |
|---|---|---|---|
| herb | link → Herb | R* | Pick from the shared catalog. |
| pending_herb_name | text | R* | Use this instead when the herb is not in the catalog yet. Central admin reconciles it later. Exactly one of `herb` or `pending_herb_name` is set. |
| amount | decimal | O | A number, e.g. 3, 0.5. |
| unit | choice + text | O | g, kg, ml, l, leaf, piece, handful, tsp, tbsp, or type another. |
| note | text | O | e.g. fresh, dried. |

### 4.7 Case (เคส / ผลการรักษา)

Anonymous. No patient name or ID.

| Field | Type | R/O | Public? | Note |
|---|---|---|---|---|
| id | number | R | – | System key. |
| recipe | link → Recipe | R | Yes | The one recipe used. Doctor comes from it. |
| patient_gender | choice | O | Yes | No name. |
| patient_age_range | choice | O | Yes | e.g. 20–39. No birth date. |
| condition | text | R | Yes | The symptom or illness. |
| treatment | text | O | Yes | Short summary of care. |
| result | choice | R | Yes | cured / better / no change. |
| duration | text | O | Yes | How long the care took. |
| photo | image | O | Yes | Must not show the face. |
| data_year | number | R | Yes | |
| updated_by, updated_at | audit | R | No | System fills. |

## 5. Standard fill-in form

Every district uses the same three sheets. Fill one Doctor sheet per healer, one Recipe
sheet per formula, and one Case sheet per result. Fill top-down: doctor first, then that
doctor's recipes, then each recipe's cases. The sheets work on paper or as a Google Form.

### Sheet A — Doctor (แบบฟอร์มหมอพื้นบ้าน)

```
District (อำเภอ): __________     Doctor code (รหัสหมอ): __________   *required
First year documented (ปีที่เก็บครั้งแรก): ______
Photo (รูปถ่าย): [ attach 1 photo — like an ID card ]
1. Full name (ชื่อ–สกุล): ______________________________  *required
2. Known as / title (ชื่อที่เรียก): ______________________
3. Gender (เพศ):  [ ] male  [ ] female  [ ] other
4. Birth year (ปีเกิด, พ.ศ.): ______
5. Address (ที่อยู่ — sub-district, village; NO map): ____________
6. Phone (เบอร์โทร — staff use only): ______________
7. Specialty (ความเชี่ยวชาญ — tick all):
   [ ] herbal (สมุนไพร) [ ] postpartum (หลังคลอด) [ ] bone (กระดูก)
   [ ] massage (นวด)    [ ] other: __________            *required
8. Years of experience (ประสบการณ์, ปี): ______
9. How they learned the skill (การสืบทอด): ________________
10. Status (สถานะ):  [ ] active  [ ] inactive  [ ] deceased   *required
11. Consent (ความยินยอม):  [ ] The healer agreed to publish. Signed agreement on file.  *required
    Date signed (วันที่ยินยอม): ______
```

Rule: give each doctor a code = district short code + running number, e.g. `MUE-01`.
Write the code on this sheet, and reuse it on the recipe sheet. Do not publish a doctor
until the consent box is ticked and the paper agreement is signed.

### Sheet B — Recipe (แบบฟอร์มตำรับยา)

```
Doctor code (รหัสหมอ — from Sheet A): __________   *required
Doctor name (ชื่อหมอเจ้าของตำรับ): ______________   (to double-check the code)
Recipe code (รหัสตำรับ): __________   *required   (doctor code + recipe number, e.g. MUE-01-R3)
District (อำเภอ): __________            Data year (ปีข้อมูล): ______
1. Recipe name (ชื่อตำรับ): ____________________________  *required
2. What it treats (สรรพคุณ / อาการ): __________________   *required
3. Ingredients (ส่วนประกอบ) — one line per herb:        *required
   | Herb name (ชื่อสมุนไพร) | Amount (จำนวน) | Unit (หน่วย) | Note (fresh/dried) |
   | ______________________ | ______________ | ____________ | __________________ |
   | ______________________ | ______________ | ____________ | __________________ |
   | ______________________ | ______________ | ____________ | __________________ |
4. How to prepare (วิธีปรุง): _________________________   *required
5. How to use / dose (วิธีใช้): _______________________   *required
6. Caution (ข้อควรระวัง): _____________________________
7. Care stage (ระยะ — optional): _____________________
8. Photo (รูป): [ attach if any ]
```

Note on herbs: write the herb name as the doctor says it. In the app, the district editor
picks the herb from the shared catalog. If the herb is not there yet, the editor saves it as
a pending herb and moves on. The central admin later matches the pending name to a catalog
herb, or adds a new herb. So the districts never manage the catalog and never get blocked.
This keeps herb names clean and lets the public filter recipes by herb.

### Sheet C — Case (แบบฟอร์มเคส / ผลการรักษา)

Keep the patient anonymous — no name, no ID.

```
Recipe code (รหัสตำรับที่ใช้ — from Sheet B): __________   *required
Recipe name (ชื่อตำรับที่ใช้): ________________________   (to double-check the code)
District (อำเภอ): __________            Data year (ปีข้อมูล): ______
1. Patient gender (เพศผู้ป่วย):  [ ] male  [ ] female  [ ] other
2. Patient age range (ช่วงอายุ):
   [ ] 0–12 [ ] 13–19 [ ] 20–39 [ ] 40–59 [ ] 60+
3. Condition / symptom (อาการ / โรค): ________________   *required
4. Treatment summary (การรักษาโดยย่อ): _______________
5. Result (ผลการรักษา):
   [ ] cured (หาย) [ ] better (ดีขึ้น) [ ] no change (ไม่เปลี่ยน)   *required
6. Duration (ระยะเวลา): ______________
7. Photo (รูป — must NOT show the face): [ attach if any ]
```

### How the sheets connect

Records link by **code**, not by name, so duplicate or misspelled names never break the link.

- Sheet B carries the **doctor code** → the app links the recipe to that doctor.
- Sheet C carries the **recipe code** → the app links the case to that recipe.
- The name field on each sheet is a double-check only. If the code and name disagree, the
  data-entry step flags the row for the central admin.

## 6. Out of scope (later paid features)

- Approval-before-publish and edit history.
- Year locking (freeze old years).
- Bulk import of old paper/Excel data.
- Herb catalog editing by districts (central admin owns it in this design).
- Server migration later.
