/** @type {import('tailwindcss').Config} */
module.exports = {
  darkMode: "class",
  content: ["./src/**/*.{html,js,svelte,ts}"],
  theme: {
    extend: {
      colors: {
        primary: "var(--primary)",
        accent: "var(--accent)",
        highlight: "var(--highlight)",
        ink: "var(--ink)",
        muted: "var(--muted)",
        paper: "var(--paper)",
        surface: "var(--surface)",
        line: "var(--line)",
        "line-strong": "var(--line-strong)",
      },
    },
  },
  plugins: [],
};
