import axios from "axios";

import { getToken } from "./auth-client";

/**
 * Base URL of the Go API server.
 *
 * NEXT_PUBLIC_ because the browser calls the backend directly with a minted
 * JWT — nothing about this request goes through Next.
 */
export const API_URL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8090";

/**
 * The client every call to the Go backend goes through.
 *
 * The interceptor below is the reason this exists rather than bare fetch: a
 * Better Auth token lives 15 minutes, so it has to be minted per request.
 * Doing that by hand at every call site is one forgotten `await` away from a
 * 401 that looks like a permissions bug.
 */
export const api = axios.create({
  baseURL: API_URL,
  headers: { "Content-Type": "application/json" },
});

api.interceptors.request.use(async (config) => {
  const token = await getToken();
  if (token) config.headers.Authorization = `Bearer ${token}`;
  return config;
});

/**
 * The message the backend actually sent, or a fallback.
 *
 * Every error body from the Go side is `{"error": "..."}`, and for a 400 that
 * text is written to be shown to a person — a broken graph names the node that
 * is wrong. Axios's own `err.message` throws that away and reports "Request
 * failed with status code 400" instead.
 */
export function apiError(err: unknown): string {
  if (axios.isAxiosError(err)) {
    return err.response?.data?.error ?? err.message;
  }
  return err instanceof Error ? err.message : "Something went wrong";
}

/**
 * A workflow graph, in React Flow's shape.
 *
 * These are the field names `useNodesState` and `useEdgesState` already hold,
 * so a save is the array the canvas has rather than a translation of it. Keep
 * it that way — a mapping layer is somewhere for the two to drift apart.
 *
 * `data` is opaque to the backend. It carries the node's label and whatever
 * configuration its type needs, and is only checked for being valid JSON.
 */
export type WorkflowNode = {
  id: string;
  type: string;
  position: { x: number; y: number };
  data?: Record<string, unknown>;
};

export type WorkflowEdge = {
  id: string;
  source: string;
  target: string;
  sourceHandle?: string;
};

export type Graph = {
  nodes: WorkflowNode[];
  edges: WorkflowEdge[];
};

export type Workflow = {
  id: string;
  name: string;
  description: string;
  graph: Graph;
  createdBy?: string;
  createdAt: string;
  updatedAt: string;
};

/**
 * Workflow endpoints.
 *
 * A save is a draft: the backend stores half-built graphs on purpose, so
 * autosaving a canvas mid-edit is expected rather than an error. It rejects
 * only a graph that is structurally broken — a duplicate id, an edge to a node
 * that is not there. Whether a workflow can actually run is asked at run time.
 */
export const workflows = {
  list: (workspaceId: string) =>
    api
      .get<{ workflows: Workflow[] }>(`/api/workspaces/${workspaceId}/workflows`)
      .then((r) => r.data.workflows),

  get: (workspaceId: string, id: string) =>
    api.get<Workflow>(`/api/workspaces/${workspaceId}/workflows/${id}`).then((r) => r.data),

  create: (workspaceId: string, body: { name: string; description?: string; graph: Graph }) =>
    api.post<Workflow>(`/api/workspaces/${workspaceId}/workflows`, body).then((r) => r.data),

  /**
   * Partial update: send only what changed.
   *
   * An omitted field is left alone and an empty one is written, so the builder
   * can autosave `{ graph }` without blanking the description.
   */
  update: (
    workspaceId: string,
    id: string,
    patch: { name?: string; description?: string; graph?: Graph },
  ) =>
    api
      .patch<Workflow>(`/api/workspaces/${workspaceId}/workflows/${id}`, patch)
      .then((r) => r.data),

  remove: (workspaceId: string, id: string) =>
    api.delete(`/api/workspaces/${workspaceId}/workflows/${id}`).then(() => undefined),

  /**
   * Ask a model for a workflow. Nothing is stored.
   *
   * The graph comes back for the canvas to open, and saving it is a separate
   * `create` the user triggers after looking at it — AI assists, it does not
   * own. Put the result straight into `useNodesState`/`useEdgesState`; it is
   * already in React Flow's shape.
   */
  generate: (workspaceId: string, prompt: string) =>
    api
      .post<Generated>(`/api/workspaces/${workspaceId}/workflows/generate`, { prompt })
      .then((r) => r.data),
};

/** A proposed workflow. It has no id, because it does not exist yet. */
export type Generated = {
  name: string;
  description: string;
  graph: Graph;
};
