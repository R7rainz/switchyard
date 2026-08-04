"use client";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useState, type ReactNode } from "react";

export function Providers({ children }: { children: ReactNode }) {
  // Created in state rather than at module scope: a module-level client is
  // shared between requests on the server, which leaks one user's cache into
  // the next user's render.
  const [client] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: {
            // Anything from the API belongs to Query. A short stale time keeps
            // a tab that has been sitting open honest without refetching on
            // every focus change.
            staleTime: 10_000,
            retry: 1,
          },
        },
      }),
  );

  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}
