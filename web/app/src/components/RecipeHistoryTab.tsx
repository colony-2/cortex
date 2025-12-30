import { useEffect, useState } from 'react';
import { CheckCircleOutlined } from '@ant-design/icons';
import { Button, Input, List, Modal, Space, Tag, Typography, message } from 'antd';
import { RecipesService, type RecipeVersion, type RecipeWithContent } from '@colony2/openapi-client';
import { getErrorMessage } from '../utils/errorHandling';

const { Text, Paragraph } = Typography;
const { TextArea } = Input;

interface RecipeHistoryTabProps {
  projectId: string;
  recipeName: string;
  currentCommit: string;
}

export default function RecipeHistoryTab({
  projectId,
  recipeName,
  currentCommit,
}: RecipeHistoryTabProps) {
  const [versions, setVersions] = useState<RecipeVersion[]>([]);
  const [loading, setLoading] = useState(false);
  const [viewingContent, setViewingContent] = useState<RecipeWithContent | null>(null);
  const [loadingContent, setLoadingContent] = useState(false);

  const loadHistory = async () => {
    setLoading(true);
    try {
      const response = await RecipesService.getRecipeHistory(projectId, recipeName);
      setVersions(response.versions);
    } catch (error: any) {
      console.error('Failed to load recipe history', error);
      message.error(getErrorMessage(error, 'Failed to load recipe history'));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadHistory();
  }, [projectId, recipeName]);

  const handleViewVersion = async (commitHash: string) => {
    setLoadingContent(true);
    try {
      const recipe = await RecipesService.getRecipe(projectId, recipeName, commitHash);
      setViewingContent(recipe);
    } catch (error: any) {
      console.error('Failed to load version content', error);
      message.error(getErrorMessage(error, 'Failed to load version content'));
    } finally {
      setLoadingContent(false);
    }
  };

  const handleCloseModal = () => {
    setViewingContent(null);
  };

  const formatDate = (dateString: string) => {
    const date = new Date(dateString);
    return date.toLocaleString();
  };

  return (
    <>
      <List
        loading={loading}
        dataSource={versions}
        renderItem={(version) => {
          const isCurrent = version.commitHash === currentCommit;
          const isPublished = version.isPublished;

          return (
            <List.Item
              key={version.commitHash}
              actions={[
                <Button
                  size="small"
                  onClick={() => handleViewVersion(version.commitHash)}
                  loading={loadingContent}
                >
                  View
                </Button>,
              ]}
            >
              <List.Item.Meta
                title={
                  <Space>
                    <Text code style={{ fontSize: 12 }}>
                      {version.shortHash}
                    </Text>
                    <Text>{version.message}</Text>
                    {isCurrent && <Tag color="blue">Current</Tag>}
                    {isPublished && (
                      <Tag color="green" icon={<CheckCircleOutlined />}>
                        Published
                      </Tag>
                    )}
                  </Space>
                }
                description={
                  <Space direction="vertical" size={0}>
                    <Text type="secondary" style={{ fontSize: 12 }}>
                      {version.author} • {formatDate(version.createdAt)}
                    </Text>
                  </Space>
                }
              />
            </List.Item>
          );
        }}
      />

      <Modal
        title={
          <Space>
            <span>Version Content</span>
            {viewingContent && (
              <>
                <Text code style={{ fontSize: 12 }}>
                  {viewingContent.commitHash.substring(0, 7)}
                </Text>
                {viewingContent.isPublished && (
                  <Tag color="green" icon={<CheckCircleOutlined />}>
                    Published
                  </Tag>
                )}
              </>
            )}
          </Space>
        }
        open={!!viewingContent}
        onCancel={handleCloseModal}
        width={800}
        footer={[
          <Button key="close" onClick={handleCloseModal}>
            Close
          </Button>,
        ]}
      >
        {viewingContent && (
          <TextArea
            value={viewingContent.rawYaml}
            rows={20}
            readOnly
            style={{
              fontSize: 13,
              fontFamily: 'Monaco, Menlo, "Ubuntu Mono", "Courier New", monospace',
              lineHeight: 1.6,
            }}
          />
        )}
      </Modal>
    </>
  );
}
