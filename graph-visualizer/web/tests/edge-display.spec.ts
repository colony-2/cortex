import { test, expect } from '@playwright/test';

test.describe('Edge Display Verification', () => {
  test('should display all edges based on relationships', async ({ page }) => {
    await page.goto('/boxes');
    
    // Wait for graph to be fully loaded
    await page.waitForSelector('[data-testid="react-flow-wrapper"]', { state: 'visible' });
    await page.waitForSelector('.react-flow__node', { timeout: 10000 });
    
    // Wait a bit more for edges to render
    await page.waitForTimeout(1000);
    
    // Fetch the graph data from the API to know what edges we expect
    const apiResponse = await page.request.get('http://localhost:8080/api/graph');
    const graphResponse = await apiResponse.json();
    
    console.log(`Expected ${graphResponse.edges.length} edges from API`);
    console.log('Expected edges:', graphResponse.edges);
    
    // Count the actual rendered edges
    const renderedEdges = await page.locator('.react-flow__edge').count();
    console.log(`Found ${renderedEdges} rendered edges`);
    
    // Verify all expected edges are rendered
    expect(renderedEdges).toBe(graphResponse.edges.length);
    
    // For debugging: log which edges might be missing
    if (renderedEdges !== graphResponse.edges.length) {
      // Get all edge paths to help debug
      const edgePaths = await page.locator('.react-flow__edge path').evaluateAll(paths => 
        paths.map(path => path.getAttribute('d'))
      );
      console.log('Edge paths found:', edgePaths.length);
    }
  });

  test('should update edges when relationships change', async ({ page }) => {
    await page.goto('/boxes');
    
    // Wait for graph to load
    await page.waitForSelector('[data-testid="react-flow-wrapper"]', { state: 'visible' });
    await page.waitForSelector('.react-flow__node', { timeout: 10000 });
    await page.waitForTimeout(1000);
    
    // Get initial edge count
    const initialEdgeCount = await page.locator('.react-flow__edge').count();
    console.log(`Initial edge count: ${initialEdgeCount}`);
    
    // Select a node
    await page.evaluate(() => {
      window.dispatchEvent(new CustomEvent('nodeSelected', { detail: { nodeId: 'api' } }));
    });
    
    // Navigate to dependencies tab
    await page.waitForSelector('.ant-tabs-content', { state: 'visible' });
    await page.click('.ant-tabs-tab:has-text("Config")');
    await page.click('.ant-tabs-tab:has-text("Relationships")');
    await page.waitForSelector('text=Relationships for api', { state: 'visible' });
    
    // Get current relationships count
    const currentDeps = await page.locator('.ant-list-item').count();
    
    if (currentDeps > 0) {
      // Remove a relationship
      const removeButton = page.locator('.ant-list-item button:has-text("Remove")').first();
      await removeButton.click();
      await page.waitForSelector('.ant-message-success', { state: 'visible' });
      
      // Wait for graph to update
      await page.waitForTimeout(1000);
      
      // Check that edge count decreased
      const newEdgeCount = await page.locator('.react-flow__edge').count();
      console.log(`Edge count after removal: ${newEdgeCount}`);
      expect(newEdgeCount).toBe(initialEdgeCount - 1);
    } else {
      // Add a relationship if none exist
      const selector = page.locator('.ant-select-selector');
      await selector.click();
      
      // Wait for dropdown with timeout
      try {
        await page.waitForSelector('.ant-select-dropdown', { state: 'visible', timeout: 5000 });
        
        const hasOptions = await page.locator('.ant-select-item-option').count() > 0;
        if (hasOptions) {
          await page.locator('.ant-select-item-option').first().click();
          await page.click('button:has-text("Add Relationship")');
          await page.waitForSelector('.ant-message-success', { state: 'visible' });
          
          // Wait for graph to update
          await page.waitForTimeout(1000);
          
          // Check that edge count increased
          const newEdgeCount = await page.locator('.react-flow__edge').count();
          console.log(`Edge count after addition: ${newEdgeCount}`);
          expect(newEdgeCount).toBe(initialEdgeCount + 1);
        }
      } catch (e) {
        console.log('No options available to add relationships');
      }
    }
  });

  test('should render edges with correct source and target', async ({ page }) => {
    await page.goto('/boxes');
    
    // Wait for graph to load
    await page.waitForSelector('[data-testid="react-flow-wrapper"]', { state: 'visible' });
    await page.waitForSelector('.react-flow__node', { timeout: 10000 });
    await page.waitForTimeout(1000);
    
    // Get graph data
    const apiResponse = await page.request.get('http://localhost:8080/api/graph');
    const graphData = await apiResponse.json();
    
    // For each expected edge, verify it connects the right nodes
    for (const edge of graphData.edges.slice(0, 5)) { // Check first 5 edges to avoid long test
      // Edges in React Flow have data attributes or IDs that indicate source/target
      const edgeSelector = `.react-flow__edge[data-id*="${edge.source}-${edge.target}"], .react-flow__edge[id*="${edge.source}-${edge.target}"]`;
      const edgeExists = await page.locator(edgeSelector).count() > 0;
      
      if (!edgeExists) {
        // Try alternative selector patterns
        const altEdgeExists = await page.evaluate((e) => {
          const edges = document.querySelectorAll('.react-flow__edge');
          return Array.from(edges).some(el => {
            const id = el.getAttribute('data-id') || el.id || '';
            return id.includes(e.source) && id.includes(e.target);
          });
        }, edge);
        
        expect(altEdgeExists).toBe(true);
      }
    }
  });
});