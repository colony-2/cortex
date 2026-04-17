import { test, expect } from '@playwright/test';

test.describe('Cortex Serve', () => {
  test.beforeEach(async ({ context }) => {
    await context.addCookies([
      {
        name: 'colony2:user:email',
        value: 'test@example.com',
        url: 'http://localhost:8080',
      },
    ]);
  });

  test('loads the project admin page without console errors', async ({ page }) => {
    // Capture console errors
    const errors: string[] = [];
    page.on('console', msg => {
      if (msg.type() === 'error') {
        errors.push(msg.text());
      }
    });

    // Capture page errors
    page.on('pageerror', err => {
      errors.push(err.message);
    });

    await page.goto('http://localhost:8080/admin/projects', { waitUntil: 'networkidle' });

    await expect(
      page.getByRole('heading', { name: 'Project Administration' })
    ).toBeVisible({ timeout: 10000 });
    await expect(page.getByRole('button', { name: 'New Project' })).toBeVisible();

    // Check that there are no JavaScript errors
    if (errors.length > 0) {
      console.error('JavaScript errors found:', errors);
    }
    expect(errors, `Found ${errors.length} JavaScript errors: ${errors.join(', ')}`).toHaveLength(0);

    const projectsResponse = await page.request.get('http://localhost:8080/api/projects');
    expect(projectsResponse.ok(), 'Projects API should return 200').toBe(true);
    const projects = await projectsResponse.json();
    expect(projects).toEqual([]);
  });

  test('shows the empty-state shell when no project is selected', async ({ page }) => {
    await page.goto('http://localhost:8080/cells', { waitUntil: 'networkidle' });

    await expect(
      page.getByText('Select or create a project to continue')
    ).toBeVisible({ timeout: 10000 });
    await expect(page.getByText('colony2')).toBeVisible();
  });
});
