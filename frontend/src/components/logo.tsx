/**
 * The mark: a keycap seated on its switch.
 *
 * Switchyard is a railway term and the tagline is already "you throw the
 * switches", so the legend is the fork itself — one line in, two out, which is
 * both a rail switch and the branch every workflow here can take. The keycap
 * and the switch under it put that on a developer's desk.
 *
 * Isometric out of flat planes rather than gradients: DESIGN.md rules gradients
 * out everywhere but the hero and says flat colour is the elevation, so each
 * face is one tone and the form comes from the geometry.
 *
 * The housing is Midnight Indigo, off the document's own palette. It is the
 * dark counterweight to the bright accents, which is exactly what a switch
 * body under a white cap has to be — a pastel there would disappear.
 */
export function Logo({ size = 28, className }: { size?: number; className?: string }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 64 64"
      fill="none"
      className={className}
      role="img"
      aria-label="Switchyard"
    >
      {/* The switch, drawn first so the cap sits over it — the overlap is what
          makes the cap read as seated rather than perched on a pedestal. It
          stays a little narrower than the cap, because a keycap overhangs its
          housing and that overhang is most of what identifies the pair. */}
      <path d="M10 38 L32 50 L32 62 L10 50 Z" fill="#10054d" />
      <path d="M54 38 L32 50 L32 62 L54 50 Z" fill="#1d1063" />
      <path d="M32 26 L54 38 L32 50 L10 38 Z" fill="#2e2460" />

      {/* The cap: two skirts sloping out to the base, then the typing surface. */}
      <path d="M12 16 L32 28 L32 40 L8 26 Z" fill="#d4d0cc" />
      <path d="M52 16 L32 28 L32 40 L56 26 Z" fill="#e8e4e0" />
      <path d="M32 4 L52 16 L32 28 L12 16 Z" fill="#ffffff" />

      {/* One hairline around the cap, so white-on-white still holds a shape. */}
      <path
        d="M32 4 L52 16 L56 26 L32 40 L8 26 L12 16 Z"
        stroke="rgba(17,17,17,0.24)"
        strokeWidth="1.5"
        strokeLinejoin="round"
      />

      {/* The legend, in the top face's plane so it sits on the cap. Kept inside
          the rhombus — |x-32|/20 + |y-16|/12 stays under 1, which the first
          draft did not, and the fork spilled off the edge of the key. */}
      <path
        d="M20.5 16 L32.6 16 M32.6 16 L41 12.6 M32.6 16 L41 19.4"
        stroke="#e8400d"
        strokeWidth="3.4"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}
