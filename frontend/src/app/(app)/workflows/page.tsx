"use client";

import Link from "next/link";
import { Copy, Play, Plus, Sparkles, Trash2 } from "lucide-react";
import { useState } from "react";

import { PageHeader } from "@/components/app-shell";
import { GraphPreview } from "@/components/graph-preview";
import { GenerateModal } from "@/components/generate-modal";
import { RecentRuns } from "@/components/recent-runs";
import { RunStatus } from "@/components/run-status";
import { Modal } from "@/components/modal";
import {
  Badge,
  Button,
  Card,
  ErrorNote,
  Eyebrow,
  Field,
  Input,
  Skeleton,
  Textarea,
} from "@/components/ui";
import { apiError, type Workflow } from "@/lib/api";
import { paletteFor } from "@/lib/categories";
import { relativeTime } from "@/lib/time";
import {
  emptyGraph,
  useCreateWorkflow,
  useDeleteWorkflow,
  useDuplicateWorkflow,
  useCreateWorkflowFromTemplate,
  useExecutions,
  useStartExecution,
  useWorkflows,
  useTemplates,
  useWorkspace,
} from "@/lib/queries";

/**
 * The shape drawn on the empty state. Not a saved workflow — it exists to show
 * what one looks like, using the same renderer a real one gets.
 */
const exampleGraph = {
  nodes: [
    { id: "t", type: "trigger.manual", position: { x: 0, y: 60 } },
    { id: "f", type: "http.request", position: { x: 120, y: 60 } },
    { id: "c", type: "logic.condition", position: { x: 240, y: 60 } },
    { id: "a", type: "ai.prompt", position: { x: 360, y: 0 } },
    { id: "s", type: "http.request", position: { x: 360, y: 120 } },
  ],
  edges: [
    { id: "e1", source: "t", target: "f" },
    { id: "e2", source: "f", target: "c" },
    { id: "e3", source: "c", target: "a", sourceHandle: "true" },
    { id: "e4", source: "c", target: "s", sourceHandle: "false" },
  ],
};

export default function WorkflowsPage() {
  const { workspace } = useWorkspace();
  const { data: flows, isPending, error } = useWorkflows(workspace?.id);
  const { data: templates } = useTemplates(workspace?.id);
  const [creating, setCreating] = useState(false);
  const [drafting, setDrafting] = useState(false);

  return (
    <>
      <PageHeader
        eyebrow="Workspace"
        title="Workflows"
        actions={
          <>
            <Button variant="neutral" onClick={() => setDrafting(true)} disabled={!workspace}>
              <Sparkles size={16} strokeWidth={1.75} />
              Draft with AI
            </Button>
            <Button onClick={() => setCreating(true)} disabled={!workspace}>
              <Plus size={16} strokeWidth={1.75} />
              New workflow
            </Button>
          </>
        }
      />

      {error && <ErrorNote>{apiError(error)}</ErrorNote>}

      {isPending || !workspace ? (
        <WorkflowListSkeleton />
      ) : flows && flows.length > 0 ? (
        <ul className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {flows.map((flow) => (
            <WorkflowCard key={flow.id} workflow={flow} workspaceId={workspace.id} />
          ))}
        </ul>
      ) : (
        <div className="flex flex-col items-center gap-6 rounded-xl bg-cream-wash px-6 py-16 text-center">
          {/* Drawn, not described. The shape of the thing is the fastest way to
              say what a workflow is. */}
          <GraphPreview graph={exampleGraph} className="h-28 w-full max-w-sm" />
          <div className="flex flex-col items-center gap-3">
            <p className="text-subheading text-ink">No workflows yet</p>
            <p className="max-w-md text-body-sm leading-relaxed text-ash">
              A workflow is a trigger, some steps, and the edges between them. Start with an empty
              canvas, or describe one and let AI draft it.
            </p>
          </div>
          <div className="flex flex-wrap justify-center gap-3">
            <Button onClick={() => setDrafting(true)}>
              <Sparkles size={16} strokeWidth={1.75} />
              Draft one with AI
            </Button>
            <Button variant="neutral" onClick={() => setCreating(true)}>
              Start from an empty canvas
            </Button>
          </div>
        </div>
      )}

      {templates && templates.length > 0 && workspace && (
        <TemplateLibrary templates={templates} workspaceId={workspace.id} />
      )}

      <RecentRuns workspaceId={workspace?.id} />

      <GenerateModal
        open={drafting}
        onClose={() => setDrafting(false)}
        workspaceId={workspace?.id}
      />

      <CreateWorkflowModal
        open={creating}
        onClose={() => setCreating(false)}
        workspaceId={workspace?.id}
      />
    </>
  );
}

