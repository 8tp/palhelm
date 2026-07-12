<script lang="ts">
  import {
    commands,
    FILTERS,
    type Filter,
  } from "../../data/commands";

  let active = $state<Filter>("All");
  const shown = $derived(
    active === "All"
      ? commands
      : commands.filter((c) => c.category === active),
  );
</script>

<div class="ledger">
  <div class="filters" role="group" aria-label="Filter commands by category">
    {#each FILTERS as f (f)}
      <button
        type="button"
        class="filter"
        aria-pressed={active === f}
        onclick={() => (active = f)}
      >
        {f}
      </button>
    {/each}
  </div>

  <p class="count" aria-live="polite">
    Showing {shown.length} of {commands.length} commands{active === "All"
      ? ""
      : ` · ${active}`}
  </p>

  <div class="scroll">
    <table>
      <thead>
        <tr>
          <th scope="col" class="c-cmd">Command</th>
          <th scope="col" class="c-desc">What it does</th>
          <th scope="col" class="c-flag"><span class="sr-only">Access</span></th>
        </tr>
      </thead>
      <tbody>
        {#each shown as c (c.command)}
          <tr>
            <td class="c-cmd">
              <span class="cmd">{c.command}</span>
              {#if c.args}<span class="args">{c.args}</span>{/if}
            </td>
            <td class="c-desc">{c.description}</td>
            <td class="c-flag">
              {#if c.admin}<span class="stamp">Admin</span>{/if}
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
  </div>
</div>

<style>
  .ledger {
    background: var(--surface);
    border: var(--border-card) solid var(--line);
    border-radius: var(--radius-card);
    box-shadow: var(--shadow-card);
    padding: var(--space-4);
    min-width: 0;
  }

  /* ── filter chips ── */
  .filters {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
    margin-bottom: var(--space-3);
  }
  .filter {
    font: 600 11px / 1 var(--font-mono);
    text-transform: uppercase;
    letter-spacing: 0.05em;
    padding: 7px 12px;
    border-radius: 5px;
    border: var(--border-ctl) solid
      color-mix(in srgb, var(--ink-3) 45%, transparent);
    background: var(--surface);
    color: var(--ink-2);
    cursor: pointer;
    white-space: nowrap;
  }
  .filter:hover {
    background: var(--surface-2);
    color: var(--ink);
  }
  .filter:active {
    translate: 0 1px;
  }
  .filter[aria-pressed="true"] {
    background: linear-gradient(var(--accent-soft), var(--accent-soft))
      var(--surface);
    color: var(--accent-ink);
    border-color: color-mix(in srgb, var(--accent) 55%, transparent);
  }
  .filter:focus-visible {
    outline: none;
    box-shadow: var(--focus-ring);
  }

  .count {
    font: 600 var(--text-xs) / 1 var(--font-mono);
    text-transform: uppercase;
    letter-spacing: var(--track-caps);
    color: var(--ink-3);
    margin-bottom: var(--space-3);
  }

  /* ── ruled ledger table ── */
  .scroll {
    overflow-x: auto;
    min-width: 0;
  }
  table {
    width: 100%;
    border-collapse: collapse;
    font-size: var(--text-sm);
    min-width: 520px; /* forces horizontal scroll inside the card, never the page */
  }
  thead th {
    position: relative; /* contain the sr-only header inside the scroll box */
    text-align: left;
    font: 600 var(--text-xs) / 1 var(--font-mono);
    text-transform: uppercase;
    letter-spacing: var(--track-caps);
    color: var(--ink-3);
    padding: 0 var(--space-3) var(--space-2);
    border-bottom: var(--border-ctl) solid var(--line-strong);
  }
  tbody tr {
    border-bottom: 1px solid var(--line);
  }
  tbody tr:last-child {
    border-bottom: 0;
  }
  td {
    padding: var(--space-3);
    vertical-align: baseline;
  }
  .c-cmd {
    white-space: nowrap;
    width: 1%;
  }
  .cmd {
    font: 600 var(--text-sm) / 1.3 var(--font-mono);
    color: var(--accent-ink);
  }
  .args {
    font: 400 var(--text-xs) / 1.3 var(--font-mono);
    color: var(--ink-3);
    margin-left: 6px;
  }
  .c-desc {
    color: var(--ink-2);
    line-height: var(--leading-body);
  }
  .c-flag {
    text-align: right;
    white-space: nowrap;
    width: 1%;
  }

  /* local ADMIN stamp , matches Stamp.astro's idle tone (importing the Astro
     component into a Svelte island is not supported, so it's re-created here) */
  .stamp {
    display: inline-flex;
    align-items: center;
    font: 600 11px / 1 var(--font-mono);
    text-transform: uppercase;
    letter-spacing: 0.05em;
    padding: 6px 10px;
    border-radius: 5px;
    white-space: nowrap;
    border: var(--border-ctl) solid
      color-mix(in srgb, var(--warn) 55%, transparent);
    background: linear-gradient(var(--warn-soft), var(--warn-soft))
      var(--surface);
    color: var(--warn-ink);
    rotate: -1.5deg;
  }

  .sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    padding: 0;
    margin: -1px;
    overflow: hidden;
    clip-path: inset(50%);
    white-space: nowrap;
  }
</style>
