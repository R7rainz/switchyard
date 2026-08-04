"use client";

import { useEffect, useRef, type ReactNode } from "react";

import { Eyebrow } from "./ui";

/**
 * A modal built on <dialog>.
 *
 * The native element already does focus trapping, an inert background, Escape
 * to close, and the top layer — all the parts a hand-rolled modal gets wrong
 * and a component library is usually installed to provide. This is the wrapper
 * that connects it to React state; there is no dialog dependency here.
 */
export function Modal({
  open,
  onClose,
  title,
  children,
}: {
  open: boolean;
  onClose: () => void;
  title: string;
  children: ReactNode;
}) {
  const ref = useRef<HTMLDialogElement>(null);

  useEffect(() => {
    const dialog = ref.current;
    if (!dialog) return;
    // showModal is what puts it in the top layer and makes the rest inert;
    // toggling the open attribute renders it inline and unguarded.
    if (open && !dialog.open) dialog.showModal();
    if (!open && dialog.open) dialog.close();
  }, [open]);

  return (
    <dialog
      ref={ref}
      // Escape and a backdrop click both arrive here, so the parent has one
      // place to learn the dialog is gone.
      onClose={onClose}
      onClick={(event) => {
        // The backdrop is the dialog element itself; a click landing on it
        // rather than on the content means outside.
        if (event.target === ref.current) ref.current?.close();
      }}
      className="m-auto w-full max-w-lg rounded-xl border border-hairline bg-canvas-white p-0 text-ink shadow-[var(--shadow-featured)]"
    >
      <div className="flex flex-col gap-6 p-6">
        <Eyebrow>{title}</Eyebrow>
        {children}
      </div>
    </dialog>
  );
}
