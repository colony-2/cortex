# Web Recipe Navigation

**Target Cell:** `web-app` (`web/app`)
**Dependencies:**
- Completion of `01_openapi_specification__api-openapi.md` (with generated TypeScript client)
- Completion of `02_recipe_service_integration__be-api.md` (backend implementation)
**Status:** Not Started

## Overview

Add a "Recipes" navigation item to the lefthand menu and implement a recipe management interface with folder-based organization using the generated OpenAPI client. Users can browse recipes hierarchically and navigate to detailed views.

## Background

The OpenAPI specification (step 01) has been implemented and the TypeScript client has been generated. This provides type-safe API access through `RecipesService` from `@colony2/openapi-client`.

Current navigation structure:
- Tickets, Workflows, Cells — Graph, Cells — List, Settings

Recipe names support hierarchical organization (e.g., `workflows/ci/build`), which will be presented as a folder tree.

## Goals

1. Add "Recipes" navigation item to lefthand menu
2. Create recipe list page with folder tree view
3. Use generated OpenAPI client for API calls
4. Support filtering by publish status
5. Show recipe metadata (latest version, published version)
6. Navigate to recipe detail/editor page
7. Maintain UI consistency

## Implementation Details

### 1. Verify Generated Client

Ensure the OpenAPI client has been generated:

```bash
cd /src/web/openapi
npm run generate
```

This should generate:
- `/src/web/openapi/src/generated/services/RecipesService.ts`
- `/src/web/openapi/src/generated/models/RecipeInfo.ts`
- `/src/web/openapi/src/generated/models/RecipeListResponse.ts`
- etc.

### 2. Add Navigation Item

**File:** `/src/web/app/src/App.tsx`

**Location:** In the navigation menu items section (around lines 146-186)

```tsx
import { FileTextOutlined } from '@ant-design/icons';

const menuItems = [
  {
    key: `/project/${selectedProjectId}/kanban`,
    icon: <AppstoreOutlined />,
    label: 'Tickets',
    disabled: !selectedProjectId,
  },
  {
    key: `/project/${selectedProjectId}/workflows`,
    icon: <ThunderboltOutlined />,
    label: 'Workflows',
    disabled: !selectedProjectId,
  },
  {
    key: `/project/${selectedProjectId}/recipes`,
    icon: <FileTextOutlined />,
    label: 'Recipes',
    disabled: !selectedProjectId,
  },
  // ... cells and settings
];
```

### 3. Add Route

**File:** `/src/web/app/src/App.tsx`

**Location:** In the Routes section (around lines 190-221)

```tsx
<Route
  path="/project/:projectId/recipes"
  element={<RecipeListPage />}
/>
<Route
  path="/project/:projectId/recipes/:recipeName/*"
  element={<RecipeDetailPage />}
/>
```

Import components:

```tsx
import RecipeListPage from './components/RecipeListPage';
// RecipeDetailPage will be added in next spec
```

### 4. Create Recipe List Page Component

**File:** `/src/web/app/src/components/RecipeListPage.tsx` (new file)

