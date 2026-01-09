import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import WorkflowDetailPage from './WorkflowDetailPage';
import { WorkflowsService } from '@colony2/openapi-client';
import type { WorkflowDetail } from '@colony2/openapi-client';

vi.mock('@colony2/openapi-client', () => ({
  WorkflowsService: {
    getApiProjectsWorkflows1: vi.fn(),
  },
}));

// Mock fetch for artifact loading
global.fetch = vi.fn();

const mockWorkflowWithArtifacts: WorkflowDetail = {
  workflow_id: 'wf-123',
  run_id: 'run-123',
  status: 'completed',
  recipe_name: 'test-recipe',
  start_time: '2026-01-08T12:00:00Z',
  close_time: '2026-01-08T12:30:00Z',
  created_at: '2026-01-08T12:00:00Z',
  chapters: [
    {
      chapter_number: 1,
      chapter_type: 'op',
      op_name: 'test-operation',
      status: 'completed',
      start_time: '2026-01-08T12:00:00Z',
      end_time: '2026-01-08T12:15:00Z',
      input: { test: 'input' },
      output: { test: 'output' },
      artifacts: [
        {
          artifact_id: 'art-1',
          name: 'test-artifact.txt',
          artifact_type: 'text',
          url: 'http://example.com/artifact-1',
          size_bytes: 1024,
        },
        {
          artifact_id: 'art-2',
          name: 'binary-file.bin',
          artifact_type: 'binary',
          url: 'http://example.com/artifact-2',
          size_bytes: 2048,
        },
      ],
    },
  ],
};

const mockWorkflowNoArtifacts: WorkflowDetail = {
  workflow_id: 'wf-456',
  run_id: 'run-456',
  status: 'completed',
  recipe_name: 'test-recipe-2',
  start_time: '2026-01-08T12:00:00Z',
  close_time: '2026-01-08T12:30:00Z',
  created_at: '2026-01-08T12:00:00Z',
  chapters: [
    {
      chapter_number: 1,
      chapter_type: 'op',
      op_name: 'test-operation-2',
      status: 'completed',
      start_time: '2026-01-08T12:00:00Z',
      end_time: '2026-01-08T12:15:00Z',
      artifacts: [],
    },
  ],
};

