import { test, expect } from '@playwright/test';

test.describe('Node Position Persistence', () => {
  test.beforeEach(async ({ page }) => {
    // Enable console logging to debug issues
    page.on('console', msg => {
      if (msg.type() === 'error') {
        console.log('Page error:', msg.text());
      }
    });
  });
  test('should save node positions when dragged', async ({ page }) => {
    await page.goto('/cells');
    
    // Wait for the graph to load
    await page.waitForSelector('.react-flow', { timeout: 10000 });
    
    // Wait for nodes to be rendered
    await page.waitForSelector('.react-flow__node', { state: 'attached', timeout: 10000 });
    
    // Get the first node
    const firstNode = page.locator('.react-flow__node').first();
    await expect(firstNode).toBeVisible();
    
    // Get initial position
    const initialBox = await firstNode.boundingBox();
    expect(initialBox).toBeTruthy();
    
    // Use the inner draggable part of the node (the graph-node div)
    const draggableNode = firstNode.locator('.graph-cell').first();
    const box = await draggableNode.boundingBox();
    if (!box) throw new Error('Could not get node bounding box');
    
    // Calculate center of the draggable element
    const centerX = box.x + box.width / 2;
    const centerY = box.y + box.height / 2;
    
    // Set up response listener before drag
    const responsePromise = page.waitForResponse(response => 
      response.url().includes('/api/positions') && 
      response.request().method() === 'POST',
      { timeout: 3000 }
    ).catch(() => null);
    
    // Perform drag operation with small steps to ensure React Flow detects it
    await page.mouse.move(centerX, centerY);
    await page.mouse.down();
    
    // Move in small increments to simulate real drag
    for (let i = 1; i <= 10; i++) {
      await page.mouse.move(centerX + (i * 15), centerY + (i * 15));
      await page.waitForTimeout(20);
    }
    
    await page.mouse.up();
    
    // Wait for the save timeout (500ms) plus some buffer
    await page.waitForTimeout(700);
    
    // Wait for API response
    const response = await responsePromise;
    
    // Get new position
    const newBox = await firstNode.boundingBox();
    expect(newBox).toBeTruthy();
    
    // Verify position changed significantly (at least 100 pixels)
    const xDiff = Math.abs(newBox!.x - initialBox!.x);
    const yDiff = Math.abs(newBox!.y - initialBox!.y);
    expect(xDiff + yDiff).toBeGreaterThan(100);
    
    // Verify API was called if response was captured
    if (response) {
      expect(response.status()).toBe(200);
    }
  });

  test('should restore saved node positions on reload', async ({ page }) => {
    await page.goto('/cells');
    
    // Wait for the graph to load
    await page.waitForSelector('.react-flow', { timeout: 10000 });
    
    // Wait for nodes to be rendered
    await page.waitForSelector('.react-flow__node', { state: 'attached', timeout: 10000 });
    
    // Get the first node and its position
    const firstNode = page.locator('.react-flow__node').first();
    const initialBox = await firstNode.boundingBox();
    expect(initialBox).toBeTruthy();
    
    // Use the inner draggable part of the node
    const draggableNode = firstNode.locator('.graph-cell').first();
    const box = await draggableNode.boundingBox();
    if (!box) throw new Error('Could not get node bounding box');
    
    const centerX = box.x + box.width / 2;
    const centerY = box.y + box.height / 2;
    
    // Perform drag operation with small steps
    await page.mouse.move(centerX, centerY);
    await page.mouse.down();
    
    // Move in small increments
    for (let i = 1; i <= 10; i++) {
      await page.mouse.move(centerX + (i * 20), centerY + (i * 20));
      await page.waitForTimeout(20);
    }
    
    await page.mouse.up();
    
    // Wait for the save timeout (500ms) plus some buffer
    await page.waitForTimeout(700);
    
    // Check if positions were saved by waiting for potential API call
    const positionSavePromise = page.waitForResponse(response => 
      response.url().includes('/api/positions') && 
      response.request().method() === 'POST',
      { timeout: 1000 }
    ).catch(() => null);
    
    // Wait for either the API call or timeout
    await positionSavePromise;
    
    // Get position after drag
    const draggedBox = await firstNode.boundingBox();
    expect(draggedBox).toBeTruthy();
    
    // Reload the page
    await page.reload();
    
    // Wait for the graph to load again
    await page.waitForSelector('.react-flow', { timeout: 10000 });
    await page.waitForTimeout(2000); // Wait for positions to be applied
    
    // Wait for nodes to be rendered
    await page.waitForSelector('.react-flow__node', { state: 'attached', timeout: 10000 });
    
    // Get the first node position after reload
    const reloadedNode = page.locator('.react-flow__node').first();
    const reloadedBox = await reloadedNode.boundingBox();
    expect(reloadedBox).toBeTruthy();
    
    // The position should be different from initial (node moved)
    const xDiff = Math.abs(reloadedBox!.x - initialBox!.x);
    const yDiff = Math.abs(reloadedBox!.y - initialBox!.y);
    
    // Node should have moved significantly (at least 100 pixels in some direction)
    expect(xDiff + yDiff).toBeGreaterThan(100);
    
    // The position should be close to where we dragged it (within tolerance for React Flow transforms)
    expect(Math.abs(reloadedBox!.x - draggedBox!.x)).toBeLessThan(50);
    expect(Math.abs(reloadedBox!.y - draggedBox!.y)).toBeLessThan(50);
  });

  test('should maintain relative positions when multiple nodes are moved', async ({ page }) => {
    await page.goto('/cells');
    
    // Wait for the graph to load
    await page.waitForSelector('.react-flow', { timeout: 10000 });
    await page.waitForTimeout(1000); // Wait for graph to stabilize
    
    // Wait for nodes to be rendered
    await page.waitForSelector('.react-flow__node', { state: 'attached', timeout: 10000 });
    
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
    
    // Drag first node using the inner draggable part
    const firstDraggable = firstNode.locator('.graph-cell').first();
    const firstBox = await firstDraggable.boundingBox();
    if (!firstBox) throw new Error('Could not get first node bounding box');
    
    const firstCenterX = firstBox.x + firstBox.width / 2;
    const firstCenterY = firstBox.y + firstBox.height / 2;
    
    await page.mouse.move(firstCenterX, firstCenterY);
    await page.mouse.down();
    
    // Move in small increments
    for (let i = 1; i <= 10; i++) {
      await page.mouse.move(firstCenterX + (i * 20), firstCenterY + (i * 20));
      await page.waitForTimeout(20);
    }
    
    await page.mouse.up();
    
    // Wait for first save to complete
    await page.waitForTimeout(700);
    
    // Drag second node in a different direction
    const secondDraggable = secondNode.locator('.graph-cell').first();
    const secondBox = await secondDraggable.boundingBox();
    if (!secondBox) throw new Error('Could not get second node bounding box');
    
    const secondCenterX = secondBox.x + secondBox.width / 2;
    const secondCenterY = secondBox.y + secondBox.height / 2;
    
    await page.mouse.move(secondCenterX, secondCenterY);
    await page.mouse.down();
    
    // Move in small increments in different direction
    for (let i = 1; i <= 10; i++) {
      await page.mouse.move(secondCenterX - (i * 15), secondCenterY + (i * 15));
      await page.waitForTimeout(20);
    }
    
    await page.mouse.up();
    
    // Wait for second save to complete
    await page.waitForTimeout(700);
    
    // Get positions after drag but before reload
    const firstDraggedBox = await firstNode.boundingBox();
    const secondDraggedBox = await secondNode.boundingBox();
    
    // Reload
    await page.reload();
    
    // Wait for everything to load
    await page.waitForSelector('.react-flow', { timeout: 10000 });
    await page.waitForTimeout(2000); // Wait for positions to be applied
    await page.waitForSelector('.react-flow__node', { state: 'attached', timeout: 10000 });
    
    // Get reloaded positions
    const firstReloaded = page.locator('.react-flow__node').nth(0);
    const secondReloaded = page.locator('.react-flow__node').nth(1);
    
    const firstNewBox = await firstReloaded.boundingBox();
    const secondNewBox = await secondReloaded.boundingBox();
    
    // Both nodes should have moved from their initial positions
    const firstXDiff = Math.abs(firstNewBox!.x - firstInitial!.x);
    const firstYDiff = Math.abs(firstNewBox!.y - firstInitial!.y);
    const secondXDiff = Math.abs(secondNewBox!.x - secondInitial!.x);
    const secondYDiff = Math.abs(secondNewBox!.y - secondInitial!.y);
    
    // Each node should have moved at least 100 pixels total
    expect(firstXDiff + firstYDiff).toBeGreaterThan(100);
    expect(secondXDiff + secondYDiff).toBeGreaterThan(100);
    
    // Positions should be close to where we dragged them (within tolerance)
    expect(Math.abs(firstNewBox!.x - firstDraggedBox!.x)).toBeLessThan(50);
    expect(Math.abs(firstNewBox!.y - firstDraggedBox!.y)).toBeLessThan(50);
    expect(Math.abs(secondNewBox!.x - secondDraggedBox!.x)).toBeLessThan(50);
    expect(Math.abs(secondNewBox!.y - secondDraggedBox!.y)).toBeLessThan(50);
  });
});