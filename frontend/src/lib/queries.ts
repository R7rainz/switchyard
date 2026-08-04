"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { executions, workflows, workspaces, type Graph, type Workflow } from "./api";

/**
 * Query keys in one place, so an invalidation and a fetch cannot disagree about
 * what they are naming.
 */
export const keys = {
  workspaces: ["workspaces"] as const,
  workflows: (workspaceId: string) => ["workflows", workspaceId] as const,
  workflow: (workspaceId: string, id: string) => ["workflow", workspaceId, id] as const,
  executions: (workspaceId: string) => ["executions", workspaceId] as const,
};

/**
 * The current workspace.
 *
 * Listing creates a personal one for an account that has none, so there is no
 * "you have no workspace" state to design for. Multiple workspaces get a picker
 * when there is a second way to make one; until then the first is the one.
 */
export function useWorkspace() {
  const query = useQuery({ queryKey: keys.workspaces, queryFn: workspaces.list });
  return { ...query, workspace: query.data?.[0] };
}

export function useWorkflows(workspaceId: string | undefined) {
  return useQuery({
    queryKey: keys.workflows(workspaceId ?? ""),
    queryFn: () => workflows.list(workspaceId!),
    enabled: Boolean(workspaceId),
  });
}

export function useWorkflow(workspaceId: string | undefined, id: string) {
  return useQuery({
    queryKey: keys.workflow(workspaceId ?? "", id),
    queryFn: () => workflows.get(workspaceId!, id),
    enabled: Boolean(workspaceId),
  });
}

export function useCreateWorkflow(workspaceId: string | undefined) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (body: { name: string; description?: string; graph: Graph }) =>
      workflows.create(workspaceId!, body),
    onSuccess: () => client.invalidateQueries({ queryKey: keys.workflows(workspaceId ?? "") }),
  });
}

export function useDeleteWorkflow(workspaceId: string | undefined) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => workflows.remove(workspaceId!, id),
    // The list is refetched rather than spliced. A delete is rare and the
    // authoritative answer is one request away; hand-maintaining the cache is
    // how it drifts from the server.
    onSuccess: () => client.invalidateQueries({ queryKey: keys.workflows(workspaceId ?? "") }),
  });
}

/** An empty canvas. A save is a draft, so the backend takes this happily. */
export const emptyGraph: Graph = { nodes: [], edges: [] };

export type { Workflow };

/**
 * A workspace's runs, newest first.
 *
 * Polled rather than streamed. The WebSocket carries one execution's events and
 * needs an id to subscribe to; a dashboard is watching for runs it has not
 * heard of yet, which is a different question. A slow interval answers it
 * without opening a socket per row.
 */
export function useExecutions(workspaceId: string | undefined, limit = 8) {
  return useQuery({
    queryKey: [...keys.executions(workspaceId ?? ""), limit],
    queryFn: () => executions.list(workspaceId!, { limit }),
    enabled: Boolean(workspaceId),
    refetchInterval: 5000,
  });
}

export function useStartExecution(workspaceId: string | undefined) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (workflowId: string) => executions.start(workspaceId!, workflowId),
    // A run appears in the list the moment it is accepted, so the strip shows
    // it PENDING rather than staying empty until the next poll.
    onSuccess: () => client.invalidateQueries({ queryKey: keys.executions(workspaceId ?? "") }),
  });
}
