import type { Metadata, Viewport } from "next";
import { IBM_Plex_Sans_Thai_Looped, Mali } from "next/font/google";

import "./globals.css";

// Mali: rounded, playful display face; Plex Thai Looped: easy-to-read chat text.
const display = Mali({ weight: ["500", "600", "700"], subsets: ["latin", "thai"], variable: "--font-display" });
const body = IBM_Plex_Sans_Thai_Looped({
  weight: ["400", "500", "600"],
  subsets: ["latin", "thai"],
  variable: "--font-body",
});

export const metadata: Metadata = {
  title: "smalltalk",
  description: "ห้องแชทที่เข้าได้ด้วยลิงก์หรือ QR ไม่ต้องลงแอป ไม่ต้องสมัครสมาชิก",
};

export const viewport: Viewport = {
  width: "device-width",
  initialScale: 1,
  interactiveWidget: "resizes-content",
  themeColor: [
    { media: "(prefers-color-scheme: light)", color: "#fdf6ff" },
    { media: "(prefers-color-scheme: dark)", color: "#16111f" },
  ],
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="th" className={`${display.variable} ${body.variable}`}>
      <body>{children}</body>
    </html>
  );
}
