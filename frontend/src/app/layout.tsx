import type { Metadata } from "next";
import { GeistMono } from "geist/font/mono";
import { GeistSans } from "geist/font/sans";

import { Providers } from "./providers";
import "./globals.css";

export const metadata: Metadata = {
  title: "Switchyard",
  description: "AI drafts the route. You throw the switches.",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    // Self-hosted rather than fetched. The type is the voice of this system,
    // and a webfont that arrives late reflows every page it lands on.
    <html lang="en" className={`${GeistSans.variable} ${GeistMono.variable}`}>
      <body>
        <Providers>{children}</Providers>
      </body>
    </html>
  );
}
