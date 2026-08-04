"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { workflows, workspaces, type Graph, type Workflow } from "./api";

/**
 * Query keys in one place, so an invalidation and a fetch cannot disagree about
 * what they are naming.
 */
export const keys = {
  workspaces: ["workspaces"] as const,
  workflows: (workspaceId: string) => ["workflows", workspaceId] as const,
  workflow: (workspaceId: string, id: string) => ["workflow", workspaceId, id] as const,
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
