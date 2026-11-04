export function cellLabel(name: string) {
  if (!name?.trim()) throw new Error('Invalid job response: required cell_name is missing');
  return name;
}