```tsx
import React, { useState, useEffect } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  Tree,
  Button,
  Space,
  Select,
  Spin,
  Empty,
  Typography,
  Card,
  Tag,
  message,
} from 'antd';
import {
  FolderOutlined,
  FileTextOutlined,
  PlusOutlined,
  ReloadOutlined,
} from '@ant-design/icons';
import type { DataNode } from 'antd/es/tree';
import { RecipesService, type RecipeInfo } from '@colony2/openapi-client';

const { Title } = Typography;

const RecipeListPage: React.FC = () => {
  const { projectId } = useParams<{ projectId: string }>();
  const navigate = useNavigate();

  const [loading, setLoading] = useState(true);
  const [recipes, setRecipes] = useState<RecipeInfo[]>([]);
  const [statusFilter, setStatusFilter] = useState<'all' | 'published' | 'unpublished'>('all');
  const [treeData, setTreeData] = useState<DataNode[]>([]);

  // Fetch recipes using generated client
  const fetchRecipes = async () => {
    if (!projectId) return;

    setLoading(true);
    try {
      const response = await RecipesService.listRecipes(projectId, statusFilter);
      setRecipes(response.recipes);
    } catch (error: any) {
      console.error('Failed to fetch recipes:', error);
      message.error('Failed to load recipes');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchRecipes();
  }, [projectId, statusFilter]);

  // Build tree structure from flat recipe list
  useEffect(() => {
    const tree = buildRecipeTree(recipes);
    setTreeData(tree);
  }, [recipes]);

  const buildRecipeTree = (recipeList: RecipeInfo[]): DataNode[] => {
    const root: Record<string, any> = {};

    recipeList.forEach(recipe => {
      const parts = recipe.name.split('/');
      let current = root;

      // Build folder structure
      for (let i = 0; i < parts.length - 1; i++) {
        const part = parts[i];
        if (!current[part]) {
          current[part] = { _folders: {}, _recipes: [] };
        }
        current = current[part]._folders;
      }

      // Add recipe to final folder
      const fileName = parts[parts.length - 1];
      if (!current[fileName]) {
        current[fileName] = { _folders: {}, _recipes: [] };
      }
      current[fileName]._recipes.push(recipe);
    });

    // Convert to Ant Design Tree format
    const convertToTreeData = (node: any, path: string = ''): DataNode[] => {
      const result: DataNode[] = [];

      Object.keys(node).forEach(key => {
        if (key === '_folders' || key === '_recipes') return;

        const currentPath = path ? `${path}/${key}` : key;
        const hasRecipes = node[key]._recipes.length > 0;
        const hasFolders = Object.keys(node[key]._folders).length > 0;

        if (hasRecipes && !hasFolders) {
          // Leaf node (recipe file)
          const recipe = node[key]._recipes[0];
          result.push({
            key: currentPath,
            title: renderRecipeTitle(key, recipe),
            icon: <FileTextOutlined />,
            isLeaf: true,
          });
        } else {
          // Folder node
          result.push({
            key: currentPath,
            title: key,
            icon: <FolderOutlined />,
            children: convertToTreeData(node[key]._folders, currentPath),
          });
        }
      });

      return result.sort((a, b) => {
        // Folders first, then files
        if (a.isLeaf && !b.isLeaf) return 1;
        if (!a.isLeaf && b.isLeaf) return -1;
        return String(a.title).localeCompare(String(b.title));
      });
    };

    return convertToTreeData(root);
  };

  const renderRecipeTitle = (name: string, recipe: RecipeInfo) => {
    const isPublished = !!recipe.publishedCommit;
    return (
      <Space>
        <span>{name}</span>
        {isPublished ? (
          <Tag color="green" style={{ fontSize: 11 }}>Published</Tag>
        ) : (
          <Tag color="orange" style={{ fontSize: 11 }}>Draft</Tag>
        )}
        <span style={{ fontSize: 11, color: '#999' }}>
          {recipe.latestCommit?.substring(0, 7)}
        </span>
      </Space>
    );
  };

  const handleSelect = (selectedKeys: React.Key[]) => {
    if (selectedKeys.length > 0) {
      const recipeName = selectedKeys[0] as string;
      // URL encode the recipe name for path parameter
      const encodedName = encodeURIComponent(recipeName);
      navigate(`/project/${projectId}/recipes/${encodedName}`);
    }
  };

  const handleCreateRecipe = () => {
    navigate(`/project/${projectId}/recipes/new`);
  };

  return (
    <div style={{ padding: 24 }}>
      <Card>
        <Space direction="vertical" size="large" style={{ width: '100%' }}>
          {/* Header */}
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <Title level={3} style={{ margin: 0 }}>Recipes</Title>
            <Space>
              <Select
                value={statusFilter}
                onChange={setStatusFilter}
                style={{ width: 150 }}
                options={[
                  { label: 'All Recipes', value: 'all' },
                  { label: 'Published', value: 'published' },
                  { label: 'Drafts', value: 'unpublished' },
                ]}
              />
              <Button
                icon={<ReloadOutlined />}
                onClick={fetchRecipes}
              >
                Refresh
              </Button>
              <Button
                type="primary"
                icon={<PlusOutlined />}
                onClick={handleCreateRecipe}
              >
                New Recipe
              </Button>
            </Space>
          </div>

          {/* Tree View */}
          {loading ? (
            <div style={{ textAlign: 'center', padding: 48 }}>
              <Spin size="large" />
            </div>
          ) : treeData.length > 0 ? (
            <Tree
              showIcon
              showLine={{ showLeafIcon: false }}
              defaultExpandAll
              treeData={treeData}
              onSelect={handleSelect}
              style={{
                background: '#fafafa',
                padding: 16,
                borderRadius: 4,
                minHeight: 400
              }}
            />
          ) : (
            <Empty
              description={
                statusFilter === 'published'
                  ? 'No published recipes'
                  : 'No recipes found'
              }
              style={{ padding: 48 }}
            >
              <Button type="primary" icon={<PlusOutlined />} onClick={handleCreateRecipe}>
                Create First Recipe
              </Button>
            </Empty>
          )}
        </Space>
      </Card>
    </div>
  );
};

export default RecipeListPage;
```

