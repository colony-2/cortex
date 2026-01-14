# Diff Viewer for Chapter Artifacts - Implementation Spec

## Overview

Enhance the chapter artifact viewer to automatically detect and display `.diff` files using a dedicated diff viewer component, rather than plain text or hex modes.

## Current State

**Location:** `/src/web/app/src/components/WorkflowDetailPage.tsx`

The artifact viewer currently supports two display modes:
- **Text mode**: Displays raw artifact content as plain text
- **Hex mode**: Displays content as a formatted hexdump (offset | hex bytes | ASCII)

**Current Implementation Details:**
- View mode stored in state: `const [viewMode, setViewMode] = useState<'text' | 'hex'>('text')`
- Radio button toggle at line 420-425 switches between modes
- Modal displays artifact content in a monospace-styled div (lines 427-449)
- Artifacts fetched via `loadArtifact()` function (lines 77-104) as blob → text
- Artifact metadata includes `name` and `artifact_type` (MIME type from backend)

## Goals

1. Auto-detect `.diff` files by filename extension
2. Display diff content with syntax highlighting, line numbers, and color-coded changes
3. Maintain existing text/hex modes for other file types
4. Provide seamless integration with existing Ant Design UI
5. Support unified diff format (standard git diff output)

## Technical Approach

### 1. Library Selection

**Recommended: `react-diff-view`**
- NPM: https://www.npmjs.com/package/react-diff-view
- GitHub: https://github.com/otakustay/react-diff-view
- Bundle size: ~50KB minified
- License: MIT

**Why `react-diff-view`:**
- Mature library (1M+ weekly downloads)
- Excellent React integration
- Supports unified and split diff views
- Customizable styling (works with Ant Design theme)
- Syntax highlighting via `refractor` or `highlight.js`
- No jQuery or legacy dependencies
- Built-in features:
  - Line-by-line diff rendering
  - Code folding for large diffs
  - Hunk headers
  - Change type indicators (+/- lines)
  - Widget system for inline comments (future extensibility)

**Alternative considered:**
- `react-diff-viewer`: Simpler but less flexible, limited styling options
- `diff2html`: Heavier, generates HTML strings (not React components)

### 2. View Mode Enhancement

**Expand view mode enum:**
```typescript
const [viewMode, setViewMode] = useState<'text' | 'hex' | 'diff'>('text');
```

**Auto-detection logic:**
```typescript
const shouldUseDiffViewer = (artifactName: string): boolean => {
  return artifactName.toLowerCase().endsWith('.diff') ||
         artifactName.toLowerCase().endsWith('.patch');
};
```

**View mode initialization:**
```typescript
const loadArtifact = async (artifact: ArtifactReference) => {
  // ... existing fetch logic ...

  // Auto-select diff mode for .diff files
  const defaultMode = shouldUseDiffViewer(artifact.name) ? 'diff' : 'text';
  setViewMode(defaultMode);

  // ... rest of function ...
};
```

### 3. Component Structure

**New component: `DiffViewer.tsx`**
```typescript
import { parseDiff, Diff, Hunk, tokenize } from 'react-diff-view';
import 'react-diff-view/style/index.css';

interface DiffViewerProps {
  diffText: string;
  fileName?: string;
}

export const DiffViewer: React.FC<DiffViewerProps> = ({ diffText, fileName }) => {
  // Parse the diff text into structured data
  const files = parseDiff(diffText);

  // Optional: Add syntax highlighting tokens
  const tokens = files.map(file => tokenize(file.hunks));

  return (
    <div className="diff-viewer-container">
      {files.map((file, index) => (
        <Diff
          key={file.oldRevision + '-' + file.newRevision}
          viewType="unified"  // or "split"
          diffType={file.type}
          hunks={file.hunks}
          tokens={tokens[index]}
        >
          {hunks => hunks.map(hunk => (
            <Hunk key={hunk.content} hunk={hunk} />
          ))}
        </Diff>
      ))}
    </div>
  );
};
```

### 4. Modal Update

**Modified artifact modal (lines 393-452):**
```typescript
<Modal
  open={selectedArtifact !== null}
  onCancel={() => setSelectedArtifact(null)}
  width="90%"
  title={selectedArtifact?.name}
  footer={[
    // Only show view mode toggle if not in diff mode
    !shouldUseDiffViewer(selectedArtifact?.name || '') && (
      <Radio.Group
        key="view-mode"
        value={viewMode}
        onChange={(e) => setViewMode(e.target.value)}
      >
        <Radio.Button value="text">Text</Radio.Button>
        <Radio.Button value="hex">Hex</Radio.Button>
      </Radio.Group>
    ),
    <Button
      key="download"
      icon={<DownloadOutlined />}
      onClick={() => downloadArtifact(selectedArtifact!)}
    >
      Download
    </Button>,
    <Button key="close" onClick={() => setSelectedArtifact(null)}>
      Close
    </Button>,
  ]}
>
  {artifactContent && (
    shouldUseDiffViewer(selectedArtifact?.name || '') ? (
      <DiffViewer
        diffText={artifactContent}
        fileName={selectedArtifact?.name}
      />
    ) : (
      <div
        style={{
          fontFamily: 'monospace',
          whiteSpace: 'pre-wrap',
          wordBreak: 'break-all',
          maxHeight: '70vh',
          overflow: 'auto',
          padding: '16px',
          backgroundColor: '#f5f5f5',
        }}
      >
        {viewMode === 'hex' ? formatHex(artifactContent) : artifactContent}
      </div>
    )
  )}
</Modal>
```

