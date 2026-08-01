import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "Switchyard",
  description: "AI drafts the route. You throw the switches.",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
