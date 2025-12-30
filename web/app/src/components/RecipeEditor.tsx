import { Alert, Form, Input, Space } from 'antd';
import { InfoCircleOutlined } from '@ant-design/icons';

const { TextArea } = Input;

interface RecipeEditorProps {
  content: string;
  name: string;
  isNewRecipe: boolean;
  onChange: (content: string) => void;
  onNameChange: (name: string) => void;
}

export default function RecipeEditor({
  content,
  name,
  isNewRecipe,
  onChange,
  onNameChange,
}: RecipeEditorProps) {
  return (
    <Space direction="vertical" style={{ width: '100%' }} size="large">
      {isNewRecipe && (
        <>
          <Alert
            message="Creating New Recipe"
            description="Enter a name and YAML content for your new recipe. Use slashes to organize recipes into folders (e.g., 'workflows/ci/build')."
            type="info"
            icon={<InfoCircleOutlined />}
            showIcon
          />
          <Form layout="vertical">
            <Form.Item
              label="Recipe Name"
              required
              help="Use slashes to organize (e.g., 'workflows/build' or 'ops/deploy')"
            >
              <Input
                value={name}
                onChange={(e) => onNameChange(e.target.value)}
                placeholder="e.g., workflows/ci/build"
                style={{ fontSize: 14, fontFamily: 'monospace' }}
              />
            </Form.Item>
          </Form>
        </>
      )}

      <Form layout="vertical">
        <Form.Item label="YAML Content">
          <TextArea
            value={content}
            onChange={(e) => onChange(e.target.value)}
            rows={25}
            style={{
              fontSize: 13,
              fontFamily: 'Monaco, Menlo, "Ubuntu Mono", "Courier New", monospace',
              lineHeight: 1.6,
            }}
            placeholder="Enter YAML content..."
          />
        </Form.Item>
      </Form>

      <Alert
        message="YAML Format"
        description={
          <div>
            Recipe files should follow the standard recipe YAML format:
            <pre style={{ marginTop: 8, fontSize: 11 }}>
{`version: "1.0"
id: recipe-name
op: echo
inputs:
  message: "Hello World"`}
            </pre>
          </div>
        }
        type="info"
        showIcon
      />
    </Space>
  );
}
