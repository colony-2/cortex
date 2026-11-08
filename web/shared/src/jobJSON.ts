import { parse, stringify, isSafeNumber, LosslessNumber, isLosslessNumber } from 'lossless-json';

// Keep opaque payload numbers exact. Use stringifyJobJSON when displaying job
// data: native JSON.stringify would serialize LosslessNumber's implementation.
export function parseJobJSON(text: string): unknown {
  return parse(text, undefined, value => isSafeNumber(value) ? Number(value) : new LosslessNumber(value));
}

export function stringifyJobJSON(value: unknown, space?: number): string | undefined {
  return stringify(value, undefined, space);
}

// Numeric leaf guard for read-only payload presentations.
export const isJobJSONNumber = isLosslessNumber;
