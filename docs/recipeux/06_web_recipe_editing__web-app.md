# Web Recipe Editing

**Target Cell:** `web-app` (`web/app`)
**Dependencies:**
- Completion of `01_openapi_specification__api-openapi.md` (with generated TypeScript client)
- Completion of `02_recipe_service_integration__be-api.md` (backend implementation)
- Completion of `03_web_recipe_navigation__web-app.md` (navigation UI)
**Status:** Not Started

## Overview

Implement a comprehensive recipe editing experience using the generated OpenAPI client. Users can create, edit, publish, and manage recipe versions with YAML editing, validation, and version history.

## Goals

1. Create recipe detail page with editor and metadata
2. Support YAML editing with syntax highlighting
3. Implement create/update/delete operations using generated client
4. Add publish/unpublish controls
5. Show version history with view/restore capabilities
6. Real-time validation feedback
7. Type-safe API integration

## Implementation Details

### 1. Install Additional Dependencies

**File:** `/src/web/app/package.json`

```json
{
  "dependencies": {
    "@monaco-editor/react": "^4.6.0",
    "js-yaml": "^4.1.0"
  },
  "devDependencies": {
    "@types/js-yaml": "^4.0.9"
  }
}
```

```bash
cd /src/web/app
npm install @monaco-editor/react js-yaml
npm install --save-dev @types/js-yaml
```

### 2. Create Recipe Detail Page Component

**File:** `/src/web/app/src/components/RecipeDetailPage.tsx` (new file)

```tsx
import React, { useState, useEffect } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  Card,
  Button,
  Space,
  Tabs,
  Typography,
  Tag,
  Modal,
  message,
  Spin,
  Alert,
  Tooltip,
  Input,
} from 'antd';
import {
  SaveOutlined,
  DeleteOutlined,
  RocketOutlined,
  StopOutlined,
  HistoryOutlined,
  ArrowLeftOutlined,
  ReloadOutlined,
} from '@ant-design/icons';
import {
  RecipesService,
  type RecipeWithContent,
  type RecipeVersion,
} from '@colony2/openapi-client';
import RecipeEditor from './RecipeEditor';
import RecipeHistoryTab from './RecipeHistoryTab';

const { Title, Text } = Typography;
const { TabPane } = Tabs;

const RecipeDetailPage: React.FC = () => {
  const { projectId, recipeName } = useParams<{
    projectId: string;
    recipeName: string;
  }>();
  const navigate = useNavigate();

  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [recipe, setRecipe] = useState<RecipeWithContent | null>(null);
  const [content, setContent] = useState<string>('');
  const [hasChanges, setHasChanges] = useState(false);
  const [validationError, setValidationError] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState('editor');

  const isNewRecipe = recipeName === 'new';
  const [newRecipeName, setNewRecipeName] = useState('');

  // Decode recipe name from URL
  const decodedRecipeName = recipeName ? decodeURIComponent(recipeName) : '';

  // Fetch recipe using generated client
  const fetchRecipe = async () => {
    if (!projectId || isNewRecipe) {
      setLoading(false);
      setContent(getDefaultRecipeTemplate());
      return;
    }

    setLoading(true);
    try {
      const response = await RecipesService.getRecipe(projectId, decodedRecipeName);
      setRecipe(response);
      setContent(response.rawYaml);
      setHasChanges(false);
    } catch (error: any) {
      console.error('Failed to fetch recipe:', error);
      message.error('Failed to load recipe');
      // Navigate back to list on 404
      if (error.status === 404) {
        navigate(`/project/${projectId}/recipes`);
      }
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchRecipe();
  }, [projectId, recipeName]);

  const getDefaultRecipeTemplate = () => {
    return `version: "1.0"
id: ""
desc: "New recipe"
op: echo
inputs:
  message: "Hello from recipe"
