import { describe, it, expect } from 'vitest';
import { ApiError } from '@colony2/openapi-client';
import { getErrorMessage } from './errorHandling';

describe('getErrorMessage', () => {
  it('should extract error message from ApiError.body.error', () => {
    const apiError = new ApiError(
      { method: 'POST', url: '/api/test' } as any,
      {
        url: '/api/test',
        status: 400,
        statusText: 'Bad Request',
        body: { error: 'recipe: invalid recipe content: unknown op: [echo] at [1:1]' },
      } as any,
      'Invalid request (validation failed)'
    );

    const result = getErrorMessage(apiError, 'Fallback message');
    expect(result).toBe('recipe: invalid recipe content: unknown op: [echo] at [1:1]');
  });

  it('should use error.message when not an ApiError', () => {
    const regularError = new Error('Network error');
    const result = getErrorMessage(regularError, 'Fallback message');
    expect(result).toBe('Network error');
  });

  it('should use fallback when error is null', () => {
    const result = getErrorMessage(null, 'Fallback message');
    expect(result).toBe('Fallback message');
  });

  it('should use fallback when ApiError.body.error is missing', () => {
    const apiError = new ApiError(
      { method: 'POST', url: '/api/test' } as any,
      {
        url: '/api/test',
        status: 500,
        statusText: 'Internal Server Error',
        body: {},
      } as any,
      'Server error'
    );

    const result = getErrorMessage(apiError, 'Fallback message');
    expect(result).toBe('Server error');
  });

  it('should handle ApiError with null body', () => {
    const apiError = new ApiError(
      { method: 'POST', url: '/api/test' } as any,
      {
        url: '/api/test',
        status: 500,
        statusText: 'Internal Server Error',
        body: null,
      } as any,
      'Server error'
    );

    const result = getErrorMessage(apiError, 'Fallback message');
    expect(result).toBe('Server error');
  });

  it('should extract detailed validation errors', () => {
    const apiError = new ApiError(
      { method: 'POST', url: '/api/recipes' } as any,
      {
        url: '/api/recipes',
        status: 400,
        statusText: 'Bad Request',
        body: {
          error: 'recipe: invalid recipe content: field "version" is required at [1:1]'
        },
      } as any,
      'Invalid request'
    );

    const result = getErrorMessage(apiError, 'Failed to save recipe');
    expect(result).toBe('recipe: invalid recipe content: field "version" is required at [1:1]');
  });

  it('should extract message from ApiError.body.errors array', () => {
    const apiError = new ApiError(
      { method: 'POST', url: '/api/recipes/test/publish' } as any,
      {
        url: '/api/recipes/test/publish',
        status: 400,
        statusText: 'Bad Request',
        body: {
          errors: [
            {
              code: 'cel_invalid',
              message: "failed to compile CEL expression: ERROR: <input>:1:5: undeclared reference to 'outputs'",
            },
          ],
          valid: false,
        },
      } as any,
      'Invalid request'
    );

    const result = getErrorMessage(apiError, 'Failed to publish recipe');
    expect(result).toBe(
      "failed to compile CEL expression: ERROR: <input>:1:5: undeclared reference to 'outputs'"
    );
  });

  it('should join multiple messages from ApiError.body.errors array', () => {
    const apiError = new ApiError(
      { method: 'POST', url: '/api/test' } as any,
      {
        url: '/api/test',
        status: 400,
        statusText: 'Bad Request',
        body: {
          errors: [{ message: 'first error' }, { message: 'second error' }],
        },
      } as any,
      'Invalid request'
    );

    const result = getErrorMessage(apiError, 'Fallback message');
    expect(result).toBe('first error\nsecond error');
  });

  it('should extract messages from a JSON string body', () => {
    const apiError = new ApiError(
      { method: 'POST', url: '/api/test' } as any,
      {
        url: '/api/test',
        status: 400,
        statusText: 'Bad Request',
        body: '{"errors":[{"message":"json body error"}]}',
      } as any,
      'Invalid request'
    );

    const result = getErrorMessage(apiError, 'Fallback message');
    expect(result).toBe('json body error');
  });

  it('should extract messages from an axios-style error', () => {
    const axiosLikeError = {
      message: 'Invalid request',
      response: {
        data: { errors: [{ message: 'validation failed: missing field' }] },
      },
    };

    const result = getErrorMessage(axiosLikeError, 'Fallback message');
    expect(result).toBe('validation failed: missing field');
  });
});
