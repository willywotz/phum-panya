'use client';

import { createContext, useContext, useEffect, useState } from 'react';

export type Locale = 'th' | 'en';

const STORAGE_KEY = 'locale';
const DEFAULT_LOCALE: Locale = 'th';

type Dictionary = Record<string, string>;

const th: Dictionary = {
  signIn: 'เข้าสู่ระบบ',
  email: 'อีเมล',
  password: 'รหัสผ่าน',
  loginError: 'อีเมลหรือรหัสผ่านไม่ถูกต้อง',
  staffDashboard: 'แดชบอร์ดเจ้าหน้าที่',
  logOut: 'ออกจากระบบ',
  districts: 'อำเภอ',
  name: 'ชื่อ',
  province: 'จังหวัด',
  add: 'เพิ่ม',
  edit: 'แก้ไข',
  delete: 'ลบ',
  save: 'บันทึก',
  cancel: 'ยกเลิก',
  actions: 'การกระทำ',
  mismatchWarning: 'รหัสกับชื่อไม่ตรงกัน โปรดตรวจสอบ',
  confirmDelete: 'ยืนยันการลบ?',
  yes: 'ใช่',
  no: 'ไม่',
  staffNav: 'เมนูเจ้าหน้าที่',
  users: 'ผู้ใช้งาน',
  herbs: 'สมุนไพร',
  doctors: 'หมอพื้นบ้าน',
  recipes: 'ตำรับยา',
  cases: 'เคสรักษา',
  code: 'รหัส',
  fullName: 'ชื่อ-นามสกุล',
  role: 'บทบาท',
  roleCentralAdmin: 'ผู้ดูแลระบบส่วนกลาง',
  roleDistrictEditor: 'ผู้แก้ไขข้อมูลอำเภอ',
  district: 'อำเภอ',
  photo: 'รูปภาพ',
  knownAs: 'ชื่อที่รู้จัก',
  gender: 'เพศ',
  genderMale: 'ชาย',
  genderFemale: 'หญิง',
  genderOther: 'อื่นๆ',
  birthYear: 'ปีเกิด',
  address: 'ที่อยู่',
  phone: 'เบอร์โทรศัพท์',
  specialty: 'ความเชี่ยวชาญ',
  specialtyHerbal: 'สมุนไพร',
  specialtyPostpartum: 'การอยู่ไฟ',
  specialtyBone: 'กระดูก',
  specialtyMassage: 'นวด',
  specialtyOther: 'อื่นๆ',
  yearsExperience: 'ปีประสบการณ์',
  lineage: 'สายตระกูล',
  consentObtained: 'ได้รับความยินยอม',
  consentDate: 'วันที่ยินยอม',
  status: 'สถานะ',
  statusActive: 'ใช้งาน',
  statusInactive: 'ไม่ใช้งาน',
  statusDeceased: 'เสียชีวิต',
  firstYear: 'ปีที่เริ่มต้น',
  thaiName: 'ชื่อไทย',
  localName: 'ชื่อท้องถิ่น',
  scientificName: 'ชื่อวิทยาศาสตร์',
  partUsed: 'ส่วนที่ใช้',
  properties: 'สรรพคุณ',
  pendingHerbs: 'สมุนไพรรอตรวจสอบ',
  pendingHerbName: 'ชื่อสมุนไพรที่รอตรวจสอบ',
  reconcileToHerb: 'จับคู่กับสมุนไพร',
  reconcile: 'จับคู่',
  indication: 'ข้อบ่งใช้',
  preparation: 'วิธีเตรียมยา',
  usage: 'วิธีใช้',
  caution: 'ข้อควรระวัง',
  careStage: 'ระยะการดูแล',
  dataYear: 'ปีข้อมูล',
  ingredient: 'ส่วนประกอบ',
  ingredients: 'ส่วนประกอบ',
  herb: 'สมุนไพร',
  herbName: 'ชื่อสมุนไพร',
  otherHerb: 'อื่นๆ (พิมพ์ชื่อ)',
  amount: 'ปริมาณ',
  unit: 'หน่วย',
  note: 'หมายเหตุ',
  addIngredient: 'เพิ่มส่วนประกอบ',
  removeIngredient: 'ลบส่วนประกอบ',
  doctor: 'หมอพื้นบ้าน',
  patientGender: 'เพศผู้ป่วย',
  patientAgeRange: 'ช่วงอายุผู้ป่วย',
  condition: 'อาการ',
  treatment: 'การรักษา',
  result: 'ผลการรักษา',
  resultCured: 'หายขาด',
  resultBetter: 'ดีขึ้น',
  resultNoChange: 'ไม่เปลี่ยนแปลง',
  duration: 'ระยะเวลา',
  recipe: 'ตำรับยา',
  ageRange0to12: '0-12 ปี',
  ageRange13to19: '13-19 ปี',
  ageRange20to39: '20-39 ปี',
  ageRange40to59: '40-59 ปี',
  ageRange60plus: '60 ปีขึ้นไป',
  home: 'หน้าแรก',
  welcome: 'ยินดีต้อนรับสู่ภูมิปัญญา',
  publicNav: 'เมนูสาธารณะ',
  search: 'ค้นหา',
  allDistricts: 'ทุกอำเภอ',
  allHerbs: 'ทุกสมุนไพร',
  print: 'พิมพ์',
  doctorNotFound: 'ไม่พบข้อมูลหมอพื้นบ้าน',
  exportCsv: 'ส่งออก CSV',
  exportExcel: 'ส่งออก Excel',
  storage: 'พื้นที่จัดเก็บ',
  storageUsed: 'ใช้ไปแล้ว',
};

