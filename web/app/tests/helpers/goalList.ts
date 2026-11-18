import { expect, type Page } from '@playwright/test';

// null selects every option; [] deliberately selects none.
export async function setStatusFilter(page: Page, labels: string[] | null) {
  await setColumnFilter(page, 'Filter statuses', labels);
}

export async function setCellFilter(page: Page, labels: string[] | null) {
  await setColumnFilter(page, 'Filter cells', labels);
}

async function setColumnFilter(page: Page, name: string, labels: string[] | null) {
  const dropdown = page.locator('.ant-table-filter-dropdown:visible');
  // Let the previous column's popup finish closing before opening another.
  await expect(dropdown).toHaveCount(0);
  await page.getByRole('button', { name, exact: true }).click();
  await expect(dropdown).toHaveCount(1);
  if (labels === null) {
    await dropdown.getByRole('checkbox', { name: 'Select all', exact: true }).check();
  } else {
    // Establish a checked state before clearing. A freshly opened filter can
    // briefly disable Reset while Ant Design restores its selected keys.
    await dropdown.getByRole('checkbox', { name: 'Select all', exact: true }).check();
    const none = dropdown.getByRole('button', { name: 'Select none', exact: true });
    await none.click();
    await expect(none).toBeDisabled();
    for (const label of labels) await dropdown.getByText(label, { exact: true }).click();
  }
  await dropdown.getByRole('button', { name: 'Apply', exact: true }).click();
  await expect(dropdown).toHaveCount(0);
}