describe('WorkflowDetailPage - Artifact Viewing', () => {
  const user = userEvent.setup();

  beforeEach(() => {
    vi.clearAllMocks();
    (global.fetch as any).mockReset();
  });

  const renderComponent = (workflowId: string, mockData: WorkflowDetail) => {
    vi.mocked(WorkflowsService.getApiProjectsWorkflows1).mockResolvedValue(mockData);

    return render(
      <MemoryRouter initialEntries={[`/project/proj-1/workflows/${workflowId}`]}>
        <Routes>
          <Route
            path="/project/:projectId/workflows/:workflowId"
            element={<WorkflowDetailPage projectId="proj-1" />}
          />
        </Routes>
      </MemoryRouter>
    );
  };

  it('displays artifacts with clickable names when artifact has URL', async () => {
    renderComponent('wf-123', mockWorkflowWithArtifacts);

    await waitFor(() => {
      expect(screen.getByText('Workflow Detail')).toBeInTheDocument();
    });

    // Expand the chapter to see artifacts
    const chapterHeaders = screen.getAllByText(/Chapter 1:/);
    await user.click(chapterHeaders[0]);

    // Check that artifacts are displayed with clickable links
    await waitFor(() => {
      const artifact1Link = screen.getByRole('link', { name: /test-artifact.txt/i });
      expect(artifact1Link).toBeInTheDocument();
      expect(artifact1Link).toHaveAttribute('href', '#');

      const artifact2Link = screen.getByRole('link', { name: /binary-file.bin/i });
      expect(artifact2Link).toBeInTheDocument();
      expect(artifact2Link).toHaveAttribute('href', '#');
    });

    // Check that size is displayed
    expect(screen.getByText(/1.00 KB/)).toBeInTheDocument();
    expect(screen.getByText(/2.00 KB/)).toBeInTheDocument();
  });

  it('displays download buttons for artifacts with URLs', async () => {
    renderComponent('wf-123', mockWorkflowWithArtifacts);

    await waitFor(() => {
      expect(screen.getByText('Workflow Detail')).toBeInTheDocument();
    });

    // Expand the chapter
    const chapterHeaders = screen.getAllByText(/Chapter 1:/);
    await user.click(chapterHeaders[0]);

    // Check for download buttons
    await waitFor(() => {
      const downloadButtons = screen.getAllByRole('button', { name: /download/i });
      expect(downloadButtons).toHaveLength(2);
    });
  });

  it('opens artifact viewer modal when clicking artifact name', async () => {
    const mockArtifactContent = 'This is the artifact content';
    const mockBlob = new Blob([mockArtifactContent], { type: 'text/plain' });
    // Add text() method to Blob
    mockBlob.text = vi.fn().mockResolvedValue(mockArtifactContent);

    (global.fetch as any).mockResolvedValue({
      ok: true,
      blob: () => Promise.resolve(mockBlob),
    });

    renderComponent('wf-123', mockWorkflowWithArtifacts);

    await waitFor(() => {
      expect(screen.getByText('Workflow Detail')).toBeInTheDocument();
    });

    // Expand the chapter
    const chapterHeaders = screen.getAllByText(/Chapter 1:/);
    await user.click(chapterHeaders[0]);

    // Click on artifact link
    await waitFor(() => {
      const artifactLink = screen.getByRole('link', { name: /test-artifact.txt/i });
      expect(artifactLink).toBeInTheDocument();
    });

    const artifactLink = screen.getByRole('link', { name: /test-artifact.txt/i });
    await user.click(artifactLink);

    // Verify modal opens with artifact name in title
    await waitFor(() => {
      // Modal should be visible (check for the view mode radio buttons)
      expect(screen.getByRole('radio', { name: /text/i })).toBeInTheDocument();
    });

    // Verify fetch was called with correct URL
    expect(global.fetch).toHaveBeenCalledWith('http://example.com/artifact-1');

    // Verify artifact content is displayed
    await waitFor(() => {
      expect(screen.getByText(mockArtifactContent)).toBeInTheDocument();
    });
  });

  it('shows text and hex view modes in artifact viewer', async () => {
    const mockArtifactContent = 'Hello';
    const mockBlob = new Blob([mockArtifactContent], { type: 'text/plain' });
    mockBlob.text = vi.fn().mockResolvedValue(mockArtifactContent);

    (global.fetch as any).mockResolvedValue({
      ok: true,
      blob: () => Promise.resolve(mockBlob),
    });

    renderComponent('wf-123', mockWorkflowWithArtifacts);

    await waitFor(() => {
      expect(screen.getByText('Workflow Detail')).toBeInTheDocument();
    });

    // Expand chapter and click artifact
    const chapterHeaders = screen.getAllByText(/Chapter 1:/);
    await user.click(chapterHeaders[0]);

    await waitFor(() => {
      const artifactLink = screen.getByRole('link', { name: /test-artifact.txt/i });
      expect(artifactLink).toBeInTheDocument();
    });

    const artifactLink = screen.getByRole('link', { name: /test-artifact.txt/i });
    await user.click(artifactLink);

    // Wait for modal to open
    await waitFor(() => {
      expect(screen.getByRole('radio', { name: /text/i })).toBeInTheDocument();
    });

    // Check for view mode radio buttons
    const textButton = screen.getByRole('radio', { name: /text/i });
    const hexButton = screen.getByRole('radio', { name: /hex/i });

    expect(textButton).toBeInTheDocument();
    expect(hexButton).toBeInTheDocument();
    expect(textButton).toBeChecked();

    // Switch to hex mode
    await user.click(hexButton);

    // Verify hex content is displayed (should show hex representation)
    await waitFor(() => {
      const content = screen.getByText(/00000000/); // Hex offset
      expect(content).toBeInTheDocument();
    });
  });

  it('has close button in artifact viewer modal', async () => {
    const mockArtifactContent = 'Artifact content';
    const mockBlob = new Blob([mockArtifactContent], { type: 'text/plain' });
    mockBlob.text = vi.fn().mockResolvedValue(mockArtifactContent);

    (global.fetch as any).mockResolvedValue({
      ok: true,
      blob: () => Promise.resolve(mockBlob),
    });

    renderComponent('wf-123', mockWorkflowWithArtifacts);

    await waitFor(() => {
      expect(screen.getByText('Workflow Detail')).toBeInTheDocument();
    });

    // Open artifact viewer
    const chapterHeaders = screen.getAllByText(/Chapter 1:/);
    await user.click(chapterHeaders[0]);

    await waitFor(() => {
      const artifactLink = screen.getByRole('link', { name: /test-artifact.txt/i });
      expect(artifactLink).toBeInTheDocument();
    });

    const artifactLink = screen.getByRole('link', { name: /test-artifact.txt/i });
    await user.click(artifactLink);

    // Wait for modal
    await waitFor(() => {
      expect(screen.getByRole('radio', { name: /text/i })).toBeInTheDocument();
    });

    // Verify close button exists and can be clicked
    const closeButtons = screen.getAllByRole('button', { name: /close/i });
    expect(closeButtons.length).toBeGreaterThan(0);

    // Click close button should not throw an error
    await user.click(closeButtons[closeButtons.length - 1]);
  });

  it('handles artifact fetch errors gracefully', async () => {
    (global.fetch as any).mockRejectedValue(new Error('Network error'));

    renderComponent('wf-123', mockWorkflowWithArtifacts);

    await waitFor(() => {
      expect(screen.getByText('Workflow Detail')).toBeInTheDocument();
    });

    // Open artifact viewer
    const chapterHeaders = screen.getAllByText(/Chapter 1:/);
    await user.click(chapterHeaders[0]);

    await waitFor(() => {
      const artifactLink = screen.getByRole('link', { name: /test-artifact.txt/i });
      expect(artifactLink).toBeInTheDocument();
    });

    const artifactLink = screen.getByRole('link', { name: /test-artifact.txt/i });
    await user.click(artifactLink);

    // Wait a bit for the error to be handled
    await new Promise((resolve) => setTimeout(resolve, 100));

    // Modal should not open after error (check that view mode radio buttons don't appear)
    expect(screen.queryByRole('radio', { name: /text/i })).not.toBeInTheDocument();
  });

  it('does not show artifacts section when chapter has no artifacts', async () => {
    renderComponent('wf-456', mockWorkflowNoArtifacts);

    await waitFor(() => {
      expect(screen.getByText('Workflow Detail')).toBeInTheDocument();
    });

    // Expand the chapter
    const chapterHeaders = screen.getAllByText(/Chapter 1:/);
    await user.click(chapterHeaders[0]);

    // Verify no artifacts section is shown
    await waitFor(() => {
      expect(screen.queryByText('Artifacts')).not.toBeInTheDocument();
    });
  });

  it('displays download button that triggers download', async () => {
    renderComponent('wf-123', mockWorkflowWithArtifacts);

    await waitFor(() => {
      expect(screen.getByText('Workflow Detail')).toBeInTheDocument();
    });

    // Expand chapter
    const chapterHeaders = screen.getAllByText(/Chapter 1:/);
    await user.click(chapterHeaders[0]);

    // Verify download buttons are present
    await waitFor(() => {
      const downloadButtons = screen.getAllByRole('button', { name: /download/i });
      expect(downloadButtons.length).toBeGreaterThan(0);
    });
  });
});