### 5. Styling Integration

**CSS customization for Ant Design theme:**
```css
/* In WorkflowDetailPage.tsx or separate CSS file */
.diff-viewer-container {
  background: #ffffff;
  border: 1px solid #d9d9d9;
  border-radius: 2px;
  max-height: 70vh;
  overflow: auto;
}

/* Override react-diff-view defaults to match Ant Design */
.diff-viewer-container .diff-gutter {
  background: #fafafa;
}

.diff-viewer-container .diff-gutter-insert {
  background: #f6ffed;
  border-color: #b7eb8f;
}

.diff-viewer-container .diff-gutter-delete {
  background: #fff1f0;
  border-color: #ffccc7;
}

.diff-viewer-container .diff-code-insert {
  background: #f6ffed;
}

.diff-viewer-container .diff-code-delete {
  background: #fff1f0;
}

/* Hunk headers */
.diff-viewer-container .diff-hunk-header {
  background: #fafafa;
  color: rgba(0, 0, 0, 0.85);
  border-color: #d9d9d9;
}
```

## Implementation Steps

### Phase 1: Setup (30 min)
1. Install dependencies:
   ```bash
   npm install react-diff-view
   npm install --save-dev @types/react-diff-view
   ```
2. Import CSS in WorkflowDetailPage.tsx:
   ```typescript
   import 'react-diff-view/style/index.css';
   ```

### Phase 2: Create DiffViewer Component (1 hour)
3. Create `/src/web/app/src/components/DiffViewer.tsx`
4. Implement basic diff parsing and rendering
5. Add error handling for malformed diffs
6. Style to match Ant Design theme

### Phase 3: Integrate with WorkflowDetailPage (1.5 hours)
7. Add diff detection logic (`shouldUseDiffViewer`)
8. Update `viewMode` type to include 'diff'
9. Modify `loadArtifact` to auto-select diff mode
10. Update modal rendering logic with conditional DiffViewer
11. Hide text/hex toggle when viewing diffs

### Phase 4: Testing (1 hour)
12. Test with sample .diff files
13. Test with malformed diffs (graceful degradation)
14. Test view mode switching on non-diff files
15. Test download functionality for diff files
16. Verify styling consistency with Ant Design

### Phase 5: Polish (30 min)
17. Add loading states if diff parsing is slow
18. Consider adding "View as Text" fallback button for diffs
19. Update any relevant documentation

## File Changes Summary

**New Files:**
- `/src/web/app/src/components/DiffViewer.tsx` - New diff viewer component
- `/src/web/app/src/components/DiffViewer.css` (optional) - Diff viewer styles

**Modified Files:**
- `/src/web/app/src/components/WorkflowDetailPage.tsx`:
  - Import DiffViewer component
  - Add `shouldUseDiffViewer()` helper
  - Update `viewMode` type definition
  - Modify `loadArtifact()` to auto-detect
  - Update modal rendering logic
  - Update footer button logic

- `/src/web/app/package.json`:
  - Add `react-diff-view` dependency
  - Add `@types/react-diff-view` dev dependency

## UI/UX Considerations

### User Experience
1. **Auto-detection**: Diff files automatically open in diff view (no manual toggle needed)
2. **Fallback option**: Consider adding a "View as Text" button in footer for debugging malformed diffs
3. **View mode memory**: Don't persist diff mode - always auto-detect on artifact open
4. **Performance**: For very large diffs (>10k lines), consider:
   - Code folding (react-diff-view supports this)
   - Virtualization for rendering only visible hunks
   - Warning message before rendering

### Visual Design
1. **Color scheme**: Match Ant Design's success/error colors:
   - Additions: `#52c41a` (green)
   - Deletions: `#ff4d4f` (red)
   - Context: `rgba(0, 0, 0, 0.85)` (default text)
2. **Typography**: Use same monospace font as text/hex modes (`monospace` or `'Courier New'`)
3. **Spacing**: Consistent padding (16px) with other view modes

