import adapter from "@sveltejs/adapter-static";

const config = {
  compilerOptions: {
    runes: true,
  },
  kit: {
    // SPA: the app is fully client-rendered (calls the API from the browser), so emit a
    // fallback entry page that boots the client router for any route. nginx serves it via
    // `try_files $uri /index.html`. Without this, the static build has no index.html.
    adapter: adapter({
      strict: false,
      fallback: "index.html",
    }),
  },
};

export default config;
