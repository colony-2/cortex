import { ApiError } from '@colony2/openapi-client';

/**
 * Extract error message from API error response.
 * The server returns errors in the format: { error: "detailed message" }
 * The ApiError.body contains this, so we need to extract it.
 */
export function getErrorMessage(error: any, fallback: string): string {
  // Check if it's an ApiError with a body containing an error field
  if (error instanceof ApiError && error.body?.error) {
    return error.body.error;
  }
  // Fall back to the error message or the provided fallback
  return error?.message || fallback;
}
