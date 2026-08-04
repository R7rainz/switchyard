"use client";

import { Plus, Trash2 } from "lucide-react";
import { useState } from "react";

import { PageHeader } from "@/components/app-shell";
import { GraphPreview } from "@/components/graph-preview";
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
import {
  emptyGraph,
  useCreateWorkflow,
  useDeleteWorkflow,
  useWorkflows,
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
  const [creating, setCreating] = useState(false);

  return (
    <>
      <PageHeader
        eyebrow="Workspace"
        title="Workflows"
        actions={
          <Button onClick={() => setCreating(true)} disabled={!workspace}>
            <Plus size={16} strokeWidth={1.75} />
            New workflow
          </Button>
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
          <Button onClick={() => setCreating(true)}>Create the first one</Button>
        </div>
      )}

      <CreateWorkflowModal
        open={creating}
        onClose={() => setCreating(false)}
        workspaceId={workspace?.id}
      />
    </>
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
  const [confirming, setConfirming] = useState(false);

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
        <div className="rounded-t-xl bg-cream-wash px-4 py-4">
          <GraphPreview graph={workflow.graph} className="h-24 w-full" />
        </div>

        <div className="flex items-start justify-between gap-3 px-5">
          {/* Not a link yet: the page this opens is the builder, and a card
              that navigates to a 404 is worse than one that does not. */}
          <span className="text-body-lg text-ink">{workflow.name}</span>
          <button
            onClick={() => setConfirming(true)}
            aria-label={`Delete ${workflow.name}`}
            // Always visible where there is no hover. A control revealed only
            // on hover does not exist on a touch screen, and focus-visible
            // keeps it reachable by keyboard everywhere.
            className="-m-1 shrink-0 rounded-lg p-1 text-stone hover:text-phoenix-orange focus-visible:opacity-100 sm:opacity-0 sm:transition-opacity sm:group-hover:opacity-100"
          >
            <Trash2 size={16} strokeWidth={1.75} />
          </button>
        </div>

        {workflow.description && (
          <p className="line-clamp-2 px-5 text-body-sm leading-relaxed text-ash">
            {workflow.description}
          </p>
        )}

        <div className="mt-auto flex flex-wrap items-center gap-2 px-5 pb-5">
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

/**
 * Rendered in the browser from an absolute timestamp the server sent, so every
 * client agrees on the instant and disagrees only about the wording.
 */
function relativeTime(iso: string): string {
  const seconds = Math.round((Date.now() - new Date(iso).getTime()) / 1000);
  const steps: Array<[Intl.RelativeTimeFormatUnit, number]> = [
    ["second", 60],
    ["minute", 60],
    ["hour", 24],
    ["day", 7],
    ["week", 4.35],
    ["month", 12],
    ["year", Infinity],
  ];

  let value = seconds;
  for (const [unit, size] of steps) {
    if (Math.abs(value) < size) {
      return new Intl.RelativeTimeFormat(undefined, { numeric: "auto" }).format(
        -Math.round(value),
        unit,
      );
    }
    value /= size;
  }
  return "";
}
