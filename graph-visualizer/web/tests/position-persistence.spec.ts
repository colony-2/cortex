import { test, expect } from '@playwright/test';

test.describe('Node Position Persistence', () => {
  test('should save node positions when dragged', async ({ page }) => {
    await page.goto('/');
    
    // Wait for the graph to load
    await page.waitForSelector('.react-flow', { timeout: 10000 });
    
    // Wait for nodes to be rendered
    await page.waitForSelector('.react-flow__node', { timeout: 10000 });
    
    // Get the first node
    const firstNode = page.locator('.react-flow__node').first();
    await expect(firstNode).toBeVisible();
    
    // Get initial position
    const initialBox = await firstNode.boundingBox();
    expect(initialBox).toBeTruthy();
    
    // Drag the node to a new position
    await firstNode.dragTo(firstNode, {
      targetPosition: { x: 100, y: 100 }
    });
    
    // Wait for position save API call
    await page.waitForResponse(response => 
      response.url().includes('/api/positions') && 
      response.request().method() === 'POST',
      { timeout: 2000 }
    );
    
    // Get new position
    const newBox = await firstNode.boundingBox();
    expect(newBox).toBeTruthy();
    
    // Verify position changed
    expect(newBox!.x).not.toBe(initialBox!.x);
    expect(newBox!.y).not.toBe(initialBox!.y);
  });

  test('should restore saved node positions on reload', async ({ page }) => {
    await page.goto('/');
    
    // Wait for the graph to load
    await page.waitForSelector('.react-flow', { timeout: 10000 });
    
    // Wait for nodes to be rendered
    await page.waitForSelector('.react-flow__node', { timeout: 10000 });
    
    // Get the first node and its position
    const firstNode = page.locator('.react-flow__node').first();
    const initialBox = await firstNode.boundingBox();
    expect(initialBox).toBeTruthy();
    
    // Drag the node to a new position (far from initial)
    const newX = initialBox!.x + 200;
    const newY = initialBox!.y + 200;
    
    await page.mouse.move(initialBox!.x + initialBox!.width / 2, initialBox!.y + initialBox!.height / 2);
    await page.mouse.down();
    await page.mouse.move(newX, newY);
    await page.mouse.up();
    
    // Wait for position save
    await page.waitForResponse(response => 
      response.url().includes('/api/positions') && 
      response.request().method() === 'POST',
      { timeout: 2000 }
    );
    
    // Get position after drag
    const draggedBox = await firstNode.boundingBox();
    expect(draggedBox).toBeTruthy();
    
    // Reload the page
    await page.reload();
    
    // Wait for the graph to load again
    await page.waitForSelector('.react-flow', { timeout: 10000 });
    await page.waitForTimeout(2000); // Wait for positions to be applied
    
    // Wait for nodes to be rendered
    await page.waitForSelector('.react-flow__node', { timeout: 10000 });
    
    // Get the first node position after reload
    const reloadedNode = page.locator('.react-flow__node').first();
    const reloadedBox = await reloadedNode.boundingBox();
    expect(reloadedBox).toBeTruthy();
    
    // The position should be close to where we dragged it (within 30 pixels tolerance for React Flow transforms)
    expect(Math.abs(reloadedBox!.x - draggedBox!.x)).toBeLessThan(30);
    expect(Math.abs(reloadedBox!.y - draggedBox!.y)).toBeLessThan(30);
  });

  test('should maintain relative positions when multiple nodes are moved', async ({ page }) => {
    await page.goto('/');
    
    // Wait for the graph to load
    await page.waitForSelector('.react-flow', { timeout: 10000 });
    await page.waitForTimeout(1000); // Wait for graph to stabilize
    
    // Wait for nodes to be rendered
    await page.waitForSelector('.react-flow__node', { timeout: 10000 });
    
    // Get positions of first two nodes
    const firstNode = page.locator('.react-flow__node').nth(0);
    const secondNode = page.locator('.react-flow__node').nth(1);
    
    const firstInitial = await firstNode.boundingBox();
    const secondInitial = await secondNode.boundingBox();
    
    expect(firstInitial).toBeTruthy();
    expect(secondInitial).toBeTruthy();
    
    // Calculate initial distance between nodes
    const initialDistance = Math.sqrt(
      Math.pow(secondInitial!.x - firstInitial!.x, 2) + 
      Math.pow(secondInitial!.y - firstInitial!.y, 2)
    );
    
    // Drag both nodes
    await firstNode.dragTo(firstNode, {
      targetPosition: { x: 50, y: 50 }
    });
    
    await page.waitForTimeout(600); // Wait for save timeout
    
    await secondNode.dragTo(secondNode, {
      targetPosition: { x: 50, y: 50 }
    });
    
    // Wait for position saves
    await page.waitForTimeout(600);
    
    // Reload
    await page.reload();
    
    // Wait for everything to load
    await page.waitForSelector('.react-flow', { timeout: 10000 });
    await page.waitForTimeout(2000); // Wait for positions to be applied
    await page.waitForSelector('.react-flow__node', { timeout: 10000 });
    
    // Get reloaded positions
    const firstReloaded = page.locator('.react-flow__node').nth(0);
    const secondReloaded = page.locator('.react-flow__node').nth(1);
    
    const firstNewBox = await firstReloaded.boundingBox();
    const secondNewBox = await secondReloaded.boundingBox();
    
    // Both nodes should have moved from their initial positions
    expect(firstNewBox!.x).not.toBe(firstInitial!.x);
    expect(firstNewBox!.y).not.toBe(firstInitial!.y);
    expect(secondNewBox!.x).not.toBe(secondInitial!.x);
    expect(secondNewBox!.y).not.toBe(secondInitial!.y);
  });
});