`;
  };

  const handleContentChange = (newContent: string) => {
    setContent(newContent);
    setHasChanges(true);

    // Simple validation
    try {
      const yaml = require('js-yaml');
      const parsed = yaml.load(newContent);

      if (!parsed.version || !parsed.id || !parsed.op) {
        setValidationError('Recipe must have version, id, and op fields');
      } else {
        setValidationError(null);
      }
    } catch (e: any) {
      setValidationError(`YAML syntax error: ${e.message}`);
    }
  };

  const handleSave = async (autoPublish: boolean = false) => {
    if (!projectId) return;

    if (validationError) {
      message.error('Please fix validation errors before saving');
      return;
    }

    setSaving(true);
    try {
      if (isNewRecipe) {
        // Create new recipe
        if (!newRecipeName) {
          message.error('Please enter a recipe name');
          setSaving(false);
          return;
        }

        const version: RecipeVersion = await RecipesService.createRecipe(projectId, {
          name: newRecipeName,
          content: content,
          description: '',
          autoPublish: autoPublish,
        });

        message.success(`Recipe created${autoPublish ? ' and published' : ''}`);
        navigate(`/project/${projectId}/recipes/${encodeURIComponent(newRecipeName)}`);
      } else {
        // Update existing recipe
        const version: RecipeVersion = await RecipesService.updateRecipe(
          projectId,
          decodedRecipeName,
          {
            content: content,
            message: 'Update recipe',
            autoPublish: autoPublish,
            expectedCommit: recipe?.commitHash,
          }
        );

        message.success(`Recipe updated${autoPublish ? ' and published' : ''}`);
        await fetchRecipe();
      }

      setHasChanges(false);
    } catch (error: any) {
      console.error('Failed to save recipe:', error);
      if (error.status === 409) {
        message.error('Version conflict - recipe was modified by someone else');
      } else if (error.status === 400) {
        message.error('Invalid recipe content');
      } else {
        message.error('Failed to save recipe');
      }
    } finally {
      setSaving(false);
    }
  };

  const handlePublish = async () => {
    if (!projectId || !decodedRecipeName) return;

    if (hasChanges) {
      message.warning('Please save changes before publishing');
      return;
    }

    Modal.confirm({
      title: 'Publish Recipe',
      content: 'Publishing will make this version available to workflows. Continue?',
      okText: 'Publish',
      okType: 'primary',
      onOk: async () => {
        try {
          await RecipesService.publishRecipe(projectId, decodedRecipeName, {
            commitHash: recipe!.commitHash,
          });
          message.success('Recipe published');
          await fetchRecipe();
        } catch (error) {
          console.error('Failed to publish recipe:', error);
          message.error('Failed to publish recipe');
        }
      },
    });
  };

  const handleUnpublish = async () => {
    if (!projectId || !decodedRecipeName) return;

    Modal.confirm({
      title: 'Unpublish Recipe',
      content: 'Unpublishing will prevent workflows from using this recipe. Continue?',
      okText: 'Unpublish',
      okType: 'danger',
      onOk: async () => {
        try {
          await RecipesService.unpublishRecipe(projectId, decodedRecipeName);
          message.success('Recipe unpublished');
          await fetchRecipe();
        } catch (error) {
          console.error('Failed to unpublish recipe:', error);
          message.error('Failed to unpublish recipe');
        }
      },
    });
  };

  const handleDelete = () => {
    if (!projectId || !decodedRecipeName) return;

    Modal.confirm({
      title: 'Delete Recipe',
      content: 'This will permanently delete the recipe and all its history. This cannot be undone. Continue?',
      okText: 'Delete',
      okType: 'danger',
      onOk: async () => {
        try {
          await RecipesService.deleteRecipe(projectId, decodedRecipeName);
          message.success('Recipe deleted');
          navigate(`/project/${projectId}/recipes`);
        } catch (error) {
          console.error('Failed to delete recipe:', error);
          message.error('Failed to delete recipe');
        }
      },
    });
  };

  const handleBack = () => {
    if (hasChanges) {
      Modal.confirm({
        title: 'Unsaved Changes',
        content: 'You have unsaved changes. Are you sure you want to leave?',
        okText: 'Leave',
        cancelText: 'Stay',
        onOk: () => navigate(`/project/${projectId}/recipes`),
      });
    } else {
      navigate(`/project/${projectId}/recipes`);
    }
  };

  const handleViewVersion = async (commitHash: string) => {
    if (!projectId || !decodedRecipeName) return;

    try {
      const response = await RecipesService.getRecipe(
        projectId,
        decodedRecipeName,
        commitHash
      );

      Modal.info({
        title: `Version ${commitHash.substring(0, 7)}`,
        content: (
          <pre style={{
            background: '#f5f5f5',
            padding: 12,
            borderRadius: 4,
            maxHeight: 400,
            overflow: 'auto',
            fontSize: 12,
          }}>
            {response.rawYaml}
          </pre>
        ),
        width: 800,
        okText: 'Close',
      });
    } catch (error) {
      message.error('Failed to load version');
    }
  };

  const handleRestoreVersion = async (commitHash: string) => {
    if (!projectId || !decodedRecipeName) return;

    Modal.confirm({
      title: 'Restore Version',
      content: `Restore to version ${commitHash.substring(0, 7)}? This will create a new version with the old content.`,
      okText: 'Restore',
      onOk: async () => {
        try {
          const response = await RecipesService.getRecipe(
            projectId,
            decodedRecipeName,
            commitHash
          );

          setContent(response.rawYaml);
          setHasChanges(true);
          setActiveTab('editor');
          message.success('Content restored. Click Save to create new version.');
        } catch (error) {
          message.error('Failed to restore version');
        }
      },
    });
  };

  if (loading) {
    return (
      <div style={{ padding: 24, textAlign: 'center' }}>
        <Spin size="large" />
      </div>
    );
  }

  const isPublished = recipe?.isPublished;

  return (
    <div style={{ padding: 24 }}>
      <Card>
        <Space direction="vertical" size="large" style={{ width: '100%' }}>
          {/* Header */}
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
            <Space direction="vertical" size="small">
              <Space>
                <Button icon={<ArrowLeftOutlined />} onClick={handleBack}>
                  Back
                </Button>
                <Title level={3} style={{ margin: 0 }}>
                  {isNewRecipe ? 'New Recipe' : decodedRecipeName}
                </Title>
                {!isNewRecipe && (
                  <>
                    {isPublished ? (
                      <Tag color="green">Published</Tag>
                    ) : (
                      <Tag color="orange">Draft</Tag>
                    )}
                    {hasChanges && <Tag color="blue">Unsaved Changes</Tag>}
                  </>
                )}
              </Space>

              {!isNewRecipe && recipe && (
                <Text type="secondary" style={{ fontSize: 13 }}>
                  Latest: {recipe.commitHash?.substring(0, 7)}
                  {recipe.publishedAt && ` • Published: ${new Date(recipe.publishedAt).toLocaleDateString()}`}
                </Text>
              )}
            </Space>

            <Space>
              {!isNewRecipe && (
                <>
                  <Tooltip title="Refresh">
                    <Button icon={<ReloadOutlined />} onClick={fetchRecipe} />
                  </Tooltip>

                  {isPublished ? (
                    <Button icon={<StopOutlined />} onClick={handleUnpublish}>
                      Unpublish
                    </Button>
                  ) : (
                    <Button
                      type="primary"
                      icon={<RocketOutlined />}
                      onClick={handlePublish}
                      disabled={hasChanges}
                    >
                      Publish
                    </Button>
                  )}

                  <Button danger icon={<DeleteOutlined />} onClick={handleDelete}>
                    Delete
                  </Button>
                </>
              )}

              <Button
                type="primary"
                icon={<SaveOutlined />}
                onClick={() => handleSave(false)}
                disabled={!hasChanges || !!validationError}
                loading={saving}
              >
                Save
              </Button>

              {!isNewRecipe && (
                <Button
                  type="primary"
                  icon={<RocketOutlined />}
                  onClick={() => handleSave(true)}
                  disabled={!hasChanges || !!validationError}
                  loading={saving}
                >
                  Save & Publish
                </Button>
              )}
            </Space>
          </div>

          {/* Validation Error */}
          {validationError && (
            <Alert
              message="Validation Error"
              description={validationError}
              type="error"
              showIcon
              closable
              onClose={() => setValidationError(null)}
            />
          )}

          {/* Tabs */}
          <Tabs activeKey={activeTab} onChange={setActiveTab}>
            <TabPane tab="Editor" key="editor">
              {isNewRecipe && (
                <div style={{ marginBottom: 16 }}>
                  <Text strong>Recipe Name:</Text>
                  <br />
                  <Input
                    value={newRecipeName}
                    onChange={(e) => setNewRecipeName(e.target.value)}
                    placeholder="e.g., workflows/ci/build"
                    style={{ maxWidth: 400, marginTop: 8 }}
                  />
                  <br />
                  <Text type="secondary" style={{ fontSize: 12 }}>
                    Use slashes for organization (e.g., workflows/ci/build)
                  </Text>
                </div>
              )}

              <RecipeEditor value={content} onChange={handleContentChange} />
            </TabPane>

            {!isNewRecipe && (
              <TabPane
                tab={<span><HistoryOutlined /> Version History</span>}
                key="history"
              >
                <RecipeHistoryTab
                  projectId={projectId!}
                  recipeName={decodedRecipeName}
                  onViewVersion={handleViewVersion}
                  onRestoreVersion={handleRestoreVersion}
                />
              </TabPane>
            )}
          </Tabs>
        </Space>
      </Card>
    </div>
  );
};

export default RecipeDetailPage;
```

