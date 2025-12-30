import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor, cleanup } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { BrowserRouter } from 'react-router-dom';
import RecipeListPage from './RecipeListPage';
import { RecipesService } from '@colony2/openapi-client';

// Mock the RecipesService
vi.mock('@colony2/openapi-client', () => ({
  RecipesService: {
    listRecipes: vi.fn(),
  },
}));

// Mock react-router-dom navigation
const mockNavigate = vi.fn();
vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual('react-router-dom');
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  };
});

describe('RecipeListPage', () => {
  const projectId = 'proj_123';

  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    cleanup();
  });

  it('should render recipe list with tree structure', async () => {
    const mockRecipes = [
      {
        name: 'workflows/build',
        latestCommit: 'abc123',
        latestCommitAt: new Date().toISOString(),
        publishedCommit: 'abc123',
        publishedAt: new Date().toISOString(),
        publishedBy: 'user@example.com',
      },
      {
        name: 'ops/deploy',
        latestCommit: 'def456',
        latestCommitAt: new Date().toISOString(),
        publishedCommit: null,
        publishedAt: null,
        publishedBy: null,
      },
    ];

    vi.mocked(RecipesService.listRecipes).mockResolvedValue({
      recipes: mockRecipes,
    });

    render(
      <BrowserRouter>
        <RecipeListPage projectId={projectId} />
      </BrowserRouter>
    );

    await waitFor(() => {
      expect(screen.getByText('build')).toBeDefined();
      expect(screen.getByText('deploy')).toBeDefined();
    });

    // Verify published status badges
    const publishedBadges = screen.getAllByText('Published');
    expect(publishedBadges.length).toBeGreaterThan(0);

    const draftBadges = screen.getAllByText('Draft');
    expect(draftBadges.length).toBeGreaterThan(0);
  });

  it('should filter recipes by status - all', async () => {
    const mockRecipes = [
      {
        name: 'recipe1',
        latestCommit: 'abc123',
        latestCommitAt: new Date().toISOString(),
        publishedCommit: 'abc123',
        publishedAt: new Date().toISOString(),
        publishedBy: 'user@example.com',
      },
      {
        name: 'recipe2',
        latestCommit: 'def456',
        latestCommitAt: new Date().toISOString(),
        publishedCommit: null,
        publishedAt: null,
        publishedBy: null,
      },
    ];

    vi.mocked(RecipesService.listRecipes).mockResolvedValue({
      recipes: mockRecipes,
    });

    render(
      <BrowserRouter>
        <RecipeListPage projectId={projectId} />
      </BrowserRouter>
    );

    await waitFor(() => {
      expect(screen.getByText('recipe1')).toBeDefined();
      expect(screen.getByText('recipe2')).toBeDefined();
    });

    // Verify the service was called with 'all' status
    expect(vi.mocked(RecipesService.listRecipes)).toHaveBeenCalledWith(
      projectId,
      'all'
    );
  });

  it('should filter recipes by status - published only', async () => {
    const user = userEvent.setup();
    const mockPublishedRecipes = [
      {
        name: 'published-recipe',
        latestCommit: 'abc123',
        latestCommitAt: new Date().toISOString(),
        publishedCommit: 'abc123',
        publishedAt: new Date().toISOString(),
        publishedBy: 'user@example.com',
      },
    ];

    vi.mocked(RecipesService.listRecipes).mockResolvedValue({
      recipes: mockPublishedRecipes,
    });

    render(
      <BrowserRouter>
        <RecipeListPage projectId={projectId} />
      </BrowserRouter>
    );

    await waitFor(() => {
      expect(screen.getByText('published-recipe')).toBeDefined();
    });

    // Find and click the status filter dropdown
    const statusSelect = screen.getByRole('combobox');
    await user.click(statusSelect);

    // Wait for dropdown to appear and select "Published" option
    // Ant Design renders options in a portal, use findByText with container: document.body
    await waitFor(async () => {
      const options = document.querySelectorAll('.ant-select-item-option-content');
      const publishedOption = Array.from(options).find(opt => opt.textContent === 'Published');
      if (publishedOption) {
        await user.click(publishedOption as Element);
      }
    });

    await waitFor(() => {
      expect(vi.mocked(RecipesService.listRecipes)).toHaveBeenCalledWith(
        projectId,
        'published'
      );
    });
  });

  it('should filter recipes by status - draft only', async () => {
    const user = userEvent.setup();
    const mockDraftRecipes = [
      {
        name: 'draft-recipe',
        latestCommit: 'def456',
        latestCommitAt: new Date().toISOString(),
        publishedCommit: null,
        publishedAt: null,
        publishedBy: null,
      },
    ];

    vi.mocked(RecipesService.listRecipes)
      .mockResolvedValueOnce({ recipes: [] }) // Initial load
      .mockResolvedValueOnce({ recipes: mockDraftRecipes }); // After filter change

    render(
      <BrowserRouter>
        <RecipeListPage projectId={projectId} />
      </BrowserRouter>
    );

    // Find and click the status filter dropdown
    const statusSelect = screen.getByRole('combobox');
    await user.click(statusSelect);

    // Wait for dropdown to appear and select "Draft" option
    await waitFor(async () => {
      const options = document.querySelectorAll('.ant-select-item-option-content');
      const draftOption = Array.from(options).find(opt => opt.textContent === 'Draft');
      if (draftOption) {
        await user.click(draftOption as Element);
      }
    });

    await waitFor(() => {
      expect(vi.mocked(RecipesService.listRecipes)).toHaveBeenCalledWith(
        projectId,
        'unpublished'
      );
    });
  });

  it('should search recipes by name', async () => {
    const user = userEvent.setup();
    const mockRecipes = [
      {
        name: 'workflows/build',
        latestCommit: 'abc123',
        latestCommitAt: new Date().toISOString(),
        publishedCommit: null,
        publishedAt: null,
        publishedBy: null,
      },
      {
        name: 'workflows/test',
        latestCommit: 'def456',
        latestCommitAt: new Date().toISOString(),
        publishedCommit: null,
        publishedAt: null,
        publishedBy: null,
      },
      {
        name: 'ops/deploy',
        latestCommit: 'ghi789',
        latestCommitAt: new Date().toISOString(),
        publishedCommit: null,
        publishedAt: null,
        publishedBy: null,
      },
    ];

    vi.mocked(RecipesService.listRecipes).mockResolvedValue({
      recipes: mockRecipes,
    });

    render(
      <BrowserRouter>
        <RecipeListPage projectId={projectId} />
      </BrowserRouter>
    );

    await waitFor(() => {
      expect(screen.getByText('build')).toBeDefined();
    });

    // Find search input
    const searchInput = screen.getByPlaceholderText('Search recipes...');

    // Type search query
    await user.type(searchInput, 'build');

    // Only 'build' should be visible, 'test' and 'deploy' should not
    expect(screen.getByText('build')).toBeDefined();
    expect(screen.queryByText('test')).toBeNull();
    expect(screen.queryByText('deploy')).toBeNull();
  });

  it('should navigate to recipe detail on click', async () => {
    const user = userEvent.setup();
    const mockRecipes = [
      {
        name: 'my-recipe',
        latestCommit: 'abc123',
        latestCommitAt: new Date().toISOString(),
        publishedCommit: null,
        publishedAt: null,
        publishedBy: null,
      },
    ];

    vi.mocked(RecipesService.listRecipes).mockResolvedValue({
      recipes: mockRecipes,
    });

    render(
      <BrowserRouter>
        <RecipeListPage projectId={projectId} />
      </BrowserRouter>
    );

    await waitFor(() => {
      expect(screen.getByText('my-recipe')).toBeDefined();
    });

    // Click on the recipe
    const recipeItem = screen.getByText('my-recipe');
    await user.click(recipeItem);

    // Verify navigation was called with correct URL
    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith(
        `/project/${projectId}/recipes/my-recipe`
      );
    });
  });

  it('should display "no recipes" message when list is empty', async () => {
    vi.mocked(RecipesService.listRecipes).mockResolvedValue({
      recipes: [],
    });

    render(
      <BrowserRouter>
        <RecipeListPage projectId={projectId} />
      </BrowserRouter>
    );

    await waitFor(() => {
      expect(
        screen.getByText('No recipes found. Create one to get started.')
      ).toBeDefined();
    });
  });

  it('should display "no matching recipes" message when search returns empty', async () => {
    const user = userEvent.setup();
    const mockRecipes = [
      {
        name: 'recipe1',
        latestCommit: 'abc123',
        latestCommitAt: new Date().toISOString(),
        publishedCommit: null,
        publishedAt: null,
        publishedBy: null,
      },
    ];

    vi.mocked(RecipesService.listRecipes).mockResolvedValue({
      recipes: mockRecipes,
    });

    render(
      <BrowserRouter>
        <RecipeListPage projectId={projectId} />
      </BrowserRouter>
    );

    await waitFor(() => {
      expect(screen.getByText('recipe1')).toBeDefined();
    });

    // Search for non-existent recipe
    const searchInput = screen.getByPlaceholderText('Search recipes...');
    await user.type(searchInput, 'nonexistent');

    await waitFor(() => {
      expect(screen.getByText('No recipes match your search')).toBeDefined();
    });
  });

  it('should handle API errors gracefully', async () => {
    const consoleErrorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});

    vi.mocked(RecipesService.listRecipes).mockRejectedValue(
      new Error('Network error')
    );

    render(
      <BrowserRouter>
        <RecipeListPage projectId={projectId} />
      </BrowserRouter>
    );

    // Wait for error to be logged
    await waitFor(() => {
      expect(consoleErrorSpy).toHaveBeenCalledWith(
        'Failed to load recipes',
        expect.any(Error)
      );
    });

    consoleErrorSpy.mockRestore();
  });

  it('should navigate to new recipe page on create button click', async () => {
    const user = userEvent.setup();

    vi.mocked(RecipesService.listRecipes).mockResolvedValue({
      recipes: [],
    });

    render(
      <BrowserRouter>
        <RecipeListPage projectId={projectId} />
      </BrowserRouter>
    );

    // Find and click "New Recipe" button
    const newButton = await screen.findByRole('button', { name: /New Recipe/i });
    await user.click(newButton);

    // Verify navigation to new recipe page
    expect(mockNavigate).toHaveBeenCalledWith(`/project/${projectId}/recipes/new`);
  });

  it('should refresh recipe list on refresh button click', async () => {
    const user = userEvent.setup();
    const mockRecipes = [
      {
        name: 'recipe1',
        latestCommit: 'abc123',
        latestCommitAt: new Date().toISOString(),
        publishedCommit: null,
        publishedAt: null,
        publishedBy: null,
      },
    ];

    vi.mocked(RecipesService.listRecipes).mockResolvedValue({
      recipes: mockRecipes,
    });

    render(
      <BrowserRouter>
        <RecipeListPage projectId={projectId} />
      </BrowserRouter>
    );

    await waitFor(() => {
      expect(screen.getByText('recipe1')).toBeDefined();
    });

    // Click refresh button
    const refreshButton = screen.getByRole('button', { name: /Refresh/i });
    await user.click(refreshButton);

    // Verify the service was called again
    await waitFor(() => {
      expect(vi.mocked(RecipesService.listRecipes)).toHaveBeenCalledTimes(2);
    });
  });
});
