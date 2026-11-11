import "@fontsource/ibm-plex-mono/400.css";
import "@fontsource/ibm-plex-mono/500.css";
import "@fontsource/ibm-plex-sans/400.css";
import "@fontsource/ibm-plex-sans/500.css";
import "@fontsource/ibm-plex-sans/600.css";
import "@fontsource/spectral/400.css";
import "@fontsource/spectral/400-italic.css";
import "@fontsource/spectral/600.css";
import "./globals.css";

import { lazy, StrictMode, Suspense } from "react";
import { createRoot } from "react-dom/client";
import { createBrowserRouter, Navigate } from "react-router";
import { RouterProvider } from "react-router/dom";

import { Providers } from "@/providers";
import { ContentPage } from "@/routes/content";
import { DocumentsPage } from "@/routes/documents";
import { RootLayout } from "@/routes/root-layout";
import { UploadPage } from "@/routes/upload";

// Màn hỏi đáp kéo theo pdf.js (nặng): tải riêng khi mở tài liệu, để trang
// danh sách và tải lên nhẹ hơn.
const AskPage = lazy(() => import("@/routes/ask").then((m) => ({ default: m.AskPage })));

const router = createBrowserRouter([
  {
    element: <RootLayout />,
    children: [
      { index: true, element: <Navigate to="/documents" replace /> },
      { path: "documents", element: <DocumentsPage /> },
      { path: "documents/upload", element: <UploadPage /> },
      {
        path: "documents/:id",
        element: (
          <Suspense fallback={<p className="m-auto text-sm text-ink-3">Đang mở tài liệu…</p>}>
            <AskPage />
          </Suspense>
        ),
      },
      { path: "documents/:id/content", element: <ContentPage /> },
      // Đường dẫn cũ của màn này.
      { path: "documents/:id/chunks", element: <Navigate to="../content" relative="path" replace /> },
      { path: "*", element: <Navigate to="/documents" replace /> },
    ],
  },
]);

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <Providers>
      <RouterProvider router={router} />
    </Providers>
  </StrictMode>,
);