### 3. Create Recipe Editor Component

**File:** `/src/web/app/src/components/RecipeEditor.tsx` (new file)

```tsx
import React from 'react';
import Editor from '@monaco-editor/react';

interface RecipeEditorProps {
  value: string;
  onChange: (value: string) => void;
}

const RecipeEditor: React.FC<RecipeEditorProps> = ({ value, onChange }) => {
  const handleEditorChange = (newValue: string | undefined) => {
    onChange(newValue || '');
  };

  return (
    <div style={{
      border: '1px solid #d9d9d9',
      borderRadius: 4,
      overflow: 'hidden'
    }}>
      <Editor
        height="600px"
        defaultLanguage="yaml"
        value={value}
        onChange={handleEditorChange}
        theme="vs-light"
        options={{
          minimap: { enabled: false },
          fontSize: 14,
          lineNumbers: 'on',
          scrollBeyondLastLine: false,
          automaticLayout: true,
          tabSize: 2,
          wordWrap: 'on',
        }}
      />
    </div>
  );
};

export default RecipeEditor;
```

### 4. Create Recipe History Tab Component

**File:** `/src/web/app/src/components/RecipeHistoryTab.tsx` (new file)

```tsx
import React, { useState, useEffect } from 'react';
import { Table, Button, Space, Tag, Spin, Typography, message } from 'antd';
import { EyeOutlined, RollbackOutlined } from '@ant-design/icons';
import { RecipesService, type RecipeVersion } from '@colony2/openapi-client';
import type { ColumnsType } from 'antd/es/table';

const { Text } = Typography;

interface RecipeHistoryTabProps {
  projectId: string;
  recipeName: string;
  onViewVersion: (commitHash: string) => void;
  onRestoreVersion: (commitHash: string) => void;
}

const RecipeHistoryTab: React.FC<RecipeHistoryTabProps> = ({
  projectId,
  recipeName,
  onViewVersion,
  onRestoreVersion,
}) => {
  const [loading, setLoading] = useState(true);
  const [versions, setVersions] = useState<RecipeVersion[]>([]);

  useEffect(() => {
    fetchHistory();
  }, [projectId, recipeName]);

  const fetchHistory = async () => {
    setLoading(true);
    try {
      const response = await RecipesService.getRecipeHistory(projectId, recipeName);
      setVersions(response.versions);
    } catch (error) {
      console.error('Failed to fetch history:', error);
      message.error('Failed to load history');
    } finally {
      setLoading(false);
    }
  };

  const columns: ColumnsType<RecipeVersion> = [
    {
      title: 'Commit',
      dataIndex: 'shortHash',
      key: 'shortHash',
      width: 100,
      render: (shortHash: string) => <Text code>{shortHash}</Text>,
    },
    {
      title: 'Message',
      dataIndex: 'message',
      key: 'message',
      ellipsis: true,
    },
    {
      title: 'Author',
      dataIndex: 'author',
      key: 'author',
      width: 120,
    },
    {
      title: 'Date',
      dataIndex: 'createdAt',
      key: 'createdAt',
      width: 180,
      render: (date: string) => new Date(date).toLocaleString(),
    },
    {
      title: 'Status',
      dataIndex: 'isPublished',
      key: 'isPublished',
      width: 100,
      render: (isPublished: boolean) =>
        isPublished ? (
          <Tag color="green">Published</Tag>
        ) : (
          <Tag color="default">Draft</Tag>
        ),
    },
    {
      title: 'Actions',
      key: 'actions',
      width: 180,
      render: (_: any, record: RecipeVersion) => (
        <Space>
          <Button
            size="small"
            icon={<EyeOutlined />}
            onClick={() => onViewVersion(record.commitHash)}
          >
            View
          </Button>
          <Button
            size="small"
            icon={<RollbackOutlined />}
            onClick={() => onRestoreVersion(record.commitHash)}
          >
            Restore
          </Button>
        </Space>
      ),
    },
  ];

  if (loading) {
    return (
      <div style={{ textAlign: 'center', padding: 48 }}>
        <Spin size="large" />
      </div>
    );
  }

  return (
    <Table
      columns={columns}
      dataSource={versions}
      rowKey="commitHash"
      pagination={{ pageSize: 10 }}
    />
  );
};

export default RecipeHistoryTab;
```

