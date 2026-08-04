"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";

import { GraphPreview } from "./graph-preview";
import { Modal } from "./modal";
import { Button, ErrorNote, Eyebrow, Field, Textarea } from "./ui";
import { apiError, type Generated } from "@/lib/api";
import { useCreateWorkflow, useGenerateWorkflow } from "@/lib/queries";

/**
 * Describe a workflow, look at what came back, then decide.
 *
 * Two steps on purpose. Generating stores nothing — the backend returns a graph
 * and creates no row — so this shows the proposal and waits. Saving is a
 * separate action the user takes after reading it, which is what "AI assists,
 * never owns" means in practice: a generate-and-save would put a workflow in
 * the list nobody has looked at.
 */
export function GenerateModal({
  open,
  onClose,
  workspaceId,
}: {
  open: boolean;
  onClose: () => void;
  workspaceId: string | undefined;
}) {
  const router = useRouter();
  const generate = useGenerateWorkflow(workspaceId);
  const create = useCreateWorkflow(workspaceId);

  const [prompt, setPrompt] = useState("");
  const [proposal, setProposal] = useState<Generated | null>(null);

  function close() {
    generate.reset();
    create.reset();
    setPrompt("");
    setProposal(null);
    onClose();
  }

  return (
    <Modal open={open} onClose={close} title={proposal ? "Review the draft" : "Describe a workflow"}>
      {proposal ? (
        <div className="flex flex-col gap-5">
          <div className="rounded-xl bg-cream-wash p-4">
            <GraphPreview graph={proposal.graph} className="h-28 w-full" />
          </div>

          <div className="flex flex-col gap-2">
            <span className="text-body-lg text-ink">{proposal.name}</span>
            {proposal.description && (
              <span className="text-body-sm leading-relaxed text-ash">{proposal.description}</span>
            )}
            <Eyebrow>
              {proposal.graph.nodes.length} nodes · {proposal.graph.edges.length} connections
            </Eyebrow>
          </div>

          <p className="text-caption leading-relaxed text-ash">
            Nothing has been saved. Open it to check every node before it runs — a generated graph
            is a draft, and the model does not know your endpoints or your keys.
          </p>

          {create.error && <ErrorNote>{apiError(create.error)}</ErrorNote>}

          <div className="flex flex-wrap justify-end gap-3">
            <Button variant="ghost" onClick={() => setProposal(null)}>
              Try again
            </Button>
            <Button
              disabled={create.isPending}
              onClick={() =>
                create.mutate(
                  {
                    name: proposal.name,
                    description: proposal.description,
                    graph: proposal.graph,
                  },
                  {
                    // Straight onto the canvas: the point is that it gets read
                    // and edited before anyone runs it.
                    onSuccess: (saved) => {
                      close();
                      router.push(`/workflows/${saved.id}`);
                    },
                  },
                )
              }
            >
              {create.isPending ? "Saving…" : "Save and open"}
            </Button>
          </div>
        </div>
      ) : (
        <form
          className="flex flex-col gap-5"
          onSubmit={(event) => {
            event.preventDefault();
            generate.mutate(prompt.trim(), { onSuccess: setProposal });
          }}
        >
          <Field
            label="What should it do?"
            hint="Name the trigger, the steps, and any branch. The model only knows the node types this engine can actually run."
          >
            <Textarea
              autoFocus
              rows={4}
              value={prompt}
              onChange={(event) => setPrompt(event.target.value)}
              placeholder="When a pull request is merged, fetch the diff, summarise it, and post to Slack — but page the on-call instead if it touches migrations."
            />
          </Field>

          {generate.error && <ErrorNote>{apiError(generate.error)}</ErrorNote>}

          <div className="flex justify-end gap-3">
            <Button type="button" variant="ghost" onClick={close}>
              Cancel
            </Button>
            <Button type="submit" disabled={generate.isPending || !prompt.trim()}>
              {generate.isPending ? "Drafting…" : "Draft it"}
            </Button>
          </div>
        </form>
      )}
    </Modal>
  );
}
