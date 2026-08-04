/**
 * The mark: an isometric keyswitch.
 *
 * Two readings on purpose. The cross is a Cherry MX stem — the shape anyone who
 * has pulled a keycap recognises instantly — and it is also a crossing, which
 * is what a switchyard is: the place tracks branch. The tagline is already
 * "you throw the switches", so the mark says the same thing the product does.
 *
 * Dimensionality comes from three flat faces rather than a gradient or a
 * shadow. DESIGN.md is explicit that flat colour is the elevation, and a
 * gradient here would be the one thing the system refuses everywhere else.
 */
export function Logo({ size = 20, className }: { size?: number; className?: string }) {
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
      {/* Left face — the shadow side, so the darkest of the three. */}
      <path d="M2 26 L32 44 L32 58 L2 40 Z" fill="#111111" />
      {/* Right face, one step up from the left so the form reads as solid. */}
      <path d="M62 26 L32 44 L32 58 L62 40 Z" fill="#1b1a19" />
      {/* Top face — the lit plane, and the one the stem sits on. */}
      <path d="M32 8 L62 26 L32 44 L2 26 Z" fill="#302e2c" />

      {/* The stem. Orange is the one accent this system allows, and it is
          reserved for the thing that carries meaning — here, the switch. */}
      <path
        d="M38 18.4 L44.6 22.4 L38.6 26 L44.6 29.6 L38 33.6 L32 30 L26 33.6 L19.4 29.6 L25.4 26 L19.4 22.4 L26 18.4 L32 22 Z"
        fill="#e8400d"
      />
    </svg>
  );
}
