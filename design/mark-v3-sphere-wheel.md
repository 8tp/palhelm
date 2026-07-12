# Mark v3 "sphere-wheel" (2026-07-12 rebrand)

Replaces the orb-hybrid mark (gold top hemisphere + equatorial band), which read as a
Pokeball. v3 keeps the helm wheel and reworks the orb as a generic capture sphere:
a diagonal split with a leaf-green upper half and a small offset diamond window.
No horizontal band, no center button, no gold. Gold (--brass) retires from the mark;
the token stays defined for now but nothing brand-level uses it.

## Brand constant (add beside --brass in every tokens.css copy)

```css
--sphere: #63ad50;   /* capture-sphere green: the mark's fill. Never UI chrome. */
```

Theme-independent, like --brass was. Line work (ring/ticks/split) takes the CONTEXT
color: `--accent` on page surfaces (light `#55682b` / dark `#93a653` resolve via the
token), `#ccd79b` (band-chart) on the olive band, exactly as the old mark did.

## Canonical geometry (viewBox 0 0 26 26) — display cut, >=20px

```svg
<svg viewBox="0 0 26 26" fill="none">
  <!-- upper-left half-disc: chord from 225deg to 45deg, arc through top-left -->
  <path d="M6.99 19.01 A8.5 8.5 0 1 1 19.01 6.99 Z" fill="var(--sphere)"/>
  <!-- split chord -->
  <path d="M6.99 19.01 L19.01 6.99" stroke="LINE" stroke-width="2" stroke-linecap="round"/>
  <!-- diamond window, offset into the open half -->
  <rect x="15.1" y="15.1" width="3.0" height="3.0" rx="0.6" transform="rotate(45 16.6 16.6)" fill="var(--sphere)"/>
  <!-- ring -->
  <circle cx="13" cy="13" r="8.5" stroke="LINE" stroke-width="2"/>
  <!-- 8 ticks (unchanged from v2) -->
  <g stroke="LINE" stroke-width="2" stroke-linecap="round">
    <path d="M13 1.5v4"/><path d="M13 20.5v4"/><path d="M1.5 13h4"/><path d="M20.5 13h4"/>
    <path d="M4.87 4.87l2.83 2.83"/><path d="M18.3 18.3l2.83 2.83"/>
    <path d="M21.13 4.87l-2.83 2.83"/><path d="M7.7 18.3l-2.83 2.83"/>
  </g>
</svg>
```

`LINE` = context color (see above). The old center dot is gone.

## Favicon cut (16px): same geometry MINUS the diamond (it muddies below 20px).
Static colors, no custom properties (favicon documents can't read app tokens):
light: line #55682b, sphere #63ad50; dark (prefers-color-scheme): line #93a653,
sphere #63ad50.

## Where the mark lives (all must swap)

- frontend/src/components/icons.tsx (panel rail/login/spinner mark)
- frontend/public/favicon.svg
- website/src/components/Mark.astro, website/public/favicon.svg
- docs-site/src/assets/mark.svg (band context: line #ccd79b baked), docs-site/public/favicon.svg
- assets/mark.svg, assets/favicon.svg (repo README branding)
- OG images (og-bot.png, og-docs.png: regenerate; og-dashboard/og-panel: re-crop from
  re-captured screenshots)

The mark reads legibly at 16, 26, and 64 px on the cream and night surfaces and on the
olive band.
