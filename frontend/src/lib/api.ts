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
 * A workspace owns everything: workflows, executions, credentials. Access is a
 * membership question, so every path below is scoped to one.
 *
 * Listing creates a personal workspace for an account that has none, so the
 * frontend never has to handle "signed in but nowhere to be".
 */
export type Workspace = {
  id: string;
  name: string;
  slug: string;
  createdAt: string;
};

export const workspaces = {
  list: () => api.get<{ workspaces: Workspace[] }>("/api/workspaces").then((r) => r.data.workspaces),
};

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

export type ExecutionStatus =
  | "PENDING"
  | "RUNNING"
  | "SUCCEEDED"
  | "FAILED"
  | "CANCELLED"
  | "SKIPPED";

/**
 * One thing that happened during a run.
 *
 * Every event carries the full state of its subject rather than a delta, so a
 * client that missed one is corrected by the next rather than left drifting.
 * `type` says whether the subject is the run or a node inside it.
 */
export type ExecutionEvent = {
  type: "execution" | "node";
  executionId: string;
  status: ExecutionStatus;
  nodeId?: string;
  output?: unknown;
  outputTruncated?: boolean;
  error?: string;
  at: string;
};

/**
 * Watch a run. Returns a function that closes the socket.
 *
 * **Connect before you fetch.** Nothing is replayed, so a run that finishes
 * between a fetch and this call would leave the page showing RUNNING with no
 * event left to correct it. Connect first and the fetch returns either a
 * finished run or a running one whose remaining events all arrive here.
 *
 * The token rides in the subprotocol because the WebSocket constructor cannot
 * set headers. Not a query parameter: that is the one place a bearer token must
 * not go, since URLs are what logs and proxies record.
 *
 * Reconnection is the caller's, and worth doing — the server drops a client
 * that stops reading, and closing is how it says so. Refetch on reconnect.
 */
export async function watchExecution(
  workspaceId: string,
  executionId: string,
  onEvent: (event: ExecutionEvent) => void,
): Promise<() => void> {
  const token = await getToken();
  if (!token) throw new Error("Not signed in");

  const url =
    API_URL.replace(/^http/, "ws") +
    `/api/workspaces/${workspaceId}/executions/${executionId}/events`;

  const socket = new WebSocket(url, ["bearer", token]);
  socket.onmessage = (message) => onEvent(JSON.parse(message.data) as ExecutionEvent);

  return () => socket.close();
}

/**
 * A run. The graph snapshot is deliberately absent from the list response — it
 * is the largest field by far, and a dashboard draws a row, not a canvas.
 */
export type Execution = {
  id: string;
  workflowId?: string;
  status: ExecutionStatus;
  trigger: string;
  error?: string;
  startedBy?: string;
  createdAt: string;
  startedAt?: string;
  finishedAt?: string;
  /** Computed server-side, so every client agrees on what a run took. */
  durationMs?: number;
};

export const executions = {
  /** Newest first. An empty workflowId means every run in the workspace. */
  list: (workspaceId: string, options: { workflowId?: string; limit?: number } = {}) =>
    api
      .get<{ executions: Execution[] }>(`/api/workspaces/${workspaceId}/executions`, {
        params: { workflowId: options.workflowId, limit: options.limit },
      })
      .then((r) => r.data.executions),

  start: (workspaceId: string, workflowId: string, input?: unknown) =>
    api
      .post<Execution>(
        `/api/workspaces/${workspaceId}/workflows/${workflowId}/executions`,
        input === undefined ? undefined : { input },
      )
      .then((r) => r.data),
};