### Edge Cases
1. **Malformed diffs**: If `parseDiff()` throws error, fallback to text mode with error message
2. **Empty diffs**: Show "No changes" message
3. **Binary file diffs**: Display metadata only, offer download
4. **Multi-file diffs**: Stack files vertically with file headers

## Testing Approach

### Unit Tests
- Test `shouldUseDiffViewer()` with various filenames:
  - `test.diff` → true
  - `test.patch` → true
  - `test.DIFF` → true (case insensitive)
  - `test.txt` → false
  - `diff.txt` → false

### Integration Tests
- Test DiffViewer with sample diffs:
  - Simple unified diff (1 file, 1 hunk)
  - Multi-file diff (git diff output)
  - Diff with binary files
  - Malformed diff (missing headers)

### Manual Testing Checklist
- [ ] .diff file opens in diff view automatically
- [ ] Additions shown in green with + prefix
- [ ] Deletions shown in red with - prefix
- [ ] Context lines shown in default color
- [ ] Hunk headers visible and styled
- [ ] Line numbers accurate
- [ ] Download button works for .diff files
- [ ] Can still view .diff as text/hex if needed
- [ ] Large diffs (1000+ lines) render without lag
- [ ] Modal scrolling works smoothly
- [ ] Styling matches Ant Design theme

## Backend Considerations

**No backend changes required** for MVP. The backend already:
- Serves artifact content via `/api/projects/{id}/workflows/{id}/chapters/{num}/artifacts/{name}`
- Returns raw content with appropriate Content-Type header
- Includes artifact metadata (name, type, size) in chapter details

**Future Enhancement:**
Consider setting `artifact_type` to `text/x-diff` or `text/x-patch` for diff files in the backend (currently likely `text/plain`). This would enable MIME-type-based detection as an alternative to filename checking.

**Location to update (if needed):**
- `/src/server/workflow/internal/service/service.go` line 472: `artType := art.ContentType()`
- Artifact storage layer would need to detect and set MIME type during artifact creation

## Future Enhancements

### Short-term (next iteration)
1. **Split view option**: Add toggle between unified/split view
2. **Syntax highlighting**: Use `refractor` to add language-specific syntax highlighting
3. **View as text fallback**: Add button to force text mode for debugging

### Medium-term
1. **Inline comments**: Use react-diff-view's widget system for annotations
2. **Diff statistics**: Show total additions/deletions count in modal header
3. **File tree navigation**: For multi-file diffs, add sidebar with file list
4. **Search in diff**: Add search functionality for large diffs

### Long-term
1. **Side-by-side comparison**: For non-diff files, allow comparing two artifacts
2. **Diff generation**: Generate diffs between artifact versions
3. **Apply patch**: Button to apply diff to workspace (if applicable)

## Dependencies

```json
{
  "dependencies": {
    "react-diff-view": "^3.2.1"
  },
  "devDependencies": {
    "@types/react-diff-view": "^3.2.0"
  }
}
```

**Peer dependencies** (already in project):
- `react`: ^18.x
- `react-dom`: ^18.x

**Optional peer dependencies** (for syntax highlighting):
- `refractor`: ^4.x (if adding syntax highlighting later)

## Migration Path

This is a purely additive feature:
1. No breaking changes to existing functionality
2. Text/hex modes continue to work unchanged for non-diff files
3. No API changes required
4. No database migrations needed
5. Backward compatible with existing artifacts

## Success Metrics

1. **Functional**: .diff files display with proper syntax highlighting and colors
2. **Performance**: Diffs up to 5000 lines render in <500ms
3. **UX**: Users don't need to manually select diff mode (auto-detection works)
4. **Quality**: No regressions in text/hex mode functionality
5. **Accessibility**: Diff viewer maintains keyboard navigation and screen reader support

## Open Questions

1. **Should we support split view?** (side-by-side comparison)
   - Recommendation: Start with unified only, add split later if requested

2. **Should we add syntax highlighting for diff content?**
   - Recommendation: Start without, add in follow-up PR if needed
   - Would require `refractor` dependency and language detection

3. **Should we detect diffs by MIME type instead of filename?**
   - Recommendation: Use filename for MVP, consider MIME type as enhancement
   - Would require backend changes to properly set Content-Type

4. **How to handle very large diffs (>10k lines)?**
   - Recommendation: Start with warning message + code folding
   - Add virtualization if performance issues arise in practice

5. **Should non-.diff files be viewable in diff mode?**
   - Recommendation: No, restrict to .diff/.patch extensions only
   - Prevents confusion and ensures proper diff format validation

## References

- `react-diff-view` documentation: https://github.com/otakustay/react-diff-view#readme
- Current implementation: `/src/web/app/src/components/WorkflowDetailPage.tsx:53-452`
- Artifact API handler: `/src/server/api/internal/handlers/workflows.go:130-178`
- Unified diff format spec: https://www.gnu.org/software/diffutils/manual/html_node/Detailed-Unified.html
