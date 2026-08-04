import type { Metadata } from "next";
import { Inter } from "next/font/google";

import { Providers } from "./providers";
import "./globals.css";

/**
 * DESIGN.md's typeface is Labil Grotesk Variable, which is not distributable.
 * Inter is the first substitute it names, and it carries the variable weight
 * axis the system depends on: 400 for everything up to 56px, 900 reserved for
 * the 84px display moment.
 */
const inter = Inter({
  subsets: ["latin"],
  variable: "--font-inter",
  display: "swap",
});

export const metadata: Metadata = {
  title: "Switchyard",
  description: "AI drafts the route. You throw the switches.",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="en" className={inter.variable}>
      <body>
        <Providers>{children}</Providers>
      </body>
    </html>
  );
}
