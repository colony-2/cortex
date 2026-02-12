import { ApiError } from '@colony2/openapi-client';

/**
 * Extract error message from API error response.
 * The server may return errors in formats like:
 * - { error: "detailed message" }
 * - { message: "detailed message" }
 * - { errors: [{ message: "detailed message", code?: string }, ...] }
 * - a JSON string containing any of the above
 */
export function getErrorMessage(error: any, fallback: string): string {
  if (!error) return fallback;

  const candidates: any[] = [
    error instanceof ApiError ? error.body : undefined,
    error?.body,
    error?.response?.body,
    error?.response?.data,
    error?.data,
    error,
  ].filter((value) => value !== undefined);

  for (const candidate of candidates) {
    const extracted = extractMessages(candidate);
    if (extracted.length > 0) {
      return extracted.join('\n');
    }
  }

  return error?.message || fallback;
}

function extractMessages(value: any): string[] {
  if (!value) return [];

  if (typeof value === 'string') {
    const parsed = tryParseJson(value);
    if (parsed !== undefined) return extractMessages(parsed);
    return [value].map(cleanMessage).filter(Boolean);
  }

  if (Array.isArray(value)) {
    const messages = value.flatMap((item) => extractMessages(item));
    return uniqueNonEmpty(messages);
  }

  if (typeof value !== 'object') return [];

  const obj = value as Record<string, any>;

  if (Array.isArray(obj.errors)) {
    const messages = obj.errors.flatMap((err: any) => {
      if (!err) return [];
      if (typeof err === 'string') return [err];
      if (typeof err.message === 'string') return [err.message];
      if (typeof err.error === 'string') return [err.error];
      return [];
    });
    return uniqueNonEmpty(messages.map(cleanMessage));
  }

  if (typeof obj.error === 'string') return [cleanMessage(obj.error)].filter(Boolean) as string[];
  if (typeof obj.message === 'string') return [cleanMessage(obj.message)].filter(Boolean) as string[];
  if (typeof obj.detail === 'string') return [cleanMessage(obj.detail)].filter(Boolean) as string[];
  if (typeof obj.title === 'string') return [cleanMessage(obj.title)].filter(Boolean) as string[];

  return [];
}

function tryParseJson(text: string): any | undefined {
  const trimmed = text.trim();
  if (!trimmed.startsWith('{') && !trimmed.startsWith('[')) return undefined;

  try {
    return JSON.parse(trimmed);
  } catch {
    return undefined;
  }
}

function cleanMessage(message: string): string {
  return message.trim();
}

function uniqueNonEmpty(values: string[]): string[] {
  const seen = new Set<string>();
  const result: string[] = [];

  for (const value of values) {
    const cleaned = cleanMessage(value);
    if (!cleaned) continue;
    if (seen.has(cleaned)) continue;
    seen.add(cleaned);
    result.push(cleaned);
  }

  return result;
}
