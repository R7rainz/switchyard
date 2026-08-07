import type { Pastel } from "@/components/ui";

/**
 * A node family and the colour it wears.
 *
 * DESIGN.md is explicit that the pastels are a taxonomy rather than
 * decoration, so this is the one table that assigns them — the colour a
 * capability wears on the landing page is the colour its node wears on a graph
 * preview, and will be the colour it wears on the canvas.
 *
 * The category is the part of a node type before the dot, which is also the
 * only part the engine validates at save time.
 */
export const categories = {
  trigger: { label: "Trigger", tone: "canary", hex: "#ffef99" },
  logic: { label: "Logic", tone: "mint", hex: "#b7efb2" },
  ai: { label: "AI", tone: "violet", hex: "#e2ddfd" },
  http: { label: "HTTP", tone: "pink", hex: "#ffd7f0" },
  variable: { label: "Variables", tone: "aqua", hex: "#99fff9" },
  github: { label: "GitHub", tone: "violet", hex: "#e2ddfd" },
  slack: { label: "Slack", tone: "pink", hex: "#ffd7f0" },
  discord: { label: "Discord", tone: "violet", hex: "#e2ddfd" },
  email: { label: "Email", tone: "pink", hex: "#ffd7f0" },
} as const satisfies Record<string, { label: string; tone: Pastel; hex: string }>;

export type Category = keyof typeof categories;

export const categoryOf = (nodeType: string) => nodeType.split(".")[0] ?? "";

/**
 * A type the frontend has never heard of still has to draw. The engine ships
 * runners independently of this table, so an unknown category is a gap in the
 * table rather than a broken workflow.
 */
export function paletteFor(nodeType: string) {
  const category = categoryOf(nodeType);
  return category in categories
    ? categories[category as Category]
    : { label: category || "Node", tone: "pink" as Pastel, hex: "#ecebea" };
}
