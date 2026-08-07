"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";

import { PageHeader } from "@/components/app-shell";
import {
  Badge,
  Button,
  Card,
  ErrorNote,
  Eyebrow,
} from "@/components/ui";
import { apiError, githubWebhookURL, type Graph } from "@/lib/api";
import { useCreateWorkflow, useWorkspace } from "@/lib/queries";

const githubTestGraph: Graph = {
  nodes: [
    {
      id: "trigger",
      type: "trigger.github.pull_request",
      position: { x: 0, y: 80 },
      data: { label: "PR opened", action: "opened" },
    },
    {
      id: "fetch",
      type: "github.pull_request",
      position: { x: 280, y: 80 },
      data: {
        label: "Fetch PR details",
        owner: "{{ .trigger.repository.owner.login }}",
        repo: "{{ .trigger.repository.name }}",
        number: "{{ .trigger.number }}",
      },
    },
  ],
  edges: [{ id: "trigger-fetch", source: "trigger", target: "fetch" }],
};

export default function DocsPage() {
  const router = useRouter();
  const { workspace } = useWorkspace();
  const create = useCreateWorkflow(workspace?.id);

  function createTestWorkflow() {
    create.mutate(
      {
        name: "GitHub PR webhook test",
        description: "Fetches pull-request details when a GitHub PR is opened.",
        graph: githubTestGraph,
      },
      { onSuccess: (workflow) => router.push(`/workflows/${workflow.id}`) },
    );
  }

  return (
    <>
      <PageHeader eyebrow="Guide" title="GitHub pull-request workflows" />

      <div className="flex max-w-3xl flex-col gap-5">
        <Card>
          <Eyebrow>Quick start</Eyebrow>
          <h2 className="mt-3 text-subheading text-ink">Create a known-good test workflow</h2>
          <p className="mt-2 text-body-sm leading-relaxed text-ash">
            This creates a GitHub pull-request trigger with the correct payload references. Open
            the workflow after creation and use its Copy webhook URL button.
          </p>
          <div className="mt-4 flex flex-wrap items-center gap-3">
            <Button disabled={!workspace || create.isPending} onClick={createTestWorkflow}>
              {create.isPending ? "Creating…" : "Create test workflow"}
            </Button>
            <Badge tone="mint">Action: opened</Badge>
          </div>
          {create.error && <ErrorNote>{apiError(create.error)}</ErrorNote>}
        </Card>

        <Card>
          <Eyebrow>1 · Configure credentials</Eyebrow>
          <p className="mt-3 text-body-sm leading-relaxed text-ash">
            Store both credentials in the workspace. The webhook secret authenticates GitHub&apos;s
            delivery; the PAT lets the Fetch PR Details node call GitHub&apos;s API.
          </p>
          <div className="mt-4 grid gap-3 sm:grid-cols-2">
            <CredentialHint name="github/webhook" text="Shared signing secret" />
            <CredentialHint name="github/default" text="GitHub personal access token" />
          </div>
          <p className="mt-4 text-caption leading-relaxed text-ash">
            Add both from Settings → Credentials by choosing provider <code>github</code> and the
            matching name.
          </p>
        </Card>

        <Card>
          <Eyebrow>2 · Add the repository webhook</Eyebrow>
          <p className="mt-3 text-body-sm leading-relaxed text-ash">
            In GitHub, open Repository Settings → Webhooks → Add webhook. Use the Switchyard API
            URL shown below, not the browser app URL. The builder&apos;s Copy webhook URL button
            provides the workflow-specific value automatically.
          </p>
          <CodeBlock>{githubWebhookURL("WORKFLOW_ID")}</CodeBlock>
          <ol className="mt-4 list-decimal space-y-2 pl-5 text-body-sm text-ash">
            <li>Set content type to <code>application/json</code>.</li>
            <li>Use the same value saved as <code>github/webhook</code>.</li>
            <li>Keep SSL verification enabled.</li>
            <li>Select only <strong>Pull requests</strong>, then enable the webhook.</li>
          </ol>
        </Card>

        <Card>
          <Eyebrow>3 · Test an opened event</Eyebrow>
          <p className="mt-3 text-body-sm leading-relaxed text-ash">
            Do not press Run in the builder for this workflow: a manual run has no GitHub payload
            and <code>.trigger.number</code> will be missing. Create a new pull request instead,
            then check GitHub Webhooks → Recent deliveries.
          </p>
          <div className="mt-4 grid gap-2 text-body-sm text-ash">
            <StatusRow code="202" text="Accepted; a webhook run should appear in Runs." />
            <StatusRow code="204" text="Event or action did not match the trigger." />
            <StatusRow code="401" text="Webhook secret does not match." />
            <StatusRow code="404" text="Wrong IDs or the workflow has no GitHub trigger." />
            <StatusRow code="503" text="github/webhook is missing in this workspace." />
          </div>
        </Card>

        <p className="text-body-sm text-ash">
          Need to repair the existing workflow? Open it from <Link className="text-ink underline" href="/workflows">Workflows</Link>,
          replace the Manual trigger with GitHub pull request, set action to <code>opened</code>,
          and save before redelivering the event.
        </p>
      </div>
    </>
  );
}

function CredentialHint({ name, text }: { name: string; text: string }) {
  return (
    <div className="rounded-lg bg-pearl px-3 py-3">
      <code className="text-caption text-ink">{name}</code>
      <p className="mt-1 text-caption text-ash">{text}</p>
    </div>
  );
}

function CodeBlock({ children }: { children: string }) {
  return <code className="mt-4 block overflow-x-auto rounded-lg bg-ink px-4 py-3 text-caption text-canvas-white">{children}</code>;
}

function StatusRow({ code, text }: { code: string; text: string }) {
  return (
    <div className="flex items-baseline gap-3">
      <code className="w-10 shrink-0 text-ink">{code}</code>
      <span>{text}</span>
    </div>
  );
}
