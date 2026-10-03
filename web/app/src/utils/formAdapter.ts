import type { InputFormConfig, InputForm, InputField } from '@colony2/shared';

/**
 * Adapts API InputFormConfig to component-friendly InputForm structure
 * for use with InputFormRenderer component.
 */
export function adaptInputFormConfig(
  config: InputFormConfig,
  jobId: string
): InputForm {
  // Single question format
  if (config.question) {
    // Default to paragraph_text if type is not specified
    const fieldType = config.type || 'paragraph_text';

    return {
      id: jobId,
      kind: config.kind,
      title: config.question,
      responseField: 'response',
      description: undefined,
      fields: [
        {
          id: 'response',
          label: config.question,
          type: fieldType,
          required: true,
          options: config.options?.map((o) => (typeof o === 'string' ? o : o.value)),
          optionLabels: config.kind === 'review' ? Object.fromEntries((config.options || []).map(o => [o.value, o.label || o.value])) : undefined,
          min: config.scale?.min,
          max: config.scale?.max,
          placeholder: undefined,
          validation: undefined,
        },
      ],
    };
  }

  // Multi-field format
  const fields: InputField[] =
    config.fields?.map((field) => ({
      id: field.id,
      label: field.question,
      type: field.type,
      required: field.required || false,
      options: field.options?.map((o) => (typeof o === 'string' ? o : o.value)),
      optionLabels: config.kind === 'review' ? Object.fromEntries((field.options || []).map(o => [o.value, o.label || o.value])) : undefined,
      min: field.scale?.min,
      max: field.scale?.max,
      placeholder: field.placeholder,
      validation: field.validation?.pattern
        ? {
            pattern: field.validation.pattern,
            message: `Invalid format`,
          }
        : undefined,
    })) || [];

  return {
    id: jobId,
    kind: config.kind,
    title: config.title || 'Input Request',
    description: undefined,
    fields,
  };
}
