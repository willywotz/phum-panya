import { type Page, expect } from '@playwright/test';

// Drives a shadcn/Radix Select: open the combobox by its accessible name,
// then pick the option by its visible label.
export async function selectByName(page: Page, name: string, optionLabel: string) {
  await page.getByRole('combobox', { name }).click();
  await page.getByRole('option', { name: optionLabel, exact: true }).click();
  await expect(page.getByRole('combobox', { name })).toContainText(optionLabel);
}