### 5. Update App.tsx

**File:** `/src/web/app/src/App.tsx`

Add import:

```tsx
import RecipeDetailPage from './components/RecipeDetailPage';
```

## Type Safety Examples

The generated client provides full type safety:

```typescript
// Create recipe - TypeScript enforces required fields
const version: RecipeVersion = await RecipesService.createRecipe(projectId, {
  name: 'test', // required
  content: 'yaml', // required
  description: 'desc', // optional
  autoPublish: true, // optional with default
});

// TypeScript knows the response type
console.log(version.commitHash); // string
console.log(version.isPublished); // boolean

// Get recipe with full type info
const recipe: RecipeWithContent = await RecipesService.getRecipe(
  projectId,
  recipeName,
  'v1.0.0' // optional ref parameter
);

// TypeScript knows all fields
console.log(recipe.name); // string
console.log(recipe.publishedAt); // string | null
console.log(recipe.content); // object
console.log(recipe.rawYaml); // string
```

## Testing Strategy

### Unit Tests

**File:** `/src/web/app/src/components/RecipeDetailPage.test.tsx`

Test cases:
- Recipe loads and displays
- Editor changes trigger unsaved state
- Save creates/updates recipe using generated client
- Publish/unpublish calls correct API methods
- Delete removes recipe
- Validation errors prevent save
- History tab loads versions
- View/restore version work

