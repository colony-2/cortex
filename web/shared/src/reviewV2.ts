import type { ReviewDocument, ReviewDocumentMetadata } from './services/inputActivityService';

export type ReviewScalar = string | number | boolean | null;
export interface ScalarSchema {
  type: string | string[]; enum?: ReviewScalar[]; minLength?: number; maxLength?: number; pattern?: string;
  minimum?: number; maximum?: number; exclusiveMinimum?: number; exclusiveMaximum?: number;
}
export type ReviewFeedbackMode = 'forbidden' | 'optional' | 'required';
export type ReviewV2Direction = { id: string; label: string; continues_review: boolean } & (
  { feedback_mode: ReviewFeedbackMode; allow_attachments: boolean; feedback_required?: never } |
  { feedback_required: boolean; feedback_mode?: never; allow_attachments?: never }
);
export interface ReviewV2Setting {
  id: string; control: 'text' | 'choice' | 'boolean' | 'number'; label: string; description?: string;
  value: ReviewScalar; default?: ReviewScalar; reset_label?: string; applies_to: string[]; schema: ScalarSchema;
  effect: string; choices?: { value: ReviewScalar; label: string }[];
}
export interface ReviewV2Turn {
  role: 'human' | 'agent'; text: string; attachments: ReviewDocument[]; direction?: string;
  settings?: Record<string, ReviewScalar>; receipt?: Record<string, unknown>;
}
export interface ReviewV2Request {
  review_id: string; title: string; question: string; documents: Record<string, ReviewDocument>;
  document_metadata?: Record<string, ReviewDocumentMetadata>; overview_document_id?: string;
  directions: ReviewV2Direction[]; feedback: { label: string; allow_attachments?: boolean };
  settings?: ReviewV2Setting[]; conversation: ReviewV2Turn[];
}
export interface ReviewV2Response { direction: string; feedback: string; attachments: []; settings: Record<string, ReviewScalar> }
export interface ReviewV2Output { response: Omit<ReviewV2Response, 'attachments'> & { attachments: ReviewDocument[] }; receipt: Record<string, unknown>; artifact_refs?: unknown }

export const nonblankPattern = '[^\u0009-\u000d\u0020\u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000\ufeff]';
export const hasReviewFeedback = (text: string) => new RegExp(nonblankPattern, 'u').test(text);
// Interpret a validated declaration without modifying its frozen wire shape.
export function reviewV2DirectionPolicy(request: ReviewV2Request, direction: ReviewV2Direction) {
  return direction.feedback_mode !== undefined
    ? { mode: direction.feedback_mode, allowAttachments: direction.allow_attachments }
    : { mode: direction.feedback_required ? 'required' as const : 'optional' as const, allowAttachments: request.feedback.allow_attachments! };
}
export function reviewV2ContributionError(request: ReviewV2Request, direction: ReviewV2Direction, feedback: string, fileCount: number): string | undefined {
  const policy = reviewV2DirectionPolicy(request,direction);
  if (policy.mode === 'forbidden' && feedback !== '') return 'This direction does not allow feedback. Remove the draft explicitly or choose another direction.';
  if (!policy.allowAttachments && fileCount > 0) return 'This direction does not allow attachments. Remove the files explicitly or choose another direction.';
  if (policy.mode === 'required' && !hasReviewFeedback(feedback) && !(policy.allowAttachments && fileCount > 0)) return 'This direction requires feedback or a permitted attachment.';
}
export const scalar = (v: unknown): v is ReviewScalar => v === null || typeof v === 'string' || typeof v === 'boolean' || (typeof v === 'number' && Number.isFinite(v));
const record = (v: unknown): v is Record<string, any> => v !== null && typeof v === 'object' && !Array.isArray(v);
const own = (v: object, k: string) => Object.prototype.hasOwnProperty.call(v, k);
const nonempty = (v: unknown): v is string => typeof v === 'string' && hasReviewFeedback(v);
function assert(condition: unknown, message: string): asserts condition { if (!condition) throw new Error(message); }
const unique = (values: unknown[]) => new Set(values).size === values.length;
const cleanString = (v: unknown) => typeof v === 'string' && !/[\ud800-\udfff]/u.test(v);