function TemplateLibrary({
  templates,
  workspaceId,
}: {
  templates: import("@/lib/api").WorkflowTemplate[];
  workspaceId: string;
}) {
  const create = useCreateWorkflowFromTemplate(workspaceId);
  return (
    <section className="mt-8">
      <div className="mb-3 flex items-baseline justify-between">
        <h2 className="text-subheading text-ink">Templates</h2>
        <span className="text-caption text-ash">Reusable workflow starters</span>
      </div>
      <ul className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {templates.map((template) => (
          <li key={template.id}>
            <Card className="flex h-full flex-col gap-3 p-4">
              <div>
                <p className="text-body-sm text-ink">{template.name}</p>
                {template.description && <p className="mt-1 text-caption text-ash">{template.description}</p>}
              </div>
              <Button
                variant="neutral"
                className="mt-auto h-8 self-start px-3"
                disabled={create.isPending}
                onClick={() => create.mutate({ templateId: template.id })}
              >
                {create.isPending ? "Creating…" : "Use template"}
              </Button>
            </Card>
          </li>
        ))}
      </ul>
    </section>
  );
}

/**
 * A card per workflow rather than a table row.
 *
 * The list is short and each item is a thing you open, not a record you scan a
 * column of — and a card grid reflows on a phone without the fixed-width
 * columns that made the previous table unreadable there.
 */
function WorkflowCard({ workflow, workspaceId }: { workflow: Workflow; workspaceId: string }) {
  const remove = useDeleteWorkflow(workspaceId);
  const duplicate = useDuplicateWorkflow(workspaceId);
  const start = useStartExecution(workspaceId);
  const [confirming, setConfirming] = useState(false);

  // This workflow's own runs, out of the same list the strip below shows, so
  // the card and the strip cannot disagree and there is no second request.
  const { data: allRuns } = useExecutions(workspaceId);
  const lastRun = allRuns?.find((run) => run.workflowId === workflow.id);
  const runnable = workflow.graph.nodes.length > 0;
  const githubTriggered = workflow.graph.nodes.some((node) => node.type === "trigger.github.pull_request");

  // Which node families this workflow uses, in first-seen order, deduped.
  const families = [...new Map(
    workflow.graph.nodes.map((node) => {
      const palette = paletteFor(node.type);
      return [palette.label, palette];
    }),
  ).values()];

  return (
    <li>
      <Card className="group flex h-full flex-col gap-4 p-0">
        {/* The graph, not a node count. Two workflows called "deploy" read
            identically by name and differently by shape, and recognising one
            is the entire job of this screen. */}
        <Link
          href={`/workflows/${workflow.id}`}
          aria-label={`Open ${workflow.name}`}
          className="block rounded-t-xl bg-cream-wash px-4 py-4 hover:bg-pearl"
        >
          <GraphPreview graph={workflow.graph} className="h-24 w-full" />
        </Link>

        <div className="flex items-start justify-between gap-3 px-5">
          <Link href={`/workflows/${workflow.id}`} className="text-body-lg text-ink hover:underline">
            {workflow.name}
          </Link>
          <div className="flex shrink-0 items-center gap-1">
            <button
              onClick={() => duplicate.mutate(workflow.id)}
              aria-label={`Duplicate ${workflow.name}`}
              title="Duplicate workflow"
              disabled={duplicate.isPending}
              className="-m-1 rounded-lg p-1 text-stone hover:text-ink focus-visible:opacity-100 sm:opacity-0 sm:transition-opacity sm:group-hover:opacity-100"
            >
              <Copy size={16} strokeWidth={1.75} />
            </button>
            <button
              onClick={() => setConfirming(true)}
              aria-label={`Delete ${workflow.name}`}
              // Always visible where there is no hover. A control revealed only
              // on hover does not exist on a touch screen, and focus-visible
              // keeps it reachable by keyboard everywhere.
              className="-m-1 rounded-lg p-1 text-stone hover:text-phoenix-orange focus-visible:opacity-100 sm:opacity-0 sm:transition-opacity sm:group-hover:opacity-100"
            >
              <Trash2 size={16} strokeWidth={1.75} />
            </button>
          </div>
        </div>

        {workflow.description && (
          <p className="line-clamp-2 px-5 text-body-sm leading-relaxed text-ash">
            {workflow.description}
          </p>
        )}

        <div className="flex flex-wrap items-center gap-2 px-5">
          {/* One badge per node family present, in the colour that family
              wears everywhere else. An empty draft says so plainly. */}
          {families.length > 0 ? (
            families.map((family) => (
              <Badge key={family.label} tone={family.tone}>
                {family.label}
              </Badge>
            ))
          ) : (
            <Badge>Empty draft</Badge>
          )}
          <Eyebrow className="ml-auto">{relativeTime(workflow.updatedAt)}</Eyebrow>
        </div>

        <div className="mt-auto flex items-center gap-3 border-t border-hairline px-5 py-3">
          {lastRun ? (
            <RunStatus status={lastRun.status} />
          ) : (
            <span className="text-caption text-ash">Never run</span>
          )}

          <Button
            variant="neutral"
            className="ml-auto h-8 px-3"
            // GitHub-triggered graphs need the signed payload that only GitHub
            // can provide; a manual start would fail every .trigger reference.
            title={
              githubTriggered
                ? "This workflow runs from a GitHub pull-request webhook"
                : runnable
                  ? undefined
                  : "Add a trigger before running this"
            }
            disabled={!runnable || githubTriggered || start.isPending}
            onClick={() => start.mutate(workflow.id)}
          >
            <Play size={13} strokeWidth={2} />
            {start.isPending ? "Starting…" : "Run"}
          </Button>
        </div>
      </Card>

      <Modal open={confirming} onClose={() => setConfirming(false)} title="Delete workflow">
        <p className="text-body-sm leading-relaxed text-ash">
          <span className="text-ink">{workflow.name}</span> and its graph will be removed. Past runs
          keep the copy of the graph they executed, so this does not change what already happened.
        </p>
        {remove.error && <ErrorNote>{apiError(remove.error)}</ErrorNote>}
        <div className="flex justify-end gap-3">
          <Button variant="ghost" onClick={() => setConfirming(false)}>
            Cancel
          </Button>
          <Button
            variant="danger"
            disabled={remove.isPending}
            onClick={() => remove.mutate(workflow.id, { onSuccess: () => setConfirming(false) })}
          >
            {remove.isPending ? "Deleting…" : "Delete"}
          </Button>
        </div>
      </Modal>
    </li>
  );
}

