import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Button, Card, Input, Select, Space, Tag, Tree, Typography } from 'antd';
import { FileTextOutlined, FolderOutlined, PlusOutlined, SearchOutlined } from '@ant-design/icons';
import { RecipesService, type RecipeInfo } from '@colony2/openapi-client';
import type { DataNode } from 'antd/es/tree';
import { showError } from '../utils/showError';

const { Title } = Typography;

interface RecipeListPageProps {
  projectId: string;
}

export default function RecipeListPage({ projectId }: RecipeListPageProps) {
  const [recipes, setRecipes] = useState<RecipeInfo[]>([]);
  const [loading, setLoading] = useState(false);
  const [statusFilter, setStatusFilter] = useState<'all' | 'published' | 'unpublished'>('all');
  const [searchQuery, setSearchQuery] = useState('');
  const navigate = useNavigate();

  const loadRecipes = async () => {
    setLoading(true);
    try {
      const response = await RecipesService.listRecipes(projectId, statusFilter);
      setRecipes(response.recipes);
    } catch (error: any) {
      console.error('Failed to load recipes', error);
      showError(error, 'Failed to load recipes');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadRecipes();
  }, [projectId, statusFilter]);

  // Build tree structure from flat recipe list
  const buildTree = (recipeList: RecipeInfo[]): DataNode[] => {
    const filtered = recipeList.filter((recipe) =>
      recipe.name.toLowerCase().includes(searchQuery.toLowerCase())
    );

    const tree: Record<string, any> = {};

    filtered.forEach((recipe) => {
      const parts = recipe.name.split('/');
      let current = tree;

      parts.forEach((part, index) => {
        if (!current[part]) {
          current[part] = {
            children: {},
            recipe: index === parts.length - 1 ? recipe : null,
          };
        }
        current = current[part].children;
      });
    });

    const convertToDataNodes = (node: Record<string, any>, prefix: string = ''): DataNode[] => {
      return Object.entries(node).map(([name, data]) => {
        const fullPath = prefix ? `${prefix}/${name}` : name;
        const hasChildren = Object.keys(data.children).length > 0;
        const recipe = data.recipe as RecipeInfo | null;

        if (hasChildren) {
          // Folder node
          return {
            key: fullPath,
            title: name,
            icon: <FolderOutlined />,
            children: convertToDataNodes(data.children, fullPath),
            selectable: false,
          };
        } else {
          // Recipe leaf node
          return {
            key: fullPath,
            title: (
              <Space>
                <span>{name}</span>
                {recipe?.publishedCommit ? (
                  <Tag color="green" style={{ fontSize: 10 }}>
                    Published
                  </Tag>
                ) : (
                  <Tag color="orange" style={{ fontSize: 10 }}>
                    Draft
                  </Tag>
                )}
              </Space>
            ),
            icon: <FileTextOutlined />,
            isLeaf: true,
          };
        }
      });
    };

    return convertToDataNodes(tree);
  };

  const treeData = buildTree(recipes);

  const handleSelect = (selectedKeys: React.Key[]) => {
    if (selectedKeys.length > 0) {
      const recipeName = String(selectedKeys[0]);
      navigate(`/project/${projectId}/recipes/${encodeURIComponent(recipeName)}`);
    }
  };

  const handleCreate = () => {
    navigate(`/project/${projectId}/recipes/new`);
  };

  return (
    <Card
      title={<Title level={4}>Recipes</Title>}
      extra={
        <Space>
          <Input
            placeholder="Search recipes..."
            prefix={<SearchOutlined />}
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            style={{ width: 200 }}
            allowClear
          />
          <Select
            value={statusFilter}
            onChange={setStatusFilter}
            style={{ width: 120 }}
            options={[
              { value: 'all', label: 'All' },
              { value: 'published', label: 'Published' },
              { value: 'unpublished', label: 'Draft' },
            ]}
          />
          <Button type="primary" icon={<PlusOutlined />} onClick={handleCreate}>
            New Recipe
          </Button>
          <Button onClick={loadRecipes}>Refresh</Button>
        </Space>
      }
    >
      {loading ? (
        <div>Loading recipes...</div>
      ) : treeData.length > 0 ? (
        <Tree
          showIcon
          defaultExpandAll
          treeData={treeData}
          onSelect={handleSelect}
          style={{ fontSize: 14 }}
        />
      ) : (
        <div style={{ textAlign: 'center', padding: '40px 0', color: '#999' }}>
          {searchQuery ? 'No recipes match your search' : 'No recipes found. Create one to get started.'}
        </div>
      )}
    </Card>
  );
}
