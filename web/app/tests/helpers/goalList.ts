import type { Page } from '@playwright/test';

// null selects every status; [] deliberately selects none.
export async function setStatusFilter(page: Page, labels: string[] | null) {
  await page.getByRole('button', { name: 'Filter statuses', exact: true }).click();
  const dropdown = page.locator('.ant-table-filter-dropdown:visible');
  if (labels === null) {
    await dropdown.getByRole('checkbox', { name: 'Select all', exact: true }).check();
  } else {
    const none = dropdown.getByRole('button', { name: 'Select none', exact: true });
    if (await none.isEnabled()) await none.click();
    for (const label of labels) await dropdown.getByText(label, { exact: true }).click();
  }
  await dropdown.getByRole('button', { name: 'Apply', exact: true }).click();
}
