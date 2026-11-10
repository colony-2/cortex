import { expect, it } from 'vitest';
import { dagPath, dagReturnPath } from './navigation';
it('DAG-09 routes known recipes to the story DAG and generic jobs to standalone', () => {
 expect(dagPath('t','j','recipe')).toBe('/project/t/jobs/j/story?tab=dag');
 expect(dagPath('t','j','generic')).toBe('/project/t/jobs/j/tree');
});
it.each(['/project/t/jobs/entry/tree','/project/t/jobs/entry/story?tab=dag'])('DAG-10 accepts entry destinations %s', value => expect(dagReturnPath(value,'t')).toBe(value));
it.each(['https://evil.test/project/t/jobs/j/tree','//evil.test/project/t/jobs/j/tree','/project/other/jobs/j/tree','/project/t/jobs/j/story?tab=summary','/project/t/jobs/j/story?tab=dag&other=1','/project/t/jobs/../tree','/project/t/jobs/%2e%2e/tree','/project/t/jobs/%2F/tree','/project/t/jobs/%/tree',null,{}])('DAG-10 rejects unsafe or unrelated return %s', value => expect(dagReturnPath(value,'t')).toBeUndefined());
