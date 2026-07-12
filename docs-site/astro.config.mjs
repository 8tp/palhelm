// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

export default defineConfig({
	site: 'https://docs.palhelm.com',
	integrations: [
		starlight({
			title: 'palhelm docs',
			description:
				'Documentation for Palhelm, a self-hosted control panel and Discord bot for dedicated Palworld servers.',
			favicon: '/favicon.svg',
			// Social embed card: Starlight emits og:title/description itself, but no
			// image. One default card serves every docs page.
			head: [
				{ tag: 'meta', attrs: { property: 'og:image', content: 'https://docs.palhelm.com/og-docs.png' } },
				{ tag: 'meta', attrs: { property: 'og:image:width', content: '1200' } },
				{ tag: 'meta', attrs: { property: 'og:image:height', content: '630' } },
				{ tag: 'meta', attrs: { name: 'twitter:card', content: 'summary_large_image' } },
				{ tag: 'meta', attrs: { name: 'twitter:image', content: 'https://docs.palhelm.com/og-docs.png' } },
			],
			social: [
				{ icon: 'github', label: 'GitHub', href: 'https://github.com/8tp/palhelm' },
			],
			components: {
				// Custom site title: wires src/assets/mark.svg as the logo and renders the
				// two-tone "palhelm docs" wordmark on the olive spine header.
				SiteTitle: './src/components/SiteTitle.astro',
			},
			customCss: ['./src/styles/tokens.css', './src/styles/custom.css'],
			expressiveCode: {
				// Warm cream/olive-friendly syntax themes; backgrounds are overridden
				// below with the --console-* tokens so blocks read as console wells.
				themes: ['gruvbox-dark-hard', 'gruvbox-light-hard'],
				styleOverrides: {
					borderColor: 'var(--line)',
					borderWidth: '1.5px',
					borderRadius: '8px',
					codeBackground: 'var(--console-bg)',
					codeFontFamily: 'var(--font-mono)',
					uiFontFamily: 'var(--font-mono)',
					focusBorder: 'var(--accent)',
					frames: {
						shadowColor: 'transparent',
						terminalBackground: 'var(--console-bg)',
						terminalTitlebarBackground: 'var(--surface-2)',
						terminalTitlebarForeground: 'var(--ink-2)',
						terminalTitlebarDotsForeground: 'var(--line-strong)',
						editorTabBarBackground: 'var(--surface-2)',
						editorTabBarBorderBottomColor: 'var(--line)',
						editorActiveTabBackground: 'var(--console-bg)',
						editorActiveTabForeground: 'var(--console-cmd)',
					},
				},
			},
			sidebar: [
				{ label: 'Getting started', items: [{ autogenerate: { directory: 'getting-started' } }] },
				{ label: 'Panel', items: [{ autogenerate: { directory: 'panel' } }] },
				{ label: 'Integration API', items: [{ autogenerate: { directory: 'integration-api' } }] },
				{ label: 'Bot', items: [{ autogenerate: { directory: 'bot' } }] },
				{ label: 'Architecture', items: [{ autogenerate: { directory: 'architecture' } }] },
				{ label: 'Contributing', items: [{ autogenerate: { directory: 'contributing' } }] },
			],
		}),
	],
});
