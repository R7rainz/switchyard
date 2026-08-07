import axios from "axios";

import { getToken } from "./auth-client";

/**
 * Base URL of the Go API server.
 *
 * NEXT_PUBLIC_ because the browser calls the backend directly with a minted
 * JWT — nothing about this request goes through Next.
 */
export const API_URL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8090";
export const API_PATH = "/api/v1";

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
  list: () => api.get<{ workspaces: Workspace[] }>(`${API_PATH}/workspaces`).then((r) => r.data.workspaces),
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

export type WorkflowVersion = {
  id: string;
  number: number;
  name: string;
  description: string;
  graph: Graph;
  createdBy?: string;
  createdAt: string;
};

export type WorkflowTemplate = {
  id: string;
  name: string;
  description: string;
  graph: Graph;
  createdBy?: string;
  createdAt: string;
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
      .get<{ workflows: Workflow[] }>(`${API_PATH}/workspaces/${workspaceId}/workflows`)
      .then((r) => r.data.workflows),

  get: (workspaceId: string, id: string) =>
    api.get<Workflow>(`${API_PATH}/workspaces/${workspaceId}/workflows/${id}`).then((r) => r.data),

  create: (workspaceId: string, body: { name: string; description?: string; graph: Graph }) =>
    api.post<Workflow>(`${API_PATH}/workspaces/${workspaceId}/workflows`, body).then((r) => r.data),

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
      .patch<Workflow>(`${API_PATH}/workspaces/${workspaceId}/workflows/${id}`, patch)
      .then((r) => r.data),

  remove: (workspaceId: string, id: string) =>
    api.delete(`${API_PATH}/workspaces/${workspaceId}/workflows/${id}`).then(() => undefined),

  duplicate: (workspaceId: string, id: string) =>
    api.post<Workflow>(`${API_PATH}/workspaces/${workspaceId}/workflows/${id}/duplicate`).then((r) => r.data),

  versions: (workspaceId: string, id: string) =>
    api
      .get<{ versions: WorkflowVersion[] }>(`${API_PATH}/workspaces/${workspaceId}/workflows/${id}/versions`)
      .then((r) => r.data.versions),

  restore: (workspaceId: string, id: string, version: number) =>
    api
      .post<Workflow>(`${API_PATH}/workspaces/${workspaceId}/workflows/${id}/versions/${version}/restore`)
      .then((r) => r.data),

  templates: (workspaceId: string) =>
    api
      .get<{ templates: WorkflowTemplate[] }>(`${API_PATH}/workspaces/${workspaceId}/templates`)
      .then((r) => r.data.templates),

  createTemplate: (workspaceId: string, body: { name: string; description?: string; graph: Graph }) =>
    api.post<WorkflowTemplate>(`${API_PATH}/workspaces/${workspaceId}/templates`, body).then((r) => r.data),

  createFromTemplate: (workspaceId: string, templateId: string, body?: { name?: string; description?: string }) =>
    api
      .post<Workflow>(`${API_PATH}/workspaces/${workspaceId}/templates/${templateId}/workflows`, body ?? {})
      .then((r) => r.data),

  /**
   * Ask a model for a workflow. Nothing is stored.
   *
   * The graph comes back for the canvas to open, and saving it is a separate
   * `create` the user triggers after looking at it — AI assists, it does not
   * own. Put the result straight into `useNodesState`/`useEdgesState`; it is
   * already in React Flow's shape.
   */
  generate: (workspaceId: string, prompt: string, provider?: string) =>
    api
      .post<Generated>(`${API_PATH}/workspaces/${workspaceId}/workflows/generate`, { prompt, provider })
      .then((r) => r.data),

  feedback: (
    workspaceId: string,
    body: {
      consent: true;
      prompt: string;
      outcome: "accepted" | "rejected";
      generated: Generated;
      finalGraph?: Graph;
    },
  ) => api.post(`${API_PATH}/workspaces/${workspaceId}/ai/feedback`, body).then(() => undefined),
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
  signal?: AbortSignal,
): Promise<() => void> {
  const token = await getToken();
  if (!token) throw new Error("Not signed in");
  if (signal?.aborted) throw new Error("Run watch cancelled");

  const url =
    API_URL.replace(/^http/, "ws") +
    `${API_PATH}/workspaces/${workspaceId}/executions/${executionId}/events`;

  return new Promise((resolve, reject) => {
    const socket = new WebSocket(url, ["bearer", token]);
    let opened = false;
    const close = () => socket.close();
    const abort = () => {
      close();
      reject(new Error("Run watch cancelled"));
    };
    const clearAbort = () => signal?.removeEventListener("abort", abort);

    signal?.addEventListener("abort", abort, { once: true });
    socket.onmessage = (message) => onEvent(JSON.parse(message.data) as ExecutionEvent);
    socket.onopen = () => {
      opened = true;
      clearAbort();
      resolve(close);
    };
    socket.onerror = () => {
      clearAbort();
      reject(new Error("Could not connect to run events"));
    };
    socket.onclose = () => {
      if (!opened) {
        clearAbort();
        reject(new Error("Could not connect to run events"));
      }
    };
  });
}

/**
 * A run. The graph snapshot is deliberately absent from the list response — it
 * is the largest field by far, and a dashboard draws a row, not a canvas.
 */
