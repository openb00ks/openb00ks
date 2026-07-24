import js from "@eslint/js";
import svelte from "eslint-plugin-svelte";
import globals from "globals";
import ts from "typescript-eslint";

export default [
  js.configs.recommended,
  ...ts.configs.recommended,
  ...svelte.configs["flat/recommended"],
  {
    languageOptions: {
      globals: {
        ...globals.browser,
        ...globals.node,
      },
    },
  },
  {
    files: ["**/*.svelte"],
    languageOptions: {
      parserOptions: {
        parser: ts.parser,
      },
    },
    rules: {
      "no-useless-assignment": "off",
      "svelte/no-navigation-without-resolve": "off",
      "svelte/require-each-key": "off",
      "svelte/no-immutable-reactive-statements": "off",
      "svelte/prefer-svelte-reactivity": "off",
    },
  },
  {
    ignores: [
      ".svelte-kit/**",
      "build/**",
      "dist/**",
      "node_modules/**",
      "coverage/**",
    ],
  },
];
