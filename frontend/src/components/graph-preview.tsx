import type { Graph } from "@/lib/api";
import { paletteFor } from "@/lib/categories";

/**
 * A workflow drawn small.
 *
 * The point of a dashboard is recognising a workflow, and a name plus a node
 * count does not do that — two workflows called "deploy" look identical. Their
 * shapes do not. This renders the graph the user actually drew, at the
 * positions they actually dragged it to, so a card is a picture of the thing
 * rather than a row about it.
 */

const NODE_W = 26;
const NODE_H = 12;
const PAD = 10;

export function GraphPreview({ graph, className }: { graph: Graph; className?: string }) {
  if (graph.nodes.length === 0) return <EmptyCanvas className={className} />;

  // Fit whatever the canvas positions happen to be into the viewBox. A user
  // drags nodes anywhere, so nothing here can assume an origin or a scale.
  const xs = graph.nodes.map((node) => node.position.x);
  const ys = graph.nodes.map((node) => node.position.y);
  const minX = Math.min(...xs);
  const minY = Math.min(...ys);
  // A single node, or a straight row, has zero extent on one axis — guard the
  // divide rather than letting it produce Infinity and an empty picture.
  const spanX = Math.max(Math.max(...xs) - minX, 1);
  const spanY = Math.max(Math.max(...ys) - minY, 1);

  const width = 240;
  const height = 96;
  const usableX = width - NODE_W - PAD * 2;
  const usableY = height - NODE_H - PAD * 2;

  const place = (node: (typeof graph.nodes)[number]) => ({
    x: PAD + ((node.position.x - minX) / spanX) * usableX,
    // A graph laid out in a straight line should sit on the centre line rather
    // than pinned to the top by a degenerate span.
    y:
      graph.nodes.length === 1 || Math.max(...ys) === minY
        ? (height - NODE_H) / 2
        : PAD + ((node.position.y - minY) / spanY) * usableY,
  });

  const positions = new Map(graph.nodes.map((node) => [node.id, place(node)]));

  return (
    <svg
      viewBox={`0 0 ${width} ${height}`}
      className={className}
      role="img"
      aria-label={`${graph.nodes.length} nodes, ${graph.edges.length} connections`}
    >
      {graph.edges.map((edge) => {
        const from = positions.get(edge.source);
        const to = positions.get(edge.target);
        // An edge can outlive its node in a draft — the backend stores those,
        // deliberately, so the preview has to survive one.
        if (!from || !to) return null;

        const x1 = from.x + NODE_W;
        const y1 = from.y + NODE_H / 2;
        const x2 = to.x;
        const y2 = to.y + NODE_H / 2;
        // A horizontal-tangent bezier, the same shape React Flow draws, so the
        // preview reads as a small version of the canvas rather than a diagram
        // of it.
        const bend = Math.max(12, Math.abs(x2 - x1) / 2);

        return (
          <path
            key={edge.id}
            d={`M ${x1} ${y1} C ${x1 + bend} ${y1}, ${x2 - bend} ${y2}, ${x2} ${y2}`}
            fill="none"
            stroke="currentColor"
            strokeWidth={1.25}
            className="text-stone"
          />
        );
      })}

      {graph.nodes.map((node) => {
        const at = positions.get(node.id)!;
        return (
          <rect
            key={node.id}
            x={at.x}
            y={at.y}
            width={NODE_W}
            height={NODE_H}
            rx={3}
            fill={paletteFor(node.type).hex}
          />
        );
      })}
    </svg>
  );
}

/**
 * An empty workflow is a real state — a save is a draft, so one exists before
 * it has any nodes. It gets a drawn placeholder rather than the word "empty".
 */
function EmptyCanvas({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 240 96" className={className} role="img" aria-label="Empty canvas">
      <rect
        x={8}
        y={8}
        width={224}
        height={80}
        rx={8}
        fill="none"
        stroke="currentColor"
        strokeWidth={1.25}
        strokeDasharray="4 5"
        className="text-stone"
      />
    </svg>
  );
}
