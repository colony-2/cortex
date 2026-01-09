// Debug script to inspect artifact data from the API
// Run this in the browser console on a workflow detail page

(async function() {
  console.log('=== Artifact Data Debug ===\n');

  // Extract workflow ID from URL
  const url = window.location.href;
  const match = url.match(/\/workflows\/([^\/]+)/);

  if (!match) {
    console.log('ERROR: Not on a workflow detail page');
    return;
  }

  const workflowId = match[1];
  console.log('1. Workflow ID:', workflowId);

  // Extract project ID
  const projectMatch = url.match(/\/project\/([^\/]+)/);
  const projectId = projectMatch ? projectMatch[1] : null;
  console.log('   Project ID:', projectId);

  // Try to fetch the workflow data directly
  console.log('\n2. Fetching workflow data from API...');

  try {
    const apiUrl = `/api/projects/${projectId}/workflows/${workflowId}`;
    console.log('   API URL:', apiUrl);

    const response = await fetch(apiUrl);
    console.log('   Response status:', response.status, response.statusText);

    if (!response.ok) {
      console.log('   ERROR: Failed to fetch workflow data');
      return;
    }

    const workflowData = await response.json();
    console.log('\n3. Full workflow data:', workflowData);

    // Check if there are chapters
    if (!workflowData.chapters || workflowData.chapters.length === 0) {
      console.log('\n   No chapters found in workflow data');
      return;
    }

    console.log('\n4. Chapters found:', workflowData.chapters.length);

    // Iterate through chapters and look for artifacts
    workflowData.chapters.forEach((chapter, i) => {
      console.log(`\n   Chapter ${i} (${chapter.op_name || chapter.chapter_type}):`);

      if (!chapter.artifacts || chapter.artifacts.length === 0) {
        console.log('     No artifacts');
        return;
      }

      console.log('     Artifacts:', chapter.artifacts.length);

      chapter.artifacts.forEach((artifact, j) => {
        console.log(`\n     Artifact ${j}:`);
        console.log('       Full object:', artifact);
        console.log('       name:', artifact.name);
        console.log('       artifact_id:', artifact.artifact_id);
        console.log('       artifact_type:', artifact.artifact_type);
        console.log('       size_bytes:', artifact.size_bytes);
        console.log('       url:', artifact.url);
        console.log('       url is null/undefined:', artifact.url == null);
        console.log('       url is empty string:', artifact.url === '');

        // Check all properties
        console.log('       All properties:', Object.keys(artifact));
      });
    });

  } catch (error) {
    console.log('\n   ERROR fetching workflow data:', error);
  }

  console.log('\n=== Debug Complete ===');
})();