// Check the shared scalar pattern grammar before the ECMAScript compiler sees it.
// No anchors/flags are added: JSON Schema uses search semantics.
export function portablePattern(pattern: string): RegExp {
  assert(cleanString(pattern), 'Pattern contains an unpaired surrogate');
  const chars = Array.from(pattern); let pos = 0;
  const escaped = (inside: boolean): number => {
    assert(chars[pos++] === '\\', 'Expected escape');
    const ch = chars[pos++];
    assert(ch !== undefined && (inside ? '\\[]-^' : '\\^$.*+?()[]{}|').includes(ch), 'Unsupported pattern escape');
    return ch.codePointAt(0)!;
  };
  const atom = (): boolean => {
    const ch = chars[pos];
    if (ch === '(') { pos++; expression(); assert(chars[pos++] === ')', 'Unclosed pattern group'); return false; }
    if (ch === '[') {
      pos++; if (chars[pos] === '^') pos++;
      let count = 0;
      while (pos < chars.length && chars[pos] !== ']') {
        assert(chars[pos] !== '[', 'Nested/POSIX classes are unsupported');
        const start = chars[pos] === '\\' ? escaped(true) : chars[pos++].codePointAt(0)!;
        // An unescaped hyphen is literal only at the edges.
        assert(start !== 45 || count === 0 || chars[pos] === ']' || chars[pos - 2] === '\\', 'Unescaped class hyphen');
        if (chars[pos] === '-' && chars[pos + 1] !== ']') {
          pos++; assert(pos < chars.length && chars[pos] !== '[', 'Invalid range');
          const end = chars[pos] === '\\' ? escaped(true) : chars[pos++].codePointAt(0)!;
          assert(start <= end, 'Descending/invalid scalar range');
        }
        count++;
      }
      assert(count > 0 && chars[pos++] === ']', 'Unclosed/empty pattern class'); return true;
    }
    if (ch === '\\') { escaped(false); return true; }
    assert(ch !== undefined && !'^$.*+?()[]{}|'.includes(ch), 'Unsupported pattern syntax');
    pos++; return true;
  };
  const expression = (): void => {
    let count = 0;
    while (pos < chars.length && chars[pos] !== ')') {
      if (chars[pos] === '|') { assert(count > 0, 'Empty pattern alternative'); count = 0; pos++; continue; }
      const repeatable = atom(); count++;
      if ('?*+'.includes(chars[pos] ?? '\0')) { assert(repeatable, 'Quantified groups are unsupported'); pos++; }
      else if (chars[pos] === '{') {
        assert(repeatable, 'Quantified groups are unsupported');
        const match = /^\{(\d+)(?:,(\d+))?\}/.exec(chars.slice(pos).join(''));
        assert(match && !/^0\d/.test(match[1]) && (match[2] === undefined || !/^0\d/.test(match[2])) && Number(match[1]) <= 1000 && Number(match[2] ?? match[1]) <= 1000 && Number(match[1]) <= Number(match[2] ?? match[1]), 'Invalid repeat bounds');
        pos += match[0].length;
      }
    }
    assert(count > 0, 'Empty pattern alternative');
  };
  if (chars[pos] === '^') pos++;
  expression(); assert(pos === chars.length, 'Unexpected pattern suffix');
  return new RegExp(pattern, 'u');
}

