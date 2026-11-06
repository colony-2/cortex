import { expect, it } from 'vitest';
import { validateGoalAttachments } from './goalAttachments';
const file = (name: string, size = 0) => new File([new Uint8Array(size)], name);
it('ATT-004 ATT-006 accepts binary, Unicode, empty, and case-sensitive names', () => {
 expect(() => validateGoalAttachments([file('雪.bin', 2), file('A'), file('a'), file('my brief 雪.txt')])).not.toThrow();
});
it('ATT-007 ATT-008 accepts exact limits and rejects one byte or file more', () => {
 const full = file('full', 25 * 1024 * 1024);
 expect(() => validateGoalAttachments([full])).not.toThrow();
 const ten = [full, ...Array.from({ length: 9 }, (_, i) => file(String(i)))];
 expect(() => validateGoalAttachments(ten)).not.toThrow();
 expect(() => validateGoalAttachments([...ten, file('11')])).toThrow('10 files');
 expect(() => validateGoalAttachments([full, file('extra', 1)])).toThrow('25 MiB');
});
it.each(['../x', '/x', 'a\\b', 'C:x', '=x', '.', '..', ' ', '\u0001'])('ATT-026 rejects unsafe filename %s', name => {
 expect(() => validateGoalAttachments([file(name)])).toThrow('Invalid filename');
});
