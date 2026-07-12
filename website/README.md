# Palhelm showcase website

The marketing/showcase site for Palhelm (the panel) and its Discord companion bot,
printed in the product's own expedition-field-guide design system. Astro + Svelte 5 +
Tailwind CSS v4, static output.

```sh
npm install
npm run dev       # dev server on :4321
npm run build     # static build → dist/
npm run preview   # serve the built output
```

- `DESIGN.md` — the build contract: concept, tokens, components, page specs.
- `src/styles/tokens.css` — verbatim copy of `design/mockups/tokens.css` plus a small
  marketing type-scale extension. The product repo stays the source of truth.
- `src/assets/shots/` — panel captures taken from the frontend's mock mode (`?mock`);
  pal icons show placeholder chips there because mock mode serves no icon files.
- `public/hero/` — the ambient hero footage: grok `image_to_video` over
  `assets/v2-explorations/login/hero-paper.png` (license-clean, no game assets),
  ping-pong-looped and encoded to webm/mp4 with a poster fallback.
- Every screenshot plate participates in the noon / night-camp toggle in the spine.

The GitHub links point at https://github.com/8tp/palhelm and
https://github.com/8tp/palhelm-bot.
