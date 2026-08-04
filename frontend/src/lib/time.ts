/**
 * A timestamp as "3 minutes ago".
 *
 * Rendered in the browser from the absolute instant the server sent, so every
 * client agrees on when something happened and disagrees only about the
 * wording — a server-rendered relative time would be wrong the moment it was
 * cached, and wrong by a timezone for anyone not sitting on the server.
 */
export function relativeTime(iso: string): string {
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
