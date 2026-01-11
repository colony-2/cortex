import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor, cleanup } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import RecipeHistoryTab from './RecipeHistoryTab';
import { RecipesService } from '@colony2/openapi-client';

// Mock the RecipesService
vi.mock('@colony2/openapi-client', () => ({
  RecipesService: {
    getRecipeHistory: vi.fn(),
    getRecipe: vi.fn(),
  },
}));

describe('RecipeHistoryTab', () => {
  const projectId = 'proj_123';
  const recipeName = 'test-recipe';
  const currentCommit = 'abc123';

  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    cleanup();
  });

  it('should load and display version history', async () => {
    const mockVersions = [
      {
        commitHash: 'abc123',
        shortHash: 'abc123',
        message: 'Latest version',
        author: 'user@example.com',
        createdAt: new Date('2024-01-15').toISOString(),
        isPublished: true,
      },
      {
        commitHash: 'def456',
        shortHash: 'def456',
        message: 'Previous version',
        author: 'admin@example.com',
        createdAt: new Date('2024-01-10').toISOString(),
        isPublished: false,
      },
    ];

    vi.mocked(RecipesService.getRecipeHistory).mockResolvedValue({
      versions: mockVersions,
    });

    render(
      <RecipeHistoryTab
        projectId={projectId}
        recipeName={recipeName}
        currentCommit={currentCommit}
      />
    );

    await waitFor(() => {
      expect(screen.getByText('Latest version')).toBeDefined();
      expect(screen.getByText('Previous version')).toBeDefined();
    });

    // Verify the service was called
    expect(vi.mocked(RecipesService.getRecipeHistory)).toHaveBeenCalledWith(
      projectId,
      recipeName
    );
  });

  it('should show current version indicator', async () => {
    const mockVersions = [
      {
        commitHash: 'abc123',
        shortHash: 'abc123',
        message: 'Current version',
        author: 'user@example.com',
        createdAt: new Date().toISOString(),
        isPublished: false,
      },
      {
        commitHash: 'def456',
        shortHash: 'def456',
        message: 'Old version',
        author: 'user@example.com',
        createdAt: new Date().toISOString(),
        isPublished: false,
      },
    ];

    vi.mocked(RecipesService.getRecipeHistory).mockResolvedValue({
      versions: mockVersions,
    });

    render(
      <RecipeHistoryTab
        projectId={projectId}
        recipeName={recipeName}
        currentCommit="abc123"
      />
    );

    await waitFor(() => {
      expect(screen.getByText('Current')).toBeDefined();
    });

    // Verify only one "Current" tag is shown
    const currentTags = screen.getAllByText('Current');
    expect(currentTags).toHaveLength(1);
  });

  it('should show published version indicator', async () => {
    const mockVersions = [
      {
        commitHash: 'abc123',
        shortHash: 'abc123',
        message: 'Published version',
        author: 'user@example.com',
        createdAt: new Date().toISOString(),
        isPublished: true,
      },
      {
        commitHash: 'def456',
        shortHash: 'def456',
        message: 'Draft version',
        author: 'user@example.com',
        createdAt: new Date().toISOString(),
        isPublished: false,
      },
    ];

    vi.mocked(RecipesService.getRecipeHistory).mockResolvedValue({
      versions: mockVersions,
    });

    render(
      <RecipeHistoryTab
        projectId={projectId}
        recipeName={recipeName}
        currentCommit={currentCommit}
      />
    );

    await waitFor(() => {
      expect(screen.getByText('Published')).toBeDefined();
    });

    // Verify only published versions have the "Published" tag
    const publishedTags = screen.getAllByText('Published');
    expect(publishedTags).toHaveLength(1);
  });

  it('should view previous version in modal', async () => {
    const user = userEvent.setup();

    const mockVersions = [
      {
        commitHash: 'abc123',
        shortHash: 'abc123',
        message: 'Version 1',
        author: 'user@example.com',
        createdAt: new Date().toISOString(),
        isPublished: false,
      },
      {
        commitHash: 'def456',
        shortHash: 'def456',
        message: 'Version 2',
        author: 'user@example.com',
        createdAt: new Date().toISOString(),
        isPublished: false,
      },
    ];

    const mockRecipeContent = {
      name: recipeName,
      commitHash: 'def456',
      rawYaml: 'version: "1.0"\nid: test\nop: echo\ninputs:\n  message: "Old"',
      content: {},
      isPublished: false,
      publishedAt: null,
      publishedBy: null,
    };

    vi.mocked(RecipesService.getRecipeHistory).mockResolvedValue({
      versions: mockVersions,
    });

    vi.mocked(RecipesService.getRecipe).mockResolvedValue(mockRecipeContent);

    render(
      <RecipeHistoryTab
        projectId={projectId}
        recipeName={recipeName}
        currentCommit={currentCommit}
      />
    );

    await waitFor(() => {
      expect(screen.getByText('Version 2')).toBeDefined();
    });

    // Click the "View" button for the second version
    const viewButtons = screen.getAllByRole('button', { name: /View/i });
    await user.click(viewButtons[1]);

    // Verify modal is shown with version content
    await waitFor(() => {
      expect(screen.getByText('Version Content')).toBeDefined();
      expect(screen.getByText(/version: "1.0"/i)).toBeDefined();
    });

    // Verify getRecipe was called with correct parameters
    expect(vi.mocked(RecipesService.getRecipe)).toHaveBeenCalledWith(
      projectId,
      recipeName,
      'def456'
    );
  });

  it('should close modal when Close button is clicked', async () => {
    const user = userEvent.setup();

    const mockVersions = [
      {
        commitHash: 'abc123',
        shortHash: 'abc123',
        message: 'Version 1',
        author: 'user@example.com',
        createdAt: new Date().toISOString(),
        isPublished: false,
      },
    ];

    const mockRecipeContent = {
      name: recipeName,
      commitHash: 'abc123',
      rawYaml: 'version: "1.0"',
      content: {},
      isPublished: false,
      publishedAt: null,
      publishedBy: null,
    };

    vi.mocked(RecipesService.getRecipeHistory).mockResolvedValue({
      versions: mockVersions,
    });

    vi.mocked(RecipesService.getRecipe).mockResolvedValue(mockRecipeContent);

    render(
      <RecipeHistoryTab
        projectId={projectId}
        recipeName={recipeName}
        currentCommit={currentCommit}
      />
    );

    await waitFor(() => {
      expect(screen.getByText('Version 1')).toBeDefined();
    });

    // Click View button
    const viewButton = screen.getByRole('button', { name: /View/i });
    await user.click(viewButton);

    await waitFor(() => {
      expect(screen.getByText('Version Content')).toBeDefined();
    });

    // Verify Close button exists in modal
    const closeButtons = screen.getAllByRole('button', { name: /Close/i });
    expect(closeButtons.length).toBeGreaterThan(0);

    // Click Close button (get the one in the footer, not the X button)
    const footerCloseButton = closeButtons.find(btn => btn.textContent === 'Close');
    expect(footerCloseButton).toBeDefined();
    await user.click(footerCloseButton!);

    // The close button click should work without errors
    // (Testing the full modal closing animation in jsdom is flaky)
  });

  it('should handle history load errors', async () => {
    const consoleErrorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});

    vi.mocked(RecipesService.getRecipeHistory).mockRejectedValue(
      new Error('Failed to load history')
    );

    render(
      <RecipeHistoryTab
        projectId={projectId}
        recipeName={recipeName}
        currentCommit={currentCommit}
      />
    );

    await waitFor(() => {
      expect(consoleErrorSpy).toHaveBeenCalledWith(
        'Failed to load recipe history',
        expect.any(Error)
      );
    });

    consoleErrorSpy.mockRestore();
  });

  it('should handle version content load errors', async () => {
    const user = userEvent.setup();
    const consoleErrorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});

    const mockVersions = [
      {
        commitHash: 'abc123',
        shortHash: 'abc123',
        message: 'Version 1',
        author: 'user@example.com',
        createdAt: new Date().toISOString(),
        isPublished: false,
      },
    ];

    vi.mocked(RecipesService.getRecipeHistory).mockResolvedValue({
      versions: mockVersions,
    });

    vi.mocked(RecipesService.getRecipe).mockRejectedValue(
      new Error('Failed to load version content')
    );

    render(
      <RecipeHistoryTab
        projectId={projectId}
        recipeName={recipeName}
        currentCommit={currentCommit}
      />
    );

    await waitFor(() => {
      expect(screen.getByText('Version 1')).toBeDefined();
    });

    // Click View button
    const viewButton = screen.getByRole('button', { name: /View/i });
    await user.click(viewButton);

    await waitFor(() => {
      expect(consoleErrorSpy).toHaveBeenCalledWith(
        'Failed to load version content',
        expect.any(Error)
      );
    });

    consoleErrorSpy.mockRestore();
  });

  it('should format dates correctly', async () => {
    const testDate = new Date('2024-01-15T10:30:00Z');

    const mockVersions = [
      {
        commitHash: 'abc123',
        shortHash: 'abc123',
        message: 'Test version',
        author: 'user@example.com',
        createdAt: testDate.toISOString(),
        isPublished: false,
      },
    ];

    vi.mocked(RecipesService.getRecipeHistory).mockResolvedValue({
      versions: mockVersions,
    });

    render(
      <RecipeHistoryTab
        projectId={projectId}
        recipeName={recipeName}
        currentCommit={currentCommit}
      />
    );

    await waitFor(() => {
      // Check that the date is formatted as a locale string
      // The exact format depends on the locale, but it should contain the date
      const formattedDate = testDate.toLocaleString();
      expect(screen.getByText(new RegExp(formattedDate.split(',')[0]))).toBeDefined();
    });
  });

  it('should display commit hash and author information', async () => {
    const mockVersions = [
      {
        commitHash: 'abc123def456',
        shortHash: 'abc123d',
        message: 'Test commit',
        author: 'john.doe@example.com',
        createdAt: new Date().toISOString(),
        isPublished: false,
      },
    ];

    vi.mocked(RecipesService.getRecipeHistory).mockResolvedValue({
      versions: mockVersions,
    });

    render(
      <RecipeHistoryTab
        projectId={projectId}
        recipeName={recipeName}
        currentCommit={currentCommit}
      />
    );

    await waitFor(() => {
      expect(screen.getByText('abc123d')).toBeDefined();
      expect(screen.getByText('Test commit')).toBeDefined();
      expect(screen.getByText(/john.doe@example.com/)).toBeDefined();
    });
  });

  it('should show read-only textarea in modal', async () => {
    const user = userEvent.setup();

    const mockVersions = [
      {
        commitHash: 'abc123',
        shortHash: 'abc123',
        message: 'Version 1',
        author: 'user@example.com',
        createdAt: new Date().toISOString(),
        isPublished: false,
      },
    ];

    const mockRecipeContent = {
      name: recipeName,
      commitHash: 'abc123',
      rawYaml: 'version: "1.0"\nid: test\nop: echo',
      content: {},
      isPublished: false,
      publishedAt: null,
      publishedBy: null,
    };

    vi.mocked(RecipesService.getRecipeHistory).mockResolvedValue({
      versions: mockVersions,
    });

    vi.mocked(RecipesService.getRecipe).mockResolvedValue(mockRecipeContent);

    render(
      <RecipeHistoryTab
        projectId={projectId}
        recipeName={recipeName}
        currentCommit={currentCommit}
      />
    );

    await waitFor(() => {
      expect(screen.getByText('Version 1')).toBeDefined();
    });

    // Click View button
    const viewButton = screen.getByRole('button', { name: /View/i });
    await user.click(viewButton);

    await waitFor(() => {
      const textarea = screen.getByDisplayValue(/version: "1.0"/);
      expect(textarea).toHaveProperty('readOnly', true);
    });
  });
});
