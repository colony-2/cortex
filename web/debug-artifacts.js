// Debug script for artifact viewing
// Copy and paste this into the browser console on a workflow detail page

(function() {
  console.log('=== Artifact Viewing Debug Info ===\n');

  // 1. Check if we're on the workflow detail page
  const url = window.location.href;
  console.log('1. Current URL:', url);
  console.log('   Is workflow detail page:', url.includes('/workflows/'));

  // 2. Look for chapter collapse panels
  const collapsePanel = document.querySelector('.ant-collapse');
  console.log('\n2. Collapse panel found:', !!collapsePanel);

  // 3. Look for any expanded chapters
  const expandedPanels = document.querySelectorAll('.ant-collapse-item-active');
  console.log('\n3. Expanded chapters:', expandedPanels.length);

  // 4. Look for "Artifacts" text/heading
  const artifactsText = Array.from(document.querySelectorAll('*')).find(el =>
    el.textContent.trim() === 'Artifacts' && el.tagName !== 'SCRIPT'
  );
  console.log('\n4. "Artifacts" heading found:', !!artifactsText);
  if (artifactsText) {
    console.log('   Parent element:', artifactsText.parentElement?.className);
  }

  // 5. Look for artifact names/links
  const allLinks = document.querySelectorAll('a');
  console.log('\n5. Total links on page:', allLinks.length);

  const artifactLinks = Array.from(allLinks).filter(link =>
    link.href === '#' || link.textContent.includes('.') || link.closest('.ant-card')
  );
  console.log('   Potential artifact links:', artifactLinks.length);
  artifactLinks.forEach((link, i) => {
    console.log(`   Link ${i}:`, link.textContent.trim(), 'href:', link.href);
  });

  // 6. Look for download buttons
  const downloadButtons = Array.from(document.querySelectorAll('button')).filter(btn =>
    btn.textContent.includes('Download') || btn.querySelector('[aria-label="download"]')
  );
  console.log('\n6. Download buttons found:', downloadButtons.length);

  // 7. Check for any elements with "artifact" in className or id
  const artifactElements = document.querySelectorAll('[class*="artifact" i], [id*="artifact" i]');
  console.log('\n7. Elements with "artifact" in class/id:', artifactElements.length);

  // 8. Look inside cards for artifact-like content
  const cards = document.querySelectorAll('.ant-card');
  console.log('\n8. Cards on page:', cards.length);
  cards.forEach((card, i) => {
    const title = card.querySelector('.ant-card-head-title');
    if (title && title.textContent.includes('Artifacts')) {
      console.log(`   Card ${i} is Artifacts card`);
      console.log('   Card HTML:', card.innerHTML.substring(0, 500));
    }
  });

  // 9. Check the React component tree (if React DevTools data available)
  const rootElement = document.querySelector('#root, [data-reactroot]');
  console.log('\n9. React root element found:', !!rootElement);

  // 10. Look for any modal-related elements
  const modals = document.querySelectorAll('.ant-modal');
  console.log('\n10. Modals on page:', modals.length);

  // 11. Get the full HTML of the chapters section
  const chaptersCard = Array.from(cards).find(card => {
    const title = card.querySelector('.ant-card-head-title');
    return title && title.textContent.includes('Chapters');
  });

  if (chaptersCard) {
    console.log('\n11. Chapters card found. Expanding all chapters...');
    const chapterHeaders = chaptersCard.querySelectorAll('.ant-collapse-header');
    console.log('   Chapter headers to expand:', chapterHeaders.length);

    // Try to expand all chapters
    chapterHeaders.forEach((header, i) => {
      const panel = header.closest('.ant-collapse-item');
      const isActive = panel?.classList.contains('ant-collapse-item-active');
      console.log(`   Chapter ${i} is ${isActive ? 'expanded' : 'collapsed'}`);

      if (!isActive) {
        console.log(`   Clicking to expand chapter ${i}...`);
        header.click();
      }
    });

    // Wait a bit for expansion animation
    setTimeout(() => {
      console.log('\n12. After expansion, checking for artifacts...');

      const artifactsCards = Array.from(document.querySelectorAll('.ant-card')).filter(card => {
        const title = card.querySelector('.ant-card-head-title');
        return title && title.textContent.trim() === 'Artifacts';
      });

      console.log('   Artifacts cards found:', artifactsCards.length);

      artifactsCards.forEach((card, i) => {
        console.log(`\n   Artifacts Card ${i}:`);
        console.log('   Full HTML:', card.outerHTML);

        const cardBody = card.querySelector('.ant-card-body');
        if (cardBody) {
          console.log('   Card body HTML:', cardBody.innerHTML);
        }
      });

      // Look for any spans or divs that might contain artifact names
      const allText = document.body.innerText;
      const looksLikeFilename = allText.match(/\w+\.(txt|log|json|xml|bin|dat|yml|yaml|md)[^\w]/gi);
      console.log('\n13. Text that looks like filenames:', looksLikeFilename);

      console.log('\n=== Debug Complete ===');
      console.log('Please copy all of the above output and share it.');
    }, 500);
  } else {
    console.log('\n11. No Chapters card found!');
    console.log('\n=== Debug Complete ===');
    console.log('Please copy all of the above output and share it.');
  }
})();