export type Execution = {
  id: string;
  workflowId?: string;
  retryOf?: string;
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

/** What one node did during one run. */
export type NodeRun = {
  nodeId: string;
  status: ExecutionStatus;
  output?: unknown;
  error?: string;
  startedAt?: string;
  finishedAt?: string;
  durationMs?: number;
};

/** One run, whole: the row, the graph it actually ran, and what each node did. */
export type ExecutionDetail = {
  execution: Execution;
  graph: Graph;
  nodes: NodeRun[];
};

/** The three statuses a run stops at. A node can also be SKIPPED. */
export const isTerminal = (status: ExecutionStatus | null | undefined) =>
  status === "SUCCEEDED" || status === "FAILED" || status === "CANCELLED";

export const executions = {
  /** Newest first. An empty workflowId means every run in the workspace. */
  list: (workspaceId: string, options: { workflowId?: string; limit?: number } = {}) =>
    api
      .get<{ executions: Execution[] }>(`${API_PATH}/workspaces/${workspaceId}/executions`, {
        params: { workflowId: options.workflowId, limit: options.limit },
      })
      .then((r) => r.data.executions),

  /**
   * One run with the graph it executed and what each node did.
   *
   * Fetched *after* the socket is open, never before: nothing is replayed, so a
   * run that finishes in the gap would leave the canvas showing RUNNING with no
   * event left to correct it.
   */
  get: (workspaceId: string, id: string) =>
    api
      .get<ExecutionDetail>(`${API_PATH}/workspaces/${workspaceId}/executions/${id}`)
      .then((r) => r.data),

  start: (workspaceId: string, workflowId: string, input?: unknown, idempotencyKey = crypto.randomUUID()) =>
    api
      .post<Execution>(
        `${API_PATH}/workspaces/${workspaceId}/workflows/${workflowId}/executions`,
        input === undefined ? undefined : { input },
        { headers: { "Idempotency-Key": idempotencyKey } },
      )
      .then((r) => r.data),

  retry: (workspaceId: string, executionId: string, idempotencyKey = crypto.randomUUID()) =>
    api
      .post<Execution>(
        `${API_PATH}/workspaces/${workspaceId}/executions/${executionId}/retry`,
        undefined,
        { headers: { "Idempotency-Key": idempotencyKey } },
      )
      .then((r) => r.data),
};

/** Cancel reaches only runs this process is executing; a finished run 409s. */
export const cancelExecution = (workspaceId: string, executionId: string) =>
  api
    .post(`${API_PATH}/workspaces/${workspaceId}/executions/${executionId}/cancel`)
    .then(() => undefined);

/**
 * The four roles, strictly ordered. The backend's permission table is the
 * authority — this list exists so a picker offers them in that order.
 */
export const roles = ["VIEWER", "MEMBER", "ADMIN", "OWNER"] as const;
export type Role = (typeof roles)[number];

export type Member = { userId: string; role: Role; createdAt: string };

export type Invite = {
  id: string;
  role: Role;
  email: string;
  link: boolean;
  maxUses: number;
  useCount: number;
  invitedBy: string;
  createdAt: string;
  expiresAt?: string;
};

export const members = {
  list: (workspaceId: string) =>
    api
      .get<{ members: Member[] }>(`${API_PATH}/workspaces/${workspaceId}/members`)
      .then((r) => r.data.members),

  setRole: (workspaceId: string, userId: string, role: Role) =>
    api.patch(`${API_PATH}/workspaces/${workspaceId}/members/${userId}`, { role }).then(() => undefined),

  remove: (workspaceId: string, userId: string) =>
    api.delete(`${API_PATH}/workspaces/${workspaceId}/members/${userId}`).then(() => undefined),
};

export const invites = {
  list: (workspaceId: string) =>
    api
      .get<{ invites: Invite[] }>(`${API_PATH}/workspaces/${workspaceId}/invites`)
      .then((r) => r.data.invites),

  /**
   * The token comes back exactly once. Only its hash is stored, so it cannot be
   * shown again — re-sharing a link means revoke and re-issue.
   */
  create: (workspaceId: string, body: { role: Role; email?: string; maxUses?: number }) =>
    api
      .post<{ invite: Invite; token: string; joinURL: string }>(
        `${API_PATH}/workspaces/${workspaceId}/invites`,
        body,
      )
      .then((r) => r.data),

  revoke: (workspaceId: string, inviteId: string) =>
    api.delete(`${API_PATH}/workspaces/${workspaceId}/invites/${inviteId}`).then(() => undefined),

  accept: (token: string) =>
    api.post<Workspace>(`${API_PATH}/invites/${token}/accept`).then((r) => r.data),
};

/**
 * A stored key, described but never returned.
 *
 * There is no endpoint that hands a secret back — deliberately. So this is
 * write, list-metadata, and delete; replacing a secret is how you rotate one.
 */
export type Credential = {
  provider: string;
  name: string;
  createdAt: string;
  updatedAt: string;
};

export const credentials = {
  list: (workspaceId: string) =>
    api
      .get<{ credentials: Credential[] }>(`${API_PATH}/workspaces/${workspaceId}/credentials`)
      .then((r) => r.data.credentials),

  /** Replaces whatever was held under the same provider and name. */
  put: (workspaceId: string, provider: string, name: string, secret: string) =>
    api
      .put(`${API_PATH}/workspaces/${workspaceId}/credentials/${provider}/${name}`, { secret })
      .then(() => undefined),

  remove: (workspaceId: string, provider: string, name: string) =>
    api
      .delete(`${API_PATH}/workspaces/${workspaceId}/credentials/${provider}/${name}`)
      .then(() => undefined),
};

/** Default credential slot used when no provider is selected. */
export const AI_CREDENTIAL = { provider: "openrouter", name: "default" } as const;
export const AI_PROVIDERS = ["openrouter", "openai", "anthropic", "gemini"] as const;
export type AIProvider = (typeof AI_PROVIDERS)[number];