### Integration Tests

Manual testing checklist:
1. **Create Flow:** New recipe → Enter name → Edit → Save → Verify in list
2. **Edit Flow:** Open recipe → Modify → Save → Verify new version in history
3. **Publish Flow:** Create → Save → Publish → Verify Published badge
4. **History Flow:** View version → Restore → Save → Verify new version created
5. **Delete Flow:** Delete → Confirm → Verify removed from list
6. **Validation:** Remove required field → Verify error → Fix → Save enabled
7. **URL Encoding:** Recipe with slashes → Navigate → Verify correct recipe loads

## Success Criteria

- [ ] Recipe detail page component created
- [ ] Recipe editor component with Monaco
- [ ] Recipe history tab component
- [ ] All components use generated OpenAPI client
- [ ] Type safety enforced (TypeScript checks pass)
- [ ] Create recipe flow works
- [ ] Update recipe flow works
- [ ] Delete recipe flow works
- [ ] Publish/unpublish controls work
- [ ] Version history displays correctly
- [ ] View version modal works
- [ ] Restore version works
- [ ] YAML validation works
- [ ] Unsaved changes warning works
- [ ] URL encoding/decoding works for recipe names with slashes
- [ ] Error handling with proper messages
- [ ] UI matches existing page styles
- [ ] Dependencies installed
- [ ] Unit tests pass
- [ ] Manual testing checklist complete

## Next Steps

This completes the Recipe UX implementation. The system now provides:
- Git-backed recipe version control
- Full CRUD operations via type-safe API
- Publish/unpublish lifecycle management
- Web UI with folder organization
- YAML editor with validation
- Version history and restoration

Potential enhancements:
- Recipe templates library
- Advanced YAML schema validation
- Diff view between versions
- Recipe import/export
- Recipe cloning
- Batch publish operations
- Recipe usage tracking (which workflows use which recipes)