const en: Dictionary = {
  signIn: 'Sign in',
  email: 'Email',
  password: 'Password',
  loginError: 'Wrong email or password',
  staffDashboard: 'Staff dashboard',
  logOut: 'Log out',
  districts: 'Districts',
  name: 'Name',
  province: 'Province',
  add: 'Add',
  edit: 'Edit',
  delete: 'Delete',
  save: 'Save',
  cancel: 'Cancel',
  actions: 'Actions',
  mismatchWarning: 'Code and name do not match — please check',
  confirmDelete: 'Confirm delete?',
  yes: 'Yes',
  no: 'No',
  staffNav: 'Staff menu',
  users: 'Users',
  herbs: 'Herbs',
  doctors: 'Doctors',
  recipes: 'Recipes',
  cases: 'Cases',
  code: 'Code',
  fullName: 'Full name',
  role: 'Role',
  roleCentralAdmin: 'Central admin',
  roleDistrictEditor: 'District editor',
  district: 'District',
  photo: 'Photo',
  knownAs: 'Known as',
  gender: 'Gender',
  genderMale: 'Male',
  genderFemale: 'Female',
  genderOther: 'Other',
  birthYear: 'Birth year',
  address: 'Address',
  phone: 'Phone',
  specialty: 'Specialty',
  specialtyHerbal: 'Herbal',
  specialtyPostpartum: 'Postpartum care',
  specialtyBone: 'Bone setting',
  specialtyMassage: 'Massage',
  specialtyOther: 'Other',
  yearsExperience: 'Years of experience',
  lineage: 'Lineage',
  consentObtained: 'Consent obtained',
  consentDate: 'Consent date',
  status: 'Status',
  statusActive: 'Active',
  statusInactive: 'Inactive',
  statusDeceased: 'Deceased',
  firstYear: 'First year',
  thaiName: 'Thai name',
  localName: 'Local name',
  scientificName: 'Scientific name',
  partUsed: 'Part used',
  properties: 'Properties',
  pendingHerbs: 'Pending herbs',
  pendingHerbName: 'Pending herb name',
  reconcileToHerb: 'Reconcile to herb',
  reconcile: 'Reconcile',
  indication: 'Indication',
  preparation: 'Preparation',
  usage: 'Usage',
  caution: 'Caution',
  careStage: 'Care stage',
  dataYear: 'Data year',
  ingredient: 'Ingredient',
  ingredients: 'Ingredients',
  herb: 'Herb',
  herbName: 'Herb name',
  otherHerb: 'Other (type name)',
  amount: 'Amount',
  unit: 'Unit',
  note: 'Note',
  addIngredient: 'Add ingredient',
  removeIngredient: 'Remove ingredient',
  doctor: 'Doctor',
  patientGender: 'Patient gender',
  patientAgeRange: 'Patient age range',
  condition: 'Condition',
  treatment: 'Treatment',
  result: 'Result',
  resultCured: 'Cured',
  resultBetter: 'Better',
  resultNoChange: 'No change',
  duration: 'Duration',
  recipe: 'Recipe',
  ageRange0to12: '0-12',
  ageRange13to19: '13-19',
  ageRange20to39: '20-39',
  ageRange40to59: '40-59',
  ageRange60plus: '60+',
  home: 'Home',
  welcome: 'Welcome to Phum Panya',
  publicNav: 'Public menu',
  search: 'Search',
  allDistricts: 'All districts',
  allHerbs: 'All herbs',
  print: 'Print',
  doctorNotFound: 'Doctor not found',
  exportCsv: 'Export CSV',
  exportExcel: 'Export Excel',
  storage: 'Storage',
  storageUsed: 'Used',
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