function CreateWorkflowModal({
  open,
  onClose,
  workspaceId,
}: {
  open: boolean;
  onClose: () => void;
  workspaceId: string | undefined;
}) {
  const create = useCreateWorkflow(workspaceId);
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");

  function close() {
    create.reset();
    setName("");
    setDescription("");
    onClose();
  }

  return (
    <Modal open={open} onClose={close} title="New workflow">
      <form
        className="flex flex-col gap-5"
        onSubmit={(event) => {
          event.preventDefault();
          // An empty canvas is a valid save: the backend stores drafts on
          // purpose, so a workflow can exist before it can run.
          create.mutate(
            { name: name.trim(), description: description.trim(), graph: emptyGraph },
            { onSuccess: close },
          );
        }}
      >
        <Field label="Name">
          <Input
            autoFocus
            required
            value={name}
            onChange={(event) => setName(event.target.value)}
            placeholder="Deploy on merge"
          />
        </Field>

        <Field label="Description">
          <Textarea
            rows={3}
            value={description}
            onChange={(event) => setDescription(event.target.value)}
            placeholder="What this workflow is for"
          />
        </Field>

        {create.error && <ErrorNote>{apiError(create.error)}</ErrorNote>}

        <div className="flex justify-end gap-3">
          <Button type="button" variant="ghost" onClick={close}>
            Cancel
          </Button>
          <Button type="submit" disabled={create.isPending || !name.trim()}>
            {create.isPending ? "Creating…" : "Create"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}

function WorkflowListSkeleton() {
  return (
    <ul className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
      {[0, 1, 2].map((card) => (
        <li key={card}>
          <Card className="flex h-40 flex-col gap-3">
            <Skeleton className="h-5 w-40" />
            <Skeleton className="h-4 w-full" />
            <Skeleton className="mt-auto h-6 w-24" />
          </Card>
        </li>
      ))}
    </ul>
  );
}