export function validateReviewScalar(value: unknown, schema: ScalarSchema): string | undefined {
  if (!scalar(value)) return 'Enter a scalar JSON value';
  const types = Array.isArray(schema.type) ? schema.type : [schema.type];
  const type = value === null ? 'null' : typeof value;
  if (!types.includes(type) && !(type === 'number' && types.includes('integer') && Number.isInteger(value))) return `Expected ${types.join(' or ')}`;
  if (schema.enum && !schema.enum.some(v => v === value)) return 'Choose a declared value';
  if (typeof value === 'string') {
    const length = Array.from(value).length;
    if (schema.minLength !== undefined && length < schema.minLength) return `Use at least ${schema.minLength} characters`;
    if (schema.maxLength !== undefined && length > schema.maxLength) return `Use at most ${schema.maxLength} characters`;
    if (schema.pattern !== undefined && !portablePattern(schema.pattern).test(value)) return 'Value does not match the declared pattern';
  }
  if (typeof value === 'number') {
    if ((schema.minimum !== undefined && value < schema.minimum) || (schema.maximum !== undefined && value > schema.maximum) ||
        (schema.exclusiveMinimum !== undefined && value <= schema.exclusiveMinimum) || (schema.exclusiveMaximum !== undefined && value >= schema.exclusiveMaximum)) return 'Value is outside the declared bounds';
  }
}
function validateSchema(raw: unknown, control: ReviewV2Setting['control']): asserts raw is ScalarSchema {
  assert(record(raw), 'Missing scalar schema');
  const allowed = ['type','enum','minLength','maxLength','pattern','minimum','maximum','exclusiveMinimum','exclusiveMaximum'];
  assert(Object.keys(raw).every(k => allowed.includes(k)), 'Unsupported scalar schema keyword');
  const types = Array.isArray(raw.type) ? raw.type : [raw.type];
  const nonnull = types.filter(t => t !== 'null');
  assert(unique(types) && nonnull.length === 1 && types.length <= 2 && ['string','number','integer','boolean'].includes(nonnull[0]), 'Expected one scalar type with optional null');
  const type = nonnull[0];
  assert(control === 'choice' || (control === 'text' ? type === 'string' : control === 'number' ? ['number','integer'].includes(type) : type === 'boolean'), 'Control and schema type disagree');
  for (const k of ['minLength','maxLength']) if (own(raw,k)) assert(type === 'string' && Number.isInteger(raw[k]) && raw[k] >= 0, 'Invalid string bound');
  if (own(raw,'pattern')) { assert(type === 'string' && typeof raw.pattern === 'string', 'Invalid string pattern'); portablePattern(raw.pattern); }
  if (own(raw,'minLength') && own(raw,'maxLength')) assert(raw.minLength <= raw.maxLength, 'Contradictory string bounds');
  for (const k of ['minimum','maximum','exclusiveMinimum','exclusiveMaximum']) if (own(raw,k)) assert(['number','integer'].includes(type) && typeof raw[k] === 'number' && Number.isFinite(raw[k]), 'Invalid numeric bound');
  const low = Math.max(raw.minimum ?? -Infinity, raw.exclusiveMinimum ?? -Infinity);
  const high = Math.min(raw.maximum ?? Infinity, raw.exclusiveMaximum ?? Infinity);
  assert(low <= high && !(low === high && (raw.exclusiveMinimum === low || raw.exclusiveMaximum === high)), 'Contradictory numeric bounds');
  if (type === 'integer') {
    const first = Math.max(Math.ceil(raw.minimum ?? -Infinity), own(raw,'exclusiveMinimum') ? Math.floor(raw.exclusiveMinimum) + 1 : -Infinity);
    const last = Math.min(Math.floor(raw.maximum ?? Infinity), own(raw,'exclusiveMaximum') ? Math.ceil(raw.exclusiveMaximum) - 1 : Infinity);
    assert(first <= last, 'Integer schema has no values');
  }
  if (own(raw,'enum')) {
    assert(Array.isArray(raw.enum) && raw.enum.length > 0 && raw.enum.every(scalar) && unique(raw.enum), 'Invalid scalar enum');
    const withoutEnum = { ...raw }; delete withoutEnum.enum;
    assert(raw.enum.every((v: unknown) => !validateReviewScalar(v, withoutEnum as ScalarSchema)), 'Enum value violates scalar schema');
  }
}
export function isReviewDocument(raw: unknown): raw is ReviewDocument {
  if (!record(raw) || raw.kind !== 'stored' || !record(raw.stored) || !record(raw.stored.key)) return false;
  const k = raw.stored.key;
  return nonempty(k.jobId) && Number.isSafeInteger(k.taskOrdinal) && k.taskOrdinal >= 0 && nonempty(k.name) && Number.isSafeInteger(k.sizeBytes) && k.sizeBytes >= 0 && (!own(raw,'name') || typeof raw.name === 'string');
}
export function parseReviewV2(raw: unknown): ReviewV2Request {
  assert(record(raw), 'Missing structured review request');
  assert(nonempty(raw.review_id) && nonempty(raw.title) && typeof raw.question === 'string', 'Missing review identity, title or question');
  assert(record(raw.documents) && Object.entries(raw.documents).every(([id,ref]) => nonempty(id) && isReviewDocument(ref)), 'Invalid review document reference');
  const ids = Object.keys(raw.documents);
  if (own(raw,'overview_document_id')) assert(ids.includes(raw.overview_document_id), 'Overview is not an attached document');
  if (own(raw,'document_metadata')) {
    assert(record(raw.document_metadata), 'Invalid document metadata');
    for (const [id, m] of Object.entries(raw.document_metadata)) {
      assert(ids.includes(id) && record(m), 'Metadata names an unattached document');
      for (const [key, limit] of [['title',120],['description',300]] as const) if (own(m,key)) assert(typeof m[key] === 'string' && Array.from(m[key]).length <= limit && !/[\x00-\x1f\x7f]/.test(m[key]), 'Invalid document display text');
      if (own(m,'role')) assert(['','neutral','primary','supporting','diagnostic'].includes(m.role), 'Invalid document role');
      if (own(m,'order')) assert(Number.isInteger(m.order) && m.order >= 0 && m.order <= 1000000, 'Invalid document order');
      if (own(m,'source_document_ids')) assert(Array.isArray(m.source_document_ids) && m.source_document_ids.length <= 128 && unique(m.source_document_ids) && m.source_document_ids.every((v: unknown) => typeof v === 'string' && ids.includes(v) && v !== id), 'Invalid document sources');
    }
  }
  assert(new TextEncoder().encode(JSON.stringify({document_metadata:raw.document_metadata,overview_document_id:raw.overview_document_id})).length <= 65536, 'Document presentation exceeds 65536 bytes');
  assert(Array.isArray(raw.directions) && raw.directions.length > 0, 'No review directions');
  assert(record(raw.feedback) && nonempty(raw.feedback.label), 'Missing feedback policy');
  const legacy = own(raw.feedback,'allow_attachments');
  if (legacy) assert(typeof raw.feedback.allow_attachments === 'boolean', 'Invalid frozen feedback policy');
  const directions: string[] = [];
  for (const d of raw.directions) {
    assert(record(d) && nonempty(d.id) && nonempty(d.label) && typeof d.continues_review === 'boolean', 'Directions require explicit feedback and continuation rules');
    if (legacy) {
      assert(typeof d.feedback_required === 'boolean' && !own(d,'feedback_mode') && !own(d,'allow_attachments'), 'Mixed or incomplete frozen direction rules');
    } else {
      assert(!own(d,'feedback_required') && ['forbidden','optional','required'].includes(d.feedback_mode) && typeof d.allow_attachments === 'boolean', 'Mixed or incomplete direction rules');
      assert(d.feedback_mode !== 'forbidden' || !d.allow_attachments, 'Forbidden feedback cannot allow attachments');
    }
    assert(!directions.includes(d.id), 'Duplicate direction ID'); directions.push(d.id);
  }
  assert(!own(raw,'settings') || Array.isArray(raw.settings), 'Settings must be an ordered array');
  const settingIDs: string[] = [];
  for (const s of raw.settings ?? []) {
    assert(record(s) && nonempty(s.id) && nonempty(s.label) && nonempty(s.effect), 'Invalid setting declaration');
    assert(!settingIDs.includes(s.id), 'Duplicate setting ID'); settingIDs.push(s.id);
    assert(['text','choice','boolean','number'].includes(s.control), 'Unsupported setting control');
    assert(!own(s,'description') || typeof s.description === 'string', 'Invalid setting description');
    assert(Array.isArray(s.applies_to) && s.applies_to.length > 0 && unique(s.applies_to) && s.applies_to.every((id: unknown) => typeof id === 'string' && directions.includes(id)), 'Invalid setting applicability');
    validateSchema(s.schema,s.control);
    assert(own(s,'value') && !validateReviewScalar(s.value,s.schema), 'Invalid current setting value');
    assert(own(s,'default') === own(s,'reset_label'), 'Reset needs a default and a label');
    if (own(s,'default')) assert(nonempty(s.reset_label) && !validateReviewScalar(s.default,s.schema), 'Invalid reset value');
    if (s.control === 'choice') {
      assert(Array.isArray(s.choices) && s.choices.length > 0 && s.choices.every((c: unknown) => record(c) && nonempty(c.label) && own(c,'value') && !validateReviewScalar(c.value,s.schema)), 'Invalid setting choices');
      assert(unique(s.choices.map((c: {value: unknown}) => c.value)), 'Duplicate choice values');
      for (const value of [s.value, ...(own(s,'default') ? [s.default] : [])]) assert(s.choices.some((c: {value: unknown}) => c.value === value) || (value === null && s.default === null && own(s,'reset_label')), 'Current/default value is not a choice');
    } else assert(!own(s,'choices'), 'Choices require a choice control');
  }
  assert(Array.isArray(raw.conversation), 'Missing conversation');
  for (const turn of raw.conversation) {
    assert(record(turn) && ['human','agent'].includes(turn.role) && typeof turn.text === 'string' && Array.isArray(turn.attachments) && turn.attachments.every(isReviewDocument), 'Invalid conversation turn');
    if (turn.role === 'human') assert(nonempty(turn.direction) && record(turn.receipt), 'Human turn needs direction and recorded receipt');
    if (own(turn,'settings')) assert(turn.role === 'human' && record(turn.settings) && Object.values(turn.settings).every(scalar), 'Invalid historical settings');
  }
  return raw as ReviewV2Request;
}
export function changedReviewSettings(request: ReviewV2Request, direction: string, drafts: Record<string, unknown>): Record<string, ReviewScalar> {
  const result: Record<string, ReviewScalar> = Object.create(null);
  for (const setting of request.settings ?? []) {
    if (!setting.applies_to.includes(direction) || !own(drafts,setting.id) || drafts[setting.id] === setting.value) continue;
    const value = drafts[setting.id]; const error = validateReviewScalar(value,setting.schema);
    assert(!error, `${setting.label}: ${error}`);
    assert(setting.control !== 'choice' || setting.choices?.some(c => c.value === value) || (value === null && setting.default === null && own(setting,'reset_label')), `${setting.label}: choose a declared value`);
    result[setting.id] = value as ReviewScalar;
  }
  return result;
}
