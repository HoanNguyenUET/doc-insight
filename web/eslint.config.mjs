import js from "@eslint/js";
import reactHooks from "eslint-plugin-react-hooks";
import reactRefresh from "eslint-plugin-react-refresh";
import globals from "globals";
import tseslint from "typescript-eslint";

export default tseslint.config(
  { ignores: ["dist/**", "node_modules/**"] },
  {
    files: ["**/*.{ts,tsx}"],
    extends: [js.configs.recommended, ...tseslint.configs.recommended, reactHooks.configs.flat.recommended],
    languageOptions: { globals: globals.browser },
    plugins: { "react-refresh": reactRefresh },
    rules: {
      "react-refresh/only-export-components": ["warn", { allowConstantExport: true }],
    },
  },
  // File entry không phải ranh giới fast refresh.
  { files: ["src/main.tsx"], rules: { "react-refresh/only-export-components": "off" } },
  // Component shadcn sinh tự động xuất kèm biến thể (buttonVariants...).
  { files: ["src/components/ui/**"], rules: { "react-refresh/only-export-components": "off" } },
);
