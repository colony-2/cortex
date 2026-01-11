import { useEffect, useState } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { ArrowLeftOutlined, CheckOutlined, CloseOutlined, DeleteOutlined, SaveOutlined } from '@ant-design/icons';
import { Button, Card, Modal, Space, Tabs, Tag, Typography, message } from 'antd';
import { RecipesService, type RecipeWithContent } from '@colony2/openapi-client';
import RecipeEditor from './RecipeEditor';
import RecipeHistoryTab from './RecipeHistoryTab';
import { getErrorMessage } from '../utils/errorHandling';

const { Title } = Typography;

interface RecipeDetailPageProps {
  projectId: string;
}

export default function RecipeDetailPage({ projectId }: RecipeDetailPageProps) {
  const location = useLocation();
  const navigate = useNavigate();

  // Extract recipe name from URL path (everything after /recipes/)
  const recipeName = location.pathname.split('/recipes/')[1] || '';
  const isNewRecipe = recipeName === 'new';

  const [recipe, setRecipe] = useState<RecipeWithContent | null>(null);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [content, setContent] = useState('');
  const [name, setName] = useState('');
  const [hasChanges, setHasChanges] = useState(false);
  const [activeTab, setActiveTab] = useState('editor');

  const loadRecipe = async () => {
    if (isNewRecipe) {
      setContent('version: "1.0"\nid: \nop: echo\ninputs:\n  message: "Hello World"');
      setName('');
      return;
    }

    setLoading(true);
    try {
      const decodedName = decodeURIComponent(recipeName);
      const data = await RecipesService.getRecipe(projectId, decodedName);
      setRecipe(data);
      setContent(data.rawYaml);
      setName(decodedName);
      setHasChanges(false);
    } catch (error: any) {
      console.error('Failed to load recipe', error);
      message.error(getErrorMessage(error, 'Failed to load recipe'));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadRecipe();
  }, [projectId, recipeName]);

  const handleSave = async (publish = false) => {
    setSaving(true);
    try {
      if (isNewRecipe) {
        // Create new recipe
        if (!name || name.trim() === '') {
          message.error('Please enter a recipe name');
          setSaving(false);
          return;
        }
        await RecipesService.createRecipe(projectId, {
          name: name.trim(),
          content,
          autoPublish: publish,
        });
        message.success(publish ? 'Recipe created and published' : 'Recipe created');
        navigate(`/project/${projectId}/recipes/${encodeURIComponent(name.trim())}`);
      } else {
        // Update existing recipe
        await RecipesService.updateRecipe(projectId, recipeName, {
          content,
          message: 'Update recipe',
          autoPublish: publish,
        });
        message.success(publish ? 'Recipe saved and published' : 'Recipe saved');
        setHasChanges(false);
        await loadRecipe();
      }
    } catch (error: any) {
      console.error('Failed to save recipe', error);
      message.error(getErrorMessage(error, 'Failed to save recipe'));
    } finally {
      setSaving(false);
    }
  };

  const handlePublish = async () => {
    if (!recipe) return;

    setSaving(true);
    try {
      await RecipesService.publishRecipe(projectId, recipeName, {});
      message.success('Recipe published');
      await loadRecipe();
    } catch (error: any) {
      console.error('Failed to publish recipe', error);
      message.error(getErrorMessage(error, 'Failed to publish recipe'));
    } finally {
      setSaving(false);
    }
  };

  const handleUnpublish = async () => {
    if (!recipe) return;

    setSaving(true);
    try {
      await RecipesService.unpublishRecipe(projectId, recipeName);
      message.success('Recipe unpublished');
      await loadRecipe();
    } catch (error: any) {
      console.error('Failed to unpublish recipe', error);
      message.error(getErrorMessage(error, 'Failed to unpublish recipe'));
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = () => {
    Modal.confirm({
      title: 'Delete Recipe',
      content: `Are you sure you want to delete "${name}"? This action cannot be undone.`,
      okText: 'Delete',
      okType: 'danger',
      onOk: async () => {
        try {
          await RecipesService.deleteRecipe(projectId, recipeName);
          message.success('Recipe deleted');
          navigate(`/project/${projectId}/recipes`);
        } catch (error: any) {
          console.error('Failed to delete recipe', error);
          message.error(getErrorMessage(error, 'Failed to delete recipe'));
        }
      },
    });
  };

  const handleContentChange = (newContent: string) => {
    setContent(newContent);
    setHasChanges(true);
  };

  const handleNameChange = (newName: string) => {
    setName(newName);
    setHasChanges(true);
  };

  const title = isNewRecipe ? 'New Recipe' : name;
  const isPublished = recipe?.isPublished || false;

  return (
    <Card
      title={
        <Space>
          <Button
            icon={<ArrowLeftOutlined />}
            onClick={() => navigate(`/project/${projectId}/recipes`)}
          />
          <Title level={4} style={{ margin: 0 }}>
            {title}
          </Title>
          {!isNewRecipe && (
            isPublished ? (
              <Tag color="green">Published</Tag>
            ) : (
              <Tag color="orange">Draft</Tag>
            )
          )}
        </Space>
      }
      extra={
        <Space>
          {!isNewRecipe && !isPublished && (
            <Button
              type="primary"
              icon={<CheckOutlined />}
              onClick={handlePublish}
              loading={saving}
            >
              Publish
            </Button>
          )}
          {!isNewRecipe && isPublished && (
            <Button
              icon={<CloseOutlined />}
              onClick={handleUnpublish}
              loading={saving}
            >
              Unpublish
            </Button>
          )}
          <Button
            type={hasChanges ? 'primary' : 'default'}
            icon={<SaveOutlined />}
            onClick={() => handleSave(false)}
            loading={saving}
            disabled={!hasChanges}
          >
            Save
          </Button>
          {hasChanges && (
            <Button
              type="primary"
              onClick={() => handleSave(true)}
              loading={saving}
            >
              Save & Publish
            </Button>
          )}
          {!isNewRecipe && (
            <Button
              danger
              icon={<DeleteOutlined />}
              onClick={handleDelete}
            >
              Delete
            </Button>
          )}
        </Space>
      }
    >
      {loading ? (
        <div>Loading recipe...</div>
      ) : (
        <Tabs
          activeKey={activeTab}
          onChange={setActiveTab}
          items={[
            {
              key: 'editor',
              label: 'Editor',
              children: (
                <RecipeEditor
                  content={content}
                  name={name}
                  isNewRecipe={isNewRecipe}
                  onChange={handleContentChange}
                  onNameChange={handleNameChange}
                />
              ),
            },
            ...(!isNewRecipe ? [{
              key: 'history',
              label: 'Version History',
              children: (
                <RecipeHistoryTab
                  projectId={projectId}
                  recipeName={recipeName}
                  currentCommit={recipe?.commitHash || ''}
                />
              ),
            }] : []),
          ]}
        />
      )}
    </Card>
  );
}
