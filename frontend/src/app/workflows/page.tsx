"use client";

import { Plus, Trash2 } from "lucide-react";
import { useState } from "react";

import { PageHeader } from "@/components/app-shell";
import { Modal } from "@/components/modal";
import {
  Button,
  EmptyState,
  ErrorNote,
  Field,
  Input,
  Mono,
  Skeleton,
  Textarea,
} from "@/components/ui";
import { apiError, type Workflow } from "@/lib/api";
import { emptyGraph, useCreateWorkflow, useDeleteWorkflow, useWorkflows, useWorkspace } from "@/lib/queries";

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
            <Plus size={14} strokeWidth={1.5} />
            New workflow
          </Button>
        }
      />

      {error && <ErrorNote>{apiError(error)}</ErrorNote>}

      {isPending || !workspace ? (
        <WorkflowListSkeleton />
      ) : flows && flows.length > 0 ? (
        <WorkflowTable workflows={flows} workspaceId={workspace.id} />
      ) : (
        <EmptyState
          title="No workflows yet"
          hint="A workflow is a graph of nodes: a trigger, some steps, and the edges between them. Start with an empty canvas or describe one and let AI draft it."
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
 * The list. Columns are mono uppercase — a header is a system surface, not
 * copy — and the rows are separated by hairlines rather than cards, because a
 * card per row would make ten workflows look like ten decisions.
 */
function WorkflowTable({
  workflows,
  workspaceId,
}: {
  workflows: Workflow[];
  workspaceId: string;
}) {
  return (
    <div className="rounded-lg border border-carbon-lift">
      {/* Column headers only exist where there are columns. Below sm the row
          stacks, and a header for a layout that is not a table is noise. */}
      <div className="hidden grid-cols-[1fr_140px_100px] items-center gap-4 border-b border-carbon-lift px-5 py-3 sm:grid">
        <Mono>Name</Mono>
        <Mono>Updated</Mono>
        <Mono className="text-right">Nodes</Mono>
      </div>

      <ul>
        {workflows.map((flow) => (
          <WorkflowRow key={flow.id} workflow={flow} workspaceId={workspaceId} />
        ))}
      </ul>
    </div>
  );
}

function WorkflowRow({ workflow, workspaceId }: { workflow: Workflow; workspaceId: string }) {
  const remove = useDeleteWorkflow(workspaceId);
  const [confirming, setConfirming] = useState(false);

  return (
    <li className="group flex flex-col gap-2 border-b border-carbon-lift px-5 py-4 last:border-b-0 hover:bg-carbon-lift/40 sm:grid sm:grid-cols-[1fr_140px_100px] sm:items-center sm:gap-4 max-sm:relative">
      <div className="flex min-w-0 flex-col gap-1">
        {/* Not a link yet: the builder is the page this opens, and a row that
            navigates to a 404 is worse than one that does not navigate. */}
        <span className="truncate text-body-sm text-bone">{workflow.name}</span>
        {workflow.description && (
          <span className="truncate text-body-sm text-warm-granite">{workflow.description}</span>
        )}
      </div>

      {/* Below sm the two right-hand columns become one mono line under the
          name. Squeezing them into fixed widths on a phone left the name
          rendering as a single letter. */}
      <span className="text-body-sm text-warm-granite max-sm:font-mono max-sm:text-caption max-sm:uppercase">
        {relativeTime(workflow.updatedAt)}
        <span className="sm:hidden"> · {workflow.graph.nodes.length} nodes</span>
      </span>

      <div className="flex items-center justify-end gap-3 max-sm:absolute max-sm:top-4 max-sm:right-5">
        <span className="hidden font-mono text-caption text-pale-stone sm:inline">
          {workflow.graph.nodes.length}
        </span>
        <button
          onClick={() => setConfirming(true)}
          aria-label={`Delete ${workflow.name}`}
          // Always visible where there is no hover. A control revealed only on
          // hover does not exist on a touch screen, and only-on-hover is not
          // reachable by keyboard either, hence focus-visible.
          className="text-graphite-mid hover:text-signal-orange focus-visible:opacity-100 sm:opacity-0 sm:transition-opacity sm:group-hover:opacity-100"
        >
          <Trash2 size={14} strokeWidth={1.5} />
        </button>
      </div>

      <Modal open={confirming} onClose={() => setConfirming(false)} title="Delete workflow">
        <p className="text-body-sm text-warm-granite">
          <span className="text-bone">{workflow.name}</span> and its graph will be removed. Past
          runs keep the copy of the graph they executed, so this does not change what already
          happened.
        </p>
        {remove.error && <ErrorNote>{apiError(remove.error)}</ErrorNote>}
        <div className="flex justify-end gap-3">
          <Button variant="ghost" onClick={() => setConfirming(false)}>
            Cancel
          </Button>
          <Button
            variant="danger"
            disabled={remove.isPending}
            onClick={() =>
              remove.mutate(workflow.id, { onSuccess: () => setConfirming(false) })
            }
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
        className="flex flex-col gap-6"
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
    <div className="flex flex-col gap-px rounded-lg border border-carbon-lift p-5">
      {[0, 1, 2].map((row) => (
        <div key={row} className="flex items-center justify-between py-4">
          <Skeleton className="h-4 w-56" />
          <Skeleton className="h-3 w-20" />
        </div>
      ))}
    </div>
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
      return new Intl.RelativeTimeFormat(undefined, { numeric: "auto" }).format(-Math.round(value), unit);
    }
    value /= size;
  }
  return "";
}
