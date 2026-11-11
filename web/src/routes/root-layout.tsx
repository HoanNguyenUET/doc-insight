import { Outlet } from "react-router";

import { Rail } from "@/components/rail";
import { Toaster } from "@/components/ui/sonner";

export function RootLayout() {
  return (
    <>
      <Rail />
      <main className="flex h-screen flex-1 flex-col overflow-hidden">
        <Outlet />
      </main>
      <Toaster position="bottom-center" />
    </>
  );
}
