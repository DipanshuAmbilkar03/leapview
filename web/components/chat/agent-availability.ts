/**
 * The agent signal carries `status.enabled`. Surfaces that only exist to talk to
 * the agent (Ask buttons, the drawer) are hidden once the server says it is off;
 * an absent flag keeps them, so older or partial signals do not hide the agent.
 */
export function agentUnavailable(agent: { status?: { enabled?: boolean } } | null | undefined): boolean {
  return agent?.status?.enabled === false
}
