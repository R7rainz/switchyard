"use client";

import { Plus, Trash2 } from "lucide-react";
import { useState } from "react";

import { PageHeader } from "@/components/app-shell";
import { Modal } from "@/components/modal";
import {
  Badge,
  Button,
  Card,
  EmptyState,
  ErrorNote,
  Eyebrow,
  Field,
  Input,
  Skeleton,
  Textarea,
} from "@/components/ui";
import { apiError, type Workflow } from "@/lib/api";
import {
  emptyGraph,
  useCreateWorkflow,
  useDeleteWorkflow,
  useWorkflows,
  useWorkspace,
} from "@/lib/queries";

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
        <EmptyState
          title="No workflows yet"
          hint="A workflow is a graph of nodes: a trigger, some steps, and the edges between them. Start with an empty canvas, or describe one and let AI draft it."
          action={<Button onClick={() => setCreating(true)}>Create the first one</Button>}
        />
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
  const nodes = workflow.graph.nodes.length;

  return (
    <li>
      <Card className="group flex h-full min-h-40 flex-col gap-3">
        <div className="flex items-start justify-between gap-3">
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
          <p className="line-clamp-2 text-body-sm leading-relaxed text-ash">
            {workflow.description}
          </p>
        )}

        <div className="mt-auto flex items-center gap-2">
          <Badge tone={nodes > 0 ? "mint" : undefined}>
            {nodes} {nodes === 1 ? "node" : "nodes"}
          </Badge>
          <Eyebrow>{relativeTime(workflow.updatedAt)}</Eyebrow>
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
