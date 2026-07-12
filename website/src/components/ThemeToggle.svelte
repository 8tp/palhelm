<script lang="ts">
  // Initial state comes from data-theme, which the no-flash script in
  // Layout.astro has already resolved (localStorage → prefers-color-scheme)
  // before this island hydrates.
  let theme = $state<"light" | "dark">(
    typeof document !== "undefined" &&
      document.documentElement.dataset.theme === "dark"
      ? "dark"
      : "light",
  );

  function toggle() {
    theme = theme === "dark" ? "light" : "dark";
    document.documentElement.dataset.theme = theme;
    try {
      localStorage.setItem("palhelm-site.theme", theme);
    } catch {
      // private mode etc. , the toggle still works for this page view
    }
  }
</script>

<button
  class="chip-toggle"
  type="button"
  aria-pressed={theme === "dark"}
  onclick={toggle}
>
  <span class="dot" aria-hidden="true"></span>
  {theme === "dark" ? "dark" : "light"}
</button>

<style>
  /* stamped toggle chip (mockups' .chip-toggle): hollow dot for the light
     theme, olive-filled dot for the dark theme */
  .chip-toggle {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    font: 600 11px/1 var(--font-mono);
    text-transform: uppercase;
    letter-spacing: 0.05em;
    padding: 6px 10px;
    border-radius: 5px;
    border: var(--border-ctl) solid color-mix(in srgb, var(--ink-3) 45%, transparent);
    background: var(--surface);
    color: var(--ink-2);
    box-shadow: var(--shadow-card);
    cursor: pointer;
    white-space: nowrap;
  }
  .dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    flex: none;
    border: var(--border-ctl) solid var(--ink-3); /* hollow: light theme */
  }
  .chip-toggle:hover { background: var(--surface-2); color: var(--ink); }
  .chip-toggle:active { translate: 0 1px; }
  .chip-toggle[aria-pressed="true"] {
    background: linear-gradient(var(--accent-soft), var(--accent-soft)) var(--surface);
    color: var(--accent-ink);
    border-color: color-mix(in srgb, var(--accent) 55%, transparent);
  }
  .chip-toggle[aria-pressed="true"] .dot {
    border: 0;
    background: var(--accent); /* filled: dark theme */
  }
</style>
