import { describe, it, expect } from 'vitest';
import { adaptInputFormConfig } from './formAdapter';
import type { InputFormConfig } from '@colony2/shared';

describe('adaptInputFormConfig', () => {
  const jobId = 'test-job-123';

  describe('single question format', () => {
    it('should handle question with explicit type', () => {
      const config: InputFormConfig = {
        question: 'What is your name?',
        type: 'short_answer',
      };

      const result = adaptInputFormConfig(config, jobId);

      expect(result).toEqual({
        id: jobId,
        title: 'What is your name?',
        responseField: 'response',
        description: undefined,
        fields: [
          {
            id: 'response',
            label: 'What is your name?',
            type: 'short_answer',
            required: true,
            options: undefined,
            min: undefined,
            max: undefined,
            placeholder: undefined,
            validation: undefined,
          },
        ],
      });
    });

    it('should default to paragraph_text when type is missing', () => {
      const config: InputFormConfig = {
        question: 'provide a user prompt',
        context: {},
      };

      const result = adaptInputFormConfig(config, jobId);

      expect(result).toEqual({
        id: jobId,
        title: 'provide a user prompt',
        responseField: 'response',
        description: undefined,
        fields: [
          {
            id: 'response',
            label: 'provide a user prompt',
            type: 'paragraph_text',
            required: true,
            options: undefined,
            min: undefined,
            max: undefined,
            placeholder: undefined,
            validation: undefined,
          },
        ],
      });
    });

    it('should handle question with options', () => {
      const config: InputFormConfig = {
        question: 'Select your favorite color',
        type: 'multiple_choice',
        options: [
          { value: 'red', label: 'Red' },
          { value: 'blue', label: 'Blue' },
          { value: 'green', label: 'Green' },
        ],
      };

      const result = adaptInputFormConfig(config, jobId);

      expect(result.fields[0].type).toBe('multiple_choice');
      expect(result.fields[0].options).toEqual(['red', 'blue', 'green']);
    });

    it('should handle question with string options', () => {
      const config: InputFormConfig = {
        question: 'Select a size',
        type: 'dropdown',
        options: ['Small', 'Medium', 'Large'],
      };

      const result = adaptInputFormConfig(config, jobId);

      expect(result.fields[0].options).toEqual(['Small', 'Medium', 'Large']);
    });

    it('should handle question with linear scale', () => {
      const config: InputFormConfig = {
        question: 'Rate your experience',
        type: 'linear_scale',
        scale: {
          min: 1,
          max: 5,
          min_label: 'Poor',
          max_label: 'Excellent',
        },
      };

      const result = adaptInputFormConfig(config, jobId);

      expect(result.fields[0].type).toBe('linear_scale');
      expect(result.fields[0].min).toBe(1);
      expect(result.fields[0].max).toBe(5);
    });
  });

  describe('multi-field format', () => {
    it('should handle multiple fields', () => {
      const config: InputFormConfig = {
        title: 'User Registration Form',
        fields: [
          {
            id: 'name',
            type: 'short_answer',
            question: 'What is your name?',
            required: true,
          },
          {
            id: 'email',
            type: 'short_answer',
            question: 'What is your email?',
            required: true,
            placeholder: 'user@example.com',
            validation: {
              pattern: '^[\\w-\\.]+@([\\w-]+\\.)+[\\w-]{2,4}$',
            },
          },
          {
            id: 'age',
            type: 'short_answer',
            question: 'What is your age?',
            required: false,
          },
        ],
      };

      const result = adaptInputFormConfig(config, jobId);

      expect(result.id).toBe(jobId);
      expect(result.title).toBe('User Registration Form');
      expect(result.fields).toHaveLength(3);

      expect(result.fields[0]).toEqual({
        id: 'name',
        label: 'What is your name?',
        type: 'short_answer',
        required: true,
        options: undefined,
        min: undefined,
        max: undefined,
        placeholder: undefined,
        validation: undefined,
      });

      expect(result.fields[1]).toEqual({
        id: 'email',
        label: 'What is your email?',
        type: 'short_answer',
        required: true,
        options: undefined,
        min: undefined,
        max: undefined,
        placeholder: 'user@example.com',
        validation: {
          pattern: '^[\\w-\\.]+@([\\w-]+\\.)+[\\w-]{2,4}$',
          message: 'Invalid format',
        },
      });

      expect(result.fields[2].required).toBe(false);
    });

    it('should use default title when not provided', () => {
      const config: InputFormConfig = {
        fields: [
          {
            id: 'response',
            type: 'short_answer',
            question: 'Enter something',
          },
        ],
      };

      const result = adaptInputFormConfig(config, jobId);

      expect(result.title).toBe('Input Request');
    });

    it('should handle field with options', () => {
      const config: InputFormConfig = {
        title: 'Survey',
        fields: [
          {
            id: 'choice',
            type: 'multiple_choice',
            question: 'Pick one',
            options: [
              { value: 'option1', label: 'Option 1' },
              { value: 'option2', label: 'Option 2' },
            ],
          },
        ],
      };

      const result = adaptInputFormConfig(config, jobId);

      expect(result.fields[0].options).toEqual(['option1', 'option2']);
    });

    it('should handle field with scale', () => {
      const config: InputFormConfig = {
        title: 'Rating Form',
        fields: [
          {
            id: 'rating',
            type: 'linear_scale',
            question: 'Rate this',
            scale: {
              min: 0,
              max: 10,
            },
          },
        ],
      };

      const result = adaptInputFormConfig(config, jobId);

      expect(result.fields[0].min).toBe(0);
      expect(result.fields[0].max).toBe(10);
    });
  });

  describe('edge cases', () => {
    it('should handle empty config with no fields', () => {
      const config: InputFormConfig = {
        title: 'Empty Form',
      };

      const result = adaptInputFormConfig(config, jobId);

      expect(result.title).toBe('Empty Form');
      expect(result.fields).toEqual([]);
    });

    it('should handle config with undefined fields', () => {
      const config: InputFormConfig = {
        title: 'No Fields Form',
        fields: undefined,
      };

      const result = adaptInputFormConfig(config, jobId);

      expect(result.fields).toEqual([]);
    });

    it('should prioritize question format over multi-field format', () => {
      // If both question and fields are present, question takes precedence
      const config: InputFormConfig = {
        question: 'Single question',
        type: 'short_answer',
        title: 'Multi-field title',
        fields: [
          {
            id: 'field1',
            type: 'paragraph_text',
            question: 'Field 1',
          },
        ],
      };

      const result = adaptInputFormConfig(config, jobId);

      expect(result.title).toBe('Single question');
      expect(result.fields).toHaveLength(1);
      expect(result.fields[0].type).toBe('short_answer');
    });
  });

  describe('real-world API responses', () => {
    it('should handle API response from bug report', () => {
      // This is the actual API response from the user's bug report
      const config: InputFormConfig = {
        question: 'provide a user prompt',
        context: {},
      };

      const result = adaptInputFormConfig(config, jobId);

      // Should create a form with a paragraph_text field
      expect(result.id).toBe(jobId);
      expect(result.title).toBe('provide a user prompt');
      expect(result.fields).toHaveLength(1);
      expect(result.fields[0]).toEqual({
        id: 'response',
        label: 'provide a user prompt',
        type: 'paragraph_text',
        required: true,
        options: undefined,
        min: undefined,
        max: undefined,
        placeholder: undefined,
        validation: undefined,
      });
    });
  });
});
