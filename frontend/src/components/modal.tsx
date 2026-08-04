"use client";

import { useEffect, useRef, type ReactNode } from "react";

import { Mono } from "./ui";

/**
 * A modal built on <dialog>.
 *
 * The native element already does focus trapping, inert background, Escape to
 * close, and the top layer — all the parts a hand-rolled modal gets wrong and
 * a component library is usually installed to provide. This is the wrapper that
 * connects it to React state; there is no dialog library here on purpose.
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
    // toggling the open attribute instead would render it inline and unguarded.
    if (open && !dialog.open) dialog.showModal();
    if (!open && dialog.open) dialog.close();
  }, [open]);

  return (
    <dialog
      ref={ref}
      // Escape and a backdrop click both come back through here, so the parent
      // has one place to learn the dialog is gone.
      onClose={onClose}
      onClick={(event) => {
        // The backdrop is the dialog element itself; a click landing on it
        // rather than on the content means outside.
        if (event.target === ref.current) ref.current?.close();
      }}
      className="m-auto w-full max-w-md rounded-lg border border-ash-stroke bg-obsidian-canvas p-0 text-bone backdrop:bg-obsidian-canvas/70"
    >
      <div className="flex flex-col gap-6 p-6">
        <Mono className="text-pale-stone">{title}</Mono>
        {children}
      </div>
    </dialog>
  );
}
