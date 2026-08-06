/**
 * What a node type is called and what it needs configured.
 *
 * This is the third place node types are listed — the engine's registry and the
 * AI system prompt are the other two — and it exists because only this one
 * knows what a *form* for each type looks like. Adding a node type means
 * touching all three: a runner in Go, a line in the prompt, and an entry here.
 * The engine is the authority; a type listed here with no runner saves fine and
 * fails its first run.
 */

import { AI_PROVIDERS } from "./api";

export type FieldKind = "text" | "textarea" | "select" | "json";

export type NodeField = {
  key: string;
  label: string;
  kind: FieldKind;
  placeholder?: string;
  options?: string[];
  hint?: string;
};

export type NodeSpec = {
  type: string;
  label: string;
  summary: string;
  fields: NodeField[];
};

/** The reference every field hint points at, written once. */
const templateHint =
  "Reference an earlier node with {{ .nodes.<id>.<field> }}, or the payload with {{ .trigger.<field> }}. Wrap a value that may contain quotes as {{ json . }}.";

const aiProviderField: NodeField = {
  key: "provider",
  label: "Provider",
  kind: "select",
  options: [...AI_PROVIDERS],
  hint: "Leave as Default to use OpenRouter, or store the matching provider/default key in workspace settings.",
};

export const nodeSpecs: NodeSpec[] = [
  {
    type: "trigger.manual",
    label: "Manual",
    summary: "Started by a person. A workflow needs exactly one trigger.",
    fields: [],
  },
  {
    type: "trigger.webhook",
    label: "Webhook",
    summary: "Started by an inbound call. Its body becomes the run's payload.",
    fields: [],
  },
  {
    type: "trigger.github.pull_request",
    label: "GitHub pull request",
    summary: "Starts when GitHub sends the selected pull-request event.",
    fields: [{ key: "action", label: "Action", kind: "select", options: ["opened", "reopened", "synchronize"] }],
  },
  {
    type: "trigger.schedule",
    label: "Schedule",
    summary: "Started on a schedule.",
    fields: [{ key: "cron", label: "Cron", kind: "text", placeholder: "0 9 * * 1-5" }],
  },
  {
    type: "logic.condition",
    label: "Condition",
    summary: "Sends the run down one of two paths.",
    fields: [
      {
        key: "value",
        label: "Value",
        kind: "text",
        placeholder: '{{ if eq .trigger.branch "main" }}true{{ else }}false{{ end }}',
        hint: `Renders to true or false, then leaves by the matching handle. ${templateHint}`,
      },
    ],
  },
  {
    type: "logic.switch",
    label: "Switch",
    summary: "Routes a value to a matching case or the default path.",
    fields: [
      {
        key: "value",
        label: "Value",
        kind: "text",
        placeholder: "{{ .trigger.action }}",
        hint: templateHint,
      },
      {
        key: "cases",
        label: "Cases",
        kind: "json",
        placeholder: '["opened", "closed"]',
        hint: "Use a JSON array of unique strings. Connect the default handle for unmatched values.",
      },
    ],
  },
  {
    type: "logic.delay",
    label: "Delay",
    summary: "Waits before continuing to the next node.",
    fields: [
      {
        key: "duration",
        label: "Duration",
        kind: "text",
        placeholder: "5s",
        hint: "Use Go duration syntax such as 500ms, 5s, or 1m.",
      },
    ],
  },
  {
    type: "variable.set",
    label: "Set variables",
    summary: "Names values so later nodes can read them.",
    fields: [
      {
        key: "values",
        label: "Values",
        kind: "json",
        placeholder: '{\n  "repo": "{{ .trigger.repo }}"\n}',
        hint: `An object of name/value pairs. Later nodes read them as {{ .nodes.<id>.<name> }}. ${templateHint}`,
      },
    ],
  },
  {
    type: "http.request",
    label: "HTTP request",
    summary: "Calls an endpoint. Outputs status and body.",
    fields: [
      {
        key: "method",
        label: "Method",
        kind: "select",
        options: ["GET", "POST", "PUT", "PATCH", "DELETE"],
      },
      { key: "url", label: "URL", kind: "text", placeholder: "https://api.example.com/repos" },
      { key: "headers", label: "Headers", kind: "json", placeholder: '{\n  "Accept": "application/json"\n}' },
      {
        key: "body",
        label: "Body",
        kind: "json",
        placeholder: '{\n  "text": "{{ json .nodes.fetch.body.name }}"\n}',
        hint: templateHint,
      },
    ],
  },
  {
    type: "ai.prompt",
    label: "AI prompt",
    summary: "Asks a model. Outputs its text.",
    fields: [
      aiProviderField,
      {
        key: "prompt",
        label: "Prompt",
        kind: "textarea",
        placeholder: "Summarise {{ .nodes.fetch.body.description }} in one line.",
        hint: templateHint,
      },
      { key: "system", label: "System", kind: "textarea", placeholder: "Be brief and factual." },
      {
        key: "model",
        label: "Model",
        kind: "text",
        placeholder: "anthropic/claude-sonnet-4.5",
        hint: "Leave empty to use the workspace default.",
      },
    ],
  },
  {
    type: "ai.chat",
    label: "Chat",
    summary: "Has a conversational model answer a prompt.",
    fields: [
      aiProviderField,
      { key: "prompt", label: "Prompt", kind: "textarea", placeholder: "Explain the deployment result." },
      { key: "system", label: "System", kind: "textarea", placeholder: "Be concise and factual." },
      { key: "model", label: "Model", kind: "text", placeholder: "anthropic/claude-sonnet-4.5" },
    ],
  },
  {
    type: "ai.summarize",
    label: "Summarize",
    summary: "Turns a longer input into a concise summary.",
    fields: [
      aiProviderField,
      { key: "text", label: "Text", kind: "textarea", placeholder: "{{ .nodes.fetch.body.description }}", hint: templateHint },
      { key: "instructions", label: "Instructions", kind: "textarea", placeholder: "Keep it to three bullets." },
      { key: "model", label: "Model", kind: "text", placeholder: "anthropic/claude-sonnet-4.5" },
    ],
  },
  {
    type: "ai.classification",
    label: "Classification",
    summary: "Assigns one label to text using a fixed set of choices.",
    fields: [
      aiProviderField,
      { key: "text", label: "Text", kind: "textarea", placeholder: "{{ .trigger.title }}", hint: templateHint },
      { key: "labels", label: "Labels", kind: "json", placeholder: '["bug", "feature", "question"]' },
      { key: "model", label: "Model", kind: "text", placeholder: "anthropic/claude-sonnet-4.5" },
    ],
  },
  {
    type: "ai.decision",
    label: "Decision",
    summary: "Answers true or false and routes the workflow accordingly.",
    fields: [
      aiProviderField,
      { key: "question", label: "Question", kind: "textarea", placeholder: "Is this ready to deploy?" },
      { key: "context", label: "Context", kind: "textarea", placeholder: "{{ .nodes.tests.body }}", hint: templateHint },
      { key: "system", label: "System", kind: "textarea", placeholder: "Decide conservatively." },
      { key: "model", label: "Model", kind: "text", placeholder: "anthropic/claude-sonnet-4.5" },
    ],
  },
  {
    type: "github.pull_request",
    label: "Get pull request",
    summary: "Reads pull-request details using the workspace GitHub token.",
    fields: [
      { key: "owner", label: "Owner", kind: "text", placeholder: "{{ .trigger.repository.owner.login }}", hint: templateHint },
      { key: "repo", label: "Repository", kind: "text", placeholder: "{{ .trigger.repository.name }}", hint: templateHint },
      { key: "number", label: "Pull request", kind: "text", placeholder: "{{ .trigger.number }}", hint: templateHint },
    ],
  },
  {
    type: "slack.message",
    label: "Send Slack message",
    summary: "Posts a message through the workspace incoming webhook.",
    fields: [{ key: "text", label: "Message", kind: "textarea", placeholder: "PR #{{ .nodes.pr.number }}: {{ .nodes.summary.text }}", hint: templateHint }],
  },
];

export const specFor = (type: string) => nodeSpecs.find((spec) => spec.type === type);

/**
 * A condition leaves by a named handle, and the name is what the engine matches
 * an edge's sourceHandle against. These two strings are a contract with Go.
 */
export const conditionHandles = ["true", "false"] as const;

export function switchHandles(data?: Record<string, unknown>) {
  let raw = data?.cases;
  if (typeof raw === "string") {
    try {
      raw = JSON.parse(raw);
    } catch {
      raw = [];
    }
  }
  const cases = Array.isArray(raw)
    ? raw.filter((value): value is string => typeof value === "string" && value.trim() !== "" && value !== "default")
    : [];
  return [...new Set([...cases, "default"])] as string[];
}
