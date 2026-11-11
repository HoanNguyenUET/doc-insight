import { clsx, type ClassValue } from "clsx";
import { extendTailwindMerge } from "tailwind-merge";

// Token tự đặt tên trong globals.css (@theme): khai báo cho tailwind-merge để
// nó gộp đúng nhóm (vd. không coi text-body là màu chữ).
const twMerge = extendTailwindMerge({
  extend: {
    theme: {
      text: ["2xs", "meta", "caption", "small", "ui", "body", "title", "heading", "display"],
      radius: ["card", "control", "control-sm"],
      shadow: ["card", "sheet", "tab", "pop", "focus"],
    },
  },
});

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}
