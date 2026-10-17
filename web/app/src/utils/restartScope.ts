import type { StoryNode } from './storyTree';

export interface RestartScope {
  scope_id: string;
  started_at: string;
}

const containsInvocation = (scope: string, invocation: string) =>
  invocation === scope || invocation.startsWith(`${scope}/`);

// Match recorded identities, never display names, ordinals, or list order.
export function restartScopeForNode(node: StoryNode | null, scopes: RestartScope[]): string | undefined {
  if (!node) return undefined;
  if (node.invocation_id) {
    return scopes.filter(scope => containsInvocation(scope.scope_id, node.invocation_id!))
      .sort((a, b) => b.scope_id.length - a.scope_id.length)[0]?.scope_id;
  }
  if (node.kind === 'recipe') {
    return scopes.find(scope => scopes.every(other => containsInvocation(scope.scope_id, other.scope_id)))?.scope_id;
  }
  // c2j does not yet expose a reliable mapping for every structural story node.
  return undefined;
}
