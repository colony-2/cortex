import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, cleanup } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import RecipeEditor from './RecipeEditor';

describe('RecipeEditor', () => {
  const mockOnChange = vi.fn();
  const mockOnNameChange = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    cleanup();
  });

  it('should render YAML editor with initial content', () => {
    const content = 'version: "1.0"\nid: test\nop: echo';

    render(
      <RecipeEditor
        content={content}
        name="test-recipe"
        isNewRecipe={false}
        onChange={mockOnChange}
        onNameChange={mockOnNameChange}
      />
    );

    // Verify YAML content is displayed
    const textarea = screen.getByRole('textbox');
    expect(textarea).toHaveProperty('value', content);
  });

  it('should call onChange when content is modified', async () => {
    const user = userEvent.setup();
    const initialContent = 'version: "1.0"\nid: test\nop: echo';

    render(
      <RecipeEditor
        content={initialContent}
        name="test-recipe"
        isNewRecipe={false}
        onChange={mockOnChange}
        onNameChange={mockOnNameChange}
      />
    );

    const textarea = screen.getByRole('textbox');
    await user.type(textarea, '\ninputs:\n  message: "Hello"');

    // Verify onChange was called with updated content
    expect(mockOnChange).toHaveBeenCalled();
    const lastCall = mockOnChange.mock.calls[mockOnChange.mock.calls.length - 1][0];
    expect(lastCall).toContain('inputs:');
  });

  it('should show name input for new recipes', () => {
    render(
      <RecipeEditor
        content=""
        name=""
        isNewRecipe={true}
        onChange={mockOnChange}
        onNameChange={mockOnNameChange}
      />
    );

    // Verify "Creating New Recipe" alert is shown
    expect(screen.getByText('Creating New Recipe')).toBeDefined();

    // Verify recipe name input is shown
    expect(screen.getByLabelText('Recipe Name')).toBeDefined();
    expect(
      screen.getByPlaceholderText('e.g., workflows/ci/build')
    ).toBeDefined();
  });

  it('should hide name input for existing recipes', () => {
    render(
      <RecipeEditor
        content="version: '1.0'"
        name="existing-recipe"
        isNewRecipe={false}
        onChange={mockOnChange}
        onNameChange={mockOnNameChange}
      />
    );

    // Verify "Creating New Recipe" alert is NOT shown
    expect(screen.queryByText('Creating New Recipe')).toBeNull();

    // Verify recipe name input is NOT shown
    expect(screen.queryByLabelText('Recipe Name')).toBeNull();
  });

  it('should call onNameChange when recipe name is modified', async () => {
    const user = userEvent.setup();

    render(
      <RecipeEditor
        content=""
        name=""
        isNewRecipe={true}
        onChange={mockOnChange}
        onNameChange={mockOnNameChange}
      />
    );

    const nameInput = screen.getByPlaceholderText('e.g., workflows/ci/build');
    await user.type(nameInput, 'my-new-recipe');

    // Verify onNameChange was called for each character
    expect(mockOnNameChange).toHaveBeenCalled();
    const lastCall = mockOnNameChange.mock.calls[mockOnNameChange.mock.calls.length - 1][0];
    expect(lastCall).toContain('my-new-recipe');
  });

  it('should display YAML format guidance', () => {
    render(
      <RecipeEditor
        content=""
        name=""
        isNewRecipe={false}
        onChange={mockOnChange}
        onNameChange={mockOnNameChange}
      />
    );

    // Verify YAML format info alert is shown
    expect(screen.getByText('YAML Format')).toBeDefined();

    // Verify example YAML is shown
    expect(screen.getByText(/version: "1.0"/)).toBeDefined();
    expect(screen.getByText(/id: recipe-name/)).toBeDefined();
    expect(screen.getByText(/op: echo/)).toBeDefined();
  });

  it('should render with empty content', () => {
    render(
      <RecipeEditor
        content=""
        name=""
        isNewRecipe={false}
        onChange={mockOnChange}
        onNameChange={mockOnNameChange}
      />
    );

    const textarea = screen.getByRole('textbox');
    expect(textarea).toHaveProperty('value', '');
    expect(textarea).toHaveProperty('placeholder', 'Enter YAML content...');
  });

  it('should display guidance for organizing recipes with slashes', () => {
    render(
      <RecipeEditor
        content=""
        name=""
        isNewRecipe={true}
        onChange={mockOnChange}
        onNameChange={mockOnNameChange}
      />
    );

    // Verify guidance about using slashes is shown
    expect(
      screen.getByText(/Use slashes to organize recipes into folders/)
    ).toBeDefined();
    expect(
      screen.getByText(/Use slashes to organize \(e.g., 'workflows\/build' or 'ops\/deploy'\)/)
    ).toBeDefined();
  });

  it('should have monospace font for YAML editor', () => {
    render(
      <RecipeEditor
        content="version: '1.0'"
        name="test"
        isNewRecipe={false}
        onChange={mockOnChange}
        onNameChange={mockOnNameChange}
      />
    );

    const textarea = screen.getByRole('textbox');
    const style = window.getComputedStyle(textarea);

    // Note: In jsdom, computed styles may not work exactly like in a real browser
    // We're checking that the fontFamily property is set
    expect(textarea.style.fontFamily).toContain('Monaco');
  });

  it('should render with correct number of rows', () => {
    render(
      <RecipeEditor
        content="version: '1.0'"
        name="test"
        isNewRecipe={false}
        onChange={mockOnChange}
        onNameChange={mockOnNameChange}
      />
    );

    const textarea = screen.getByRole('textbox');
    expect(textarea).toHaveProperty('rows', 25);
  });

  it('should handle name input with slashes correctly', async () => {
    const user = userEvent.setup();

    render(
      <RecipeEditor
        content=""
        name=""
        isNewRecipe={true}
        onChange={mockOnChange}
        onNameChange={mockOnNameChange}
      />
    );

    const nameInput = screen.getByPlaceholderText('e.g., workflows/ci/build');
    await user.type(nameInput, 'workflows/ci/build');

    // Verify onNameChange was called with the full path including slashes
    const lastCall = mockOnNameChange.mock.calls[mockOnNameChange.mock.calls.length - 1][0];
    expect(lastCall).toBe('workflows/ci/build');
  });
});
