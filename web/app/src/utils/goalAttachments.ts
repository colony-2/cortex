export function validateGoalAttachments(files: readonly File[]): void {
  if (files.length > 10) throw new Error('Attach up to 10 files.');
  if (files.reduce((sum, file) => sum + file.size, 0) > 25 * 1024 * 1024) throw new Error('Attachments must total 25 MiB or less.');
  const names = new Set<string>();
  for (const file of files) {
    if (!file.name.trim() || file.name === '.' || file.name === '..' || /[/\\=\x00-\x1f\x7f-\x9f]/u.test(file.name) || /^.:/u.test(file.name)) throw new Error(`Invalid filename “${file.name}”. Rename the file without paths or control characters.`);
    if (names.has(file.name)) throw new Error(`“${file.name}” is already attached. Remove it or rename the file locally.`);
    names.add(file.name);
  }
}
