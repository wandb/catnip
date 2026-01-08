import js from "@eslint/js";
import globals from "globals";
import reactHooks from "eslint-plugin-react-hooks";
import reactRefresh from "eslint-plugin-react-refresh";
import tseslint from "typescript-eslint";
import reactX from "eslint-plugin-react-x";
import reactDom from "eslint-plugin-react-dom";

export default tseslint.config([
  {
    ignores: [
      "dist/**",
      "reference/**",
      ".wrangler/**",
      "node_modules/**",
      "build/**",
      "coverage/**",
      "*.config.js",
      "*.config.ts",
      "container/**",
      "containers/**",
      "public/**",
      "scripts/**",
      "worker/scripts/**",
      "catnip-mobile/**",
    ],
  },
  {
    files: ["**/*.{ts,tsx}"],
    extends: [
      js.configs.recommended,
      ...tseslint.configs.recommended,
      reactX.configs["recommended-typescript"],
      reactDom.configs.recommended,
      reactHooks.configs["recommended-latest"],
      reactRefresh.configs.vite,
    ],
    languageOptions: {
      ecmaVersion: 2020,
      globals: {
        ...globals.browser,
        ...globals.node,
      },
      parserOptions: {
        projectService: {
          allowDefaultProject: ["*.js", "*.mjs"],
        },
        tsconfigRootDir: import.meta.dirname,
      },
    },
    rules: {
      // Disable overly strict rules
      "@typescript-eslint/no-explicit-any": "off",
      "@typescript-eslint/no-unsafe-assignment": "off",
      "@typescript-eslint/no-unsafe-member-access": "off",
      "@typescript-eslint/no-unsafe-argument": "off",
      "@typescript-eslint/no-unsafe-return": "off",
      "@typescript-eslint/no-unsafe-call": "off",
      "@typescript-eslint/prefer-nullish-coalescing": "off",
      "@typescript-eslint/no-floating-promises": [
        "error",
        { ignoreVoid: true },
      ],
      "@typescript-eslint/no-misused-promises": [
        "error",
        { checksVoidReturn: false },
      ],
      "@typescript-eslint/require-await": "off",
      "@typescript-eslint/await-thenable": "off",
      "@typescript-eslint/no-unnecessary-type-assertion": "off",
      "@typescript-eslint/no-inferrable-types": "off",
      "@typescript-eslint/consistent-generic-constructors": "off",
      "@typescript-eslint/restrict-template-expressions": "off",
      "@typescript-eslint/unbound-method": "off",

      // React specific
      "react-hooks/exhaustive-deps": "off", // Too many false positives
      "react-dom/no-dangerously-set-innerhtml": "off",
      "react-dom/no-missing-button-type": "off",
      "react-dom/no-unsafe-iframe-sandbox": "off",
      "react-x/no-array-index-key": "off",
      "react-x/no-unstable-default-props": "off",
      "react-x/no-unstable-context-value": "off",
      "react-x/no-use-context": "off",

      // Allow unused vars with underscore prefix
      "@typescript-eslint/no-unused-vars": [
        "error",
        {
          argsIgnorePattern: "^_",
          varsIgnorePattern: "^_",
          caughtErrorsIgnorePattern: "^_",
        },
      ],
    },
  },
  {
    // Special rules for tooltip component to prevent ESLint from breaking it
    files: ["**/components/ui/tooltip.tsx"],
    rules: {
      // Disable the rule that tries to convert Provider components to React 19 style
      // This breaks Radix UI TooltipPrimitive which requires explicit .Provider usage
      "react-x/no-context-provider": "off",
    },
  },
]);
