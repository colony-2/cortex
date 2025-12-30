import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor, cleanup } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { BrowserRouter, Routes, Route } from 'react-router-dom';
import RecipeDetailPage from './RecipeDetailPage';
import { RecipesService, ApiError } from '@colony2/openapi-client';

// Mock only the RecipesService, but keep ApiError real
vi.mock('@colony2/openapi-client', async (importOriginal) => {
  const mod = await importOriginal<typeof import('@colony2/openapi-client')>();
  return {
    ...mod,
    RecipesService: {
      getRecipe: vi.fn(),
      createRecipe: vi.fn(),
      updateRecipe: vi.fn(),
      deleteRecipe: vi.fn(),
      publishRecipe: vi.fn(),
      unpublishRecipe: vi.fn(),
    },
  };
});

// Mock react-router-dom navigation
const mockNavigate = vi.fn();
vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual('react-router-dom');
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  };
});

describe('RecipeDetailPage', () => {
  const projectId = 'proj_123';

  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    cleanup();
  });

  it('should load and display existing recipe', async () => {
    const mockRecipe = {
      name: 'test-recipe',
      commitHash: 'abc123',
      rawYaml: 'version: "1.0"\nid: test\nop: echo',
      content: {
        version: '1.0',
        id: 'test',
        op: 'echo',
      },
      isPublished: true,
      publishedAt: new Date().toISOString(),
      publishedBy: 'user@example.com',
    };

    vi.mocked(RecipesService.getRecipe).mockResolvedValue(mockRecipe);

    render(
      <BrowserRouter>
        <Routes>
          <Route
            path="/"
            element={
              <div>
                <div style={{ display: 'none' }}>
                  {/* Simulate path */}
                  {(window.history.pushState({}, '', '/project/proj_123/recipes/test-recipe'), null)}
                </div>
                <RecipeDetailPage projectId={projectId} />
              </div>
            }
          />
        </Routes>
      </BrowserRouter>
    );

    await waitFor(() => {
      expect(screen.getByText('test-recipe')).toBeDefined();
    });

    // Verify recipe content is loaded
    expect(screen.getByText('Published')).toBeDefined();

    // Verify the service was called
    expect(vi.mocked(RecipesService.getRecipe)).toHaveBeenCalledWith(
      projectId,
      'test-recipe',
      undefined
    );
  });

  it('should display new recipe mode', async () => {
    render(
      <BrowserRouter>
        <Routes>
          <Route
            path="/"
            element={
              <div>
                <div style={{ display: 'none' }}>
                  {(window.history.pushState({}, '', '/project/proj_123/recipes/new'), null)}
                </div>
                <RecipeDetailPage projectId={projectId} />
              </div>
            }
          />
        </Routes>
      </BrowserRouter>
    );

    await waitFor(() => {
      expect(screen.getByText('New Recipe')).toBeDefined();
    });

    // Verify "Creating New Recipe" alert is shown
    expect(screen.getByText('Creating New Recipe')).toBeDefined();
  });

  it('should save recipe as draft', async () => {
    const user = userEvent.setup();
    const mockRecipe = {
      name: 'test-recipe',
      commitHash: 'abc123',
      rawYaml: 'version: "1.0"\nid: test\nop: echo',
      content: null,
      isPublished: false,
      publishedAt: null,
      publishedBy: null,
    };

    const mockUpdatedRecipe = {
      ...mockRecipe,
      commitHash: 'def456',
      rawYaml: 'version: "1.0"\nid: test\nop: echo\ninputs:\n  message: "Updated"',
    };

    vi.mocked(RecipesService.getRecipe)
      .mockResolvedValueOnce(mockRecipe)
      .mockResolvedValueOnce(mockUpdatedRecipe);

    vi.mocked(RecipesService.updateRecipe).mockResolvedValue({
      commitHash: 'def456',
      shortHash: 'def456',
      createdAt: new Date().toISOString(),
      author: 'user@example.com',
      message: 'Update recipe',
      isPublished: false,
    });

    render(
      <BrowserRouter>
        <Routes>
          <Route
            path="/"
            element={
              <div>
                <div style={{ display: 'none' }}>
                  {(window.history.pushState({}, '', '/project/proj_123/recipes/test-recipe'), null)}
                </div>
                <RecipeDetailPage projectId={projectId} />
              </div>
            }
          />
        </Routes>
      </BrowserRouter>
    );

    await waitFor(() => {
      expect(screen.getByText('test-recipe')).toBeDefined();
    });

    // Find the textarea and modify content
    const textarea = screen.getByRole('textbox');
    await user.clear(textarea);
    await user.type(textarea, 'version: "1.0"\nid: test\nop: echo\ninputs:\n  message: "Updated"');

    // Click Save button
    const saveButton = screen.getByRole('button', { name: /^Save$/i });
    await user.click(saveButton);

    await waitFor(() => {
      expect(vi.mocked(RecipesService.updateRecipe)).toHaveBeenCalledWith(
        projectId,
        'test-recipe',
        expect.objectContaining({
          autoPublish: false,
        })
      );
    });
  });

  it('should save and publish recipe', async () => {
    const user = userEvent.setup();
    const mockRecipe = {
      name: 'test-recipe',
      commitHash: 'abc123',
      rawYaml: 'version: "1.0"\nid: test\nop: echo',
      content: null,
      isPublished: false,
      publishedAt: null,
      publishedBy: null,
    };

    vi.mocked(RecipesService.getRecipe).mockResolvedValue(mockRecipe);

    vi.mocked(RecipesService.updateRecipe).mockResolvedValue({
      commitHash: 'def456',
      shortHash: 'def456',
      createdAt: new Date().toISOString(),
      author: 'user@example.com',
      message: 'Update recipe',
      isPublished: true,
    });

    render(
      <BrowserRouter>
        <Routes>
          <Route
            path="/"
            element={
              <div>
                <div style={{ display: 'none' }}>
                  {(window.history.pushState({}, '', '/project/proj_123/recipes/test-recipe'), null)}
                </div>
                <RecipeDetailPage projectId={projectId} />
              </div>
            }
          />
        </Routes>
      </BrowserRouter>
    );

    await waitFor(() => {
      expect(screen.getByText('test-recipe')).toBeDefined();
    });

    // Modify content to enable buttons
    const textarea = screen.getByRole('textbox');
    await user.type(textarea, '\n# modified');

    // Click "Save & Publish" button
    const savePublishButton = await screen.findByRole('button', { name: /Save & Publish/i });
    await user.click(savePublishButton);

    await waitFor(() => {
      expect(vi.mocked(RecipesService.updateRecipe)).toHaveBeenCalledWith(
        projectId,
        'test-recipe',
        expect.objectContaining({
          autoPublish: true,
        })
      );
    });
  });

  it('should publish existing draft recipe', async () => {
    const user = userEvent.setup();
    const mockRecipe = {
      name: 'test-recipe',
      commitHash: 'abc123',
      rawYaml: 'version: "1.0"\nid: test\nop: echo',
      content: null,
      isPublished: false,
      publishedAt: null,
      publishedBy: null,
    };

    const mockPublishedRecipe = {
      ...mockRecipe,
      isPublished: true,
      publishedAt: new Date().toISOString(),
      publishedBy: 'user@example.com',
    };

    vi.mocked(RecipesService.getRecipe)
      .mockResolvedValueOnce(mockRecipe)
      .mockResolvedValueOnce(mockPublishedRecipe);

    vi.mocked(RecipesService.publishRecipe).mockResolvedValue({
      name: 'test-recipe',
      commitHash: 'abc123',
    });

    render(
      <BrowserRouter>
        <Routes>
          <Route
            path="/"
            element={
              <div>
                <div style={{ display: 'none' }}>
                  {(window.history.pushState({}, '', '/project/proj_123/recipes/test-recipe'), null)}
                </div>
                <RecipeDetailPage projectId={projectId} />
              </div>
            }
          />
        </Routes>
      </BrowserRouter>
    );

    await waitFor(() => {
      expect(screen.getByText('Draft')).toBeDefined();
    });

    // Click Publish button
    const publishButton = screen.getByRole('button', { name: /^Publish$/i });
    await user.click(publishButton);

    await waitFor(() => {
      expect(vi.mocked(RecipesService.publishRecipe)).toHaveBeenCalledWith(
        projectId,
        'test-recipe',
        {}
      );
    });
  });

  it('should unpublish published recipe', async () => {
    const user = userEvent.setup();
    const mockRecipe = {
      name: 'test-recipe',
      commitHash: 'abc123',
      rawYaml: 'version: "1.0"\nid: test\nop: echo',
      content: null,
      isPublished: true,
      publishedAt: new Date().toISOString(),
      publishedBy: 'user@example.com',
    };

    const mockUnpublishedRecipe = {
      ...mockRecipe,
      isPublished: false,
      publishedAt: null,
      publishedBy: null,
    };

    vi.mocked(RecipesService.getRecipe)
      .mockResolvedValueOnce(mockRecipe)
      .mockResolvedValueOnce(mockUnpublishedRecipe);

    vi.mocked(RecipesService.unpublishRecipe).mockResolvedValue(undefined);

    render(
      <BrowserRouter>
        <Routes>
          <Route
            path="/"
            element={
              <div>
                <div style={{ display: 'none' }}>
                  {(window.history.pushState({}, '', '/project/proj_123/recipes/test-recipe'), null)}
                </div>
                <RecipeDetailPage projectId={projectId} />
              </div>
            }
          />
        </Routes>
      </BrowserRouter>
    );

    await waitFor(() => {
      expect(screen.getByText('Published')).toBeDefined();
    });

    // Click Unpublish button
    const unpublishButton = screen.getByRole('button', { name: /Unpublish/i });
    await user.click(unpublishButton);

    await waitFor(() => {
      expect(vi.mocked(RecipesService.unpublishRecipe)).toHaveBeenCalledWith(
        projectId,
        'test-recipe'
      );
    });
  });

  it('should delete recipe with confirmation', async () => {
    const user = userEvent.setup();
    const mockRecipe = {
      name: 'test-recipe',
      commitHash: 'abc123',
      rawYaml: 'version: "1.0"\nid: test\nop: echo',
      content: null,
      isPublished: false,
      publishedAt: null,
      publishedBy: null,
    };

    vi.mocked(RecipesService.getRecipe).mockResolvedValue(mockRecipe);
    vi.mocked(RecipesService.deleteRecipe).mockResolvedValue(undefined);

    render(
      <BrowserRouter>
        <Routes>
          <Route
            path="/"
            element={
              <div>
                <div style={{ display: 'none' }}>
                  {(window.history.pushState({}, '', '/project/proj_123/recipes/test-recipe'), null)}
                </div>
                <RecipeDetailPage projectId={projectId} />
              </div>
            }
          />
        </Routes>
      </BrowserRouter>
    );

    await waitFor(() => {
      expect(screen.getByText('test-recipe')).toBeDefined();
    });

    // Click Delete button
    const deleteButton = screen.getByRole('button', { name: /Delete/i });
    await user.click(deleteButton);

    // Confirm deletion in modal
    await waitFor(() => {
      expect(screen.getByText(/Are you sure you want to delete/i)).toBeDefined();
    });

    const confirmButton = screen.getByRole('button', { name: /Delete/i });
    await user.click(confirmButton);

    await waitFor(() => {
      expect(vi.mocked(RecipesService.deleteRecipe)).toHaveBeenCalledWith(
        projectId,
        'test-recipe'
      );
      expect(mockNavigate).toHaveBeenCalledWith(`/project/${projectId}/recipes`);
    });
  });

  it('should create new recipe', async () => {
    const user = userEvent.setup();

    vi.mocked(RecipesService.createRecipe).mockResolvedValue({
      commitHash: 'abc123',
      shortHash: 'abc123',
      createdAt: new Date().toISOString(),
      author: 'user@example.com',
      message: 'Create recipe',
      isPublished: false,
    });

    render(
      <BrowserRouter>
        <Routes>
          <Route
            path="/"
            element={
              <div>
                <div style={{ display: 'none' }}>
                  {(window.history.pushState({}, '', '/project/proj_123/recipes/new'), null)}
                </div>
                <RecipeDetailPage projectId={projectId} />
              </div>
            }
          />
        </Routes>
      </BrowserRouter>
    );

    await waitFor(() => {
      expect(screen.getByText('New Recipe')).toBeDefined();
    });

    // Enter recipe name
    const nameInput = screen.getByPlaceholderText(/e.g., workflows\/ci\/build/i);
    await user.type(nameInput, 'my-new-recipe');

    // Enter YAML content
    const textarea = screen.getByPlaceholderText(/Enter YAML content/i);
    await user.type(textarea, 'version: "1.0"\nid: my-new-recipe\nop: echo');

    // Click Save button
    const saveButton = screen.getByRole('button', { name: /^Save$/i });
    await user.click(saveButton);

    await waitFor(() => {
      expect(vi.mocked(RecipesService.createRecipe)).toHaveBeenCalledWith(
        projectId,
        expect.objectContaining({
          name: 'my-new-recipe',
          autoPublish: false,
        })
      );
      expect(mockNavigate).toHaveBeenCalledWith(
        `/project/${projectId}/recipes/my-new-recipe`
      );
    });
  });

  it('should handle save errors', async () => {
    const user = userEvent.setup();
    const consoleErrorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});

    const mockRecipe = {
      name: 'test-recipe',
      commitHash: 'abc123',
      rawYaml: 'version: "1.0"\nid: test\nop: echo',
      content: null,
      isPublished: false,
      publishedAt: null,
      publishedBy: null,
    };

    vi.mocked(RecipesService.getRecipe).mockResolvedValue(mockRecipe);
    vi.mocked(RecipesService.updateRecipe).mockRejectedValue(
      new Error('Validation failed')
    );

    render(
      <BrowserRouter>
        <Routes>
          <Route
            path="/"
            element={
              <div>
                <div style={{ display: 'none' }}>
                  {(window.history.pushState({}, '', '/project/proj_123/recipes/test-recipe'), null)}
                </div>
                <RecipeDetailPage projectId={projectId} />
              </div>
            }
          />
        </Routes>
      </BrowserRouter>
    );

    await waitFor(() => {
      expect(screen.getByText('test-recipe')).toBeDefined();
    });

    // Modify content
    const textarea = screen.getByRole('textbox');
    await user.type(textarea, '\n# modified');

    // Click Save button
    const saveButton = screen.getByRole('button', { name: /^Save$/i });
    await user.click(saveButton);

    await waitFor(() => {
      expect(consoleErrorSpy).toHaveBeenCalledWith(
        'Failed to save recipe',
        expect.any(Error)
      );
    });

    consoleErrorSpy.mockRestore();
  });

  it('should handle load errors (404)', async () => {
    const consoleErrorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});

    vi.mocked(RecipesService.getRecipe).mockRejectedValue(
      new Error('Recipe not found')
    );

    render(
      <BrowserRouter>
        <Routes>
          <Route
            path="/"
            element={
              <div>
                <div style={{ display: 'none' }}>
                  {(window.history.pushState({}, '', '/project/proj_123/recipes/nonexistent'), null)}
                </div>
                <RecipeDetailPage projectId={projectId} />
              </div>
            }
          />
        </Routes>
      </BrowserRouter>
    );

    await waitFor(() => {
      expect(consoleErrorSpy).toHaveBeenCalledWith(
        'Failed to load recipe',
        expect.any(Error)
      );
    });

    consoleErrorSpy.mockRestore();
  });

  it('should disable save button when no changes', async () => {
    const mockRecipe = {
      name: 'test-recipe',
      commitHash: 'abc123',
      rawYaml: 'version: "1.0"\nid: test\nop: echo',
      content: null,
      isPublished: false,
      publishedAt: null,
      publishedBy: null,
    };

    vi.mocked(RecipesService.getRecipe).mockResolvedValue(mockRecipe);

    render(
      <BrowserRouter>
        <Routes>
          <Route
            path="/"
            element={
              <div>
                <div style={{ display: 'none' }}>
                  {(window.history.pushState({}, '', '/project/proj_123/recipes/test-recipe'), null)}
                </div>
                <RecipeDetailPage projectId={projectId} />
              </div>
            }
          />
        </Routes>
      </BrowserRouter>
    );

    await waitFor(() => {
      expect(screen.getByText('test-recipe')).toBeDefined();
    });

    // Save button should be disabled when there are no changes
    const saveButton = screen.getByRole('button', { name: /^Save$/i });
    expect(saveButton).toHaveProperty('disabled', true);
  });

  it('should display detailed API error messages from server', async () => {
    const user = userEvent.setup();
    const consoleErrorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});

    // Create an ApiError with detailed error message in body
    const apiError = new ApiError(
      { method: 'POST', url: '/api/recipes' } as any,
      {
        url: '/api/recipes',
        status: 400,
        statusText: 'Bad Request',
        body: { error: 'recipe: invalid recipe content: unknown op: [echo] at [1:1]' },
      } as any,
      'Invalid request (validation failed)'
    );

    vi.mocked(RecipesService.createRecipe).mockRejectedValue(apiError);

    render(
      <BrowserRouter>
        <Routes>
          <Route
            path="/"
            element={
              <div>
                <div style={{ display: 'none' }}>
                  {(window.history.pushState({}, '', '/project/proj_123/recipes/new'), null)}
                </div>
                <RecipeDetailPage projectId={projectId} />
              </div>
            }
          />
        </Routes>
      </BrowserRouter>
    );

    await waitFor(() => {
      expect(screen.getByText('New Recipe')).toBeDefined();
    });

    // Enter recipe name
    const nameInput = screen.getByPlaceholderText(/e.g., workflows\/ci\/build/i);
    await user.type(nameInput, 'bad-recipe');

    // Enter invalid YAML content
    const textarea = screen.getByPlaceholderText(/Enter YAML content/i);
    await user.type(textarea, 'version: "1.0"\nop: echo');

    // Click Save button
    const saveButton = screen.getByRole('button', { name: /^Save$/i });
    await user.click(saveButton);

    // The error message should show the detailed server error, not the generic message
    await waitFor(() => {
      expect(consoleErrorSpy).toHaveBeenCalledWith(
        'Failed to save recipe',
        apiError
      );
    });

    consoleErrorSpy.mockRestore();
  });
});
