import type { Metadata } from "next";
import { IBM_Plex_Mono, IBM_Plex_Sans, Spectral } from "next/font/google";

import { Providers } from "@/app/providers";
import { Rail } from "@/components/rail";
import { Toaster } from "@/components/ui/sonner";

import "./globals.css";

const sans = IBM_Plex_Sans({
  subsets: ["latin", "vietnamese"],
  weight: ["400", "500", "600"],
  variable: "--font-plex-sans",
});
const serif = Spectral({
  subsets: ["latin", "vietnamese"],
  weight: ["400", "600"],
  style: ["normal", "italic"],
  variable: "--font-spectral",
});
const mono = IBM_Plex_Mono({
  subsets: ["latin", "vietnamese"],
  weight: ["400", "500"],
  variable: "--font-plex-mono",
});

export const metadata: Metadata = {
  title: "Doc Insight",
  description: "Hỏi đáp tài liệu nội bộ, câu trả lời kèm trích dẫn tới trang gốc.",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="vi" className={`${sans.variable} ${serif.variable} ${mono.variable}`}>
      <body className="flex h-screen overflow-hidden">
        <Providers>
          <Rail />
          <main className="flex h-screen flex-1 flex-col overflow-hidden">{children}</main>
          <Toaster position="bottom-center" />
        </Providers>
      </body>
    </html>
  );
}