**Key Features:**
- Uses generated `RecipesService.listRecipes()` for type-safe API calls
- `RecipeInfo` type from generated client ensures type safety
- Folder tree visualization of hierarchical names
- Published/Draft status badges
- Short commit hash display
- Filter by status
- URL encodes recipe names for navigation

### 5. Update Package Dependencies

**File:** `/src/web/app/package.json`

Ensure dependencies are current:

```json
{
  "dependencies": {
    "@colony2/openapi-client": "workspace:*",
    "antd": "^5.x.x",
    "react": "^18.x.x",
    "react-router-dom": "^6.x.x",
    "@ant-design/icons": "^5.x.x"
  }
}
```

Install if needed:

```bash
cd /src/web/app
npm install
```

## Generated Client Usage Examples

The generated client provides type-safe methods:

```typescript
import { RecipesService, type RecipeInfo, type RecipeListResponse } from '@colony2/openapi-client';

// List recipes
const response: RecipeListResponse = await RecipesService.listRecipes(
  'proj_123',
  'published'
);

// Access with type safety
response.recipes.forEach((recipe: RecipeInfo) => {
  console.log(recipe.name); // TypeScript knows this exists
  console.log(recipe.publishedCommit); // string | null
});

// Get recipe
const recipe = await RecipesService.getRecipe(
  'proj_123',
  'workflows/ci/build',
  'v1.0.0' // optional ref
);

// Create recipe
const version = await RecipesService.createRecipe('proj_123', {
  name: 'workflows/test',
  content: 'version: "1.0"...',
  autoPublish: true
});
```

## Testing Strategy

### Unit Tests

**File:** `/src/web/app/src/components/RecipeListPage.test.tsx`

Test cases:
- Recipe tree builds correctly from flat list
- Folders sorted before files
- Published/draft tags display correctly
- Status filter calls API with correct parameter
- Navigation triggers with URL-encoded recipe name
- Create button navigates to new recipe page
- Empty state displays when no recipes

### Manual Testing

1. **Navigation:**
   - Click "Recipes" in lefthand menu
   - Verify page loads

2. **Recipe Display:**
   - Verify recipes display in tree structure
   - Verify folders can expand/collapse
   - Verify published badges appear correctly

3. **Filtering:**
   - Select "Published" filter
   - Verify only published recipes show
   - Select "Drafts" filter
   - Verify only unpublished recipes show

4. **Actions:**
   - Click recipe
   - Verify navigation to detail page (URL includes encoded name)
   - Click "New Recipe"
   - Verify navigation to create page
   - Click "Refresh"
   - Verify data reloads

5. **Edge Cases:**
   - No recipes: verify empty state
   - Deep nesting: verify tree handles it
   - Special characters in names: verify display/navigation

## Type Safety Benefits

Using the generated OpenAPI client provides:

1. **Compile-time checks:** TypeScript catches API contract violations
2. **Autocomplete:** IDE suggests available fields
3. **Refactoring safety:** Renaming fields updates all usages
4. **Documentation:** JSDoc comments from OpenAPI spec
5. **Error handling:** Typed error responses

## Success Criteria

- [ ] "Recipes" navigation item added
- [ ] Route configured for recipe list
- [ ] Recipe list page component created
- [ ] Uses generated `RecipesService` for API calls
- [ ] Uses generated types (`RecipeInfo`, etc.)
- [ ] Folder tree displays recipes hierarchically
- [ ] Status filter works
- [ ] Published/draft badges display
- [ ] Navigation to detail works with URL encoding
- [ ] "New Recipe" button works
- [ ] Refresh button works
- [ ] Empty state displays correctly
- [ ] Loading state shows during fetch
- [ ] Error handling with user-friendly messages
- [ ] UI matches existing page styles
- [ ] Unit tests pass
- [ ] Manual testing complete

## Next Steps

After completing this implementation:
1. Proceed to `04_web_recipe_editing__web-app.md` - Implement recipe editor with version management
