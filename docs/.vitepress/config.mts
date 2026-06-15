import { defineConfig } from 'vitepress'

// VitePress configuration for the radix documentation site.
//
// The published site is built only from `docs/site/` (see `srcDir` below) so the
// internal design docs that already live in `docs/` (EXECUTION_PLAN.md,
// COMMAND_DESIGN.md, CUSTOM_AUTH_PROVIDER.md) are never scanned or published.
//
// Deployed to a GitHub Pages *project* site at https://osuritz.github.io/radix/,
// hence `base: '/radix/'`.
export default defineConfig({
  // Build content from docs/site only.
  srcDir: './site',

  title: 'Radix',
  description:
    'Multi-mode HTTP server for local development: static serving, reverse proxy, request echo, and API mocking — one self-contained Go binary.',

  // GitHub Pages project-site base path.
  base: '/radix/',

  // Clean URLs (drop the trailing .html).
  cleanUrls: true,

  // Last-updated timestamps from git.
  lastUpdated: true,

  head: [['meta', { name: 'theme-color', content: '#3c8772' }]],

  themeConfig: {
    nav: [
      { text: 'Home', link: '/' },
      { text: 'Getting Started', link: '/getting-started' },
      {
        text: 'Commands',
        items: [
          { text: 'serve', link: '/commands/serve' },
          { text: 'proxy', link: '/commands/proxy' },
          { text: 'echo', link: '/commands/echo' },
          { text: 'mock', link: '/commands/mock' },
          { text: 'gencert', link: '/commands/gencert' },
          { text: 'version', link: '/commands/version' },
          { text: 'validate', link: '/commands/validate' },
        ],
      },
      { text: 'Configuration', link: '/configuration' },
      {
        text: 'Guides',
        items: [
          { text: 'Mock guide', link: '/guides/mock' },
          { text: 'TLS / HTTPS', link: '/guides/tls' },
          { text: 'Observability', link: '/guides/observability' },
          { text: 'Logging', link: '/guides/logging' },
        ],
      },
      { text: 'Troubleshooting', link: '/troubleshooting' },
    ],

    sidebar: [
      {
        text: 'Introduction',
        items: [
          { text: 'Overview', link: '/' },
          { text: 'Getting started', link: '/getting-started' },
        ],
      },
      {
        text: 'Commands',
        collapsed: false,
        items: [
          { text: 'serve', link: '/commands/serve' },
          { text: 'proxy', link: '/commands/proxy' },
          { text: 'echo', link: '/commands/echo' },
          { text: 'mock', link: '/commands/mock' },
          { text: 'gencert', link: '/commands/gencert' },
          { text: 'version', link: '/commands/version' },
          { text: 'validate', link: '/commands/validate' },
        ],
      },
      {
        text: 'Reference',
        items: [{ text: 'Configuration', link: '/configuration' }],
      },
      {
        text: 'Guides',
        collapsed: false,
        items: [
          { text: 'Mock guide', link: '/guides/mock' },
          { text: 'TLS / HTTPS', link: '/guides/tls' },
          { text: 'Observability', link: '/guides/observability' },
          { text: 'Logging', link: '/guides/logging' },
        ],
      },
      {
        text: 'Help',
        items: [{ text: 'Troubleshooting / FAQ', link: '/troubleshooting' }],
      },
    ],

    socialLinks: [
      { icon: 'github', link: 'https://github.com/osuritz/radix' },
    ],

    editLink: {
      pattern: 'https://github.com/osuritz/radix/edit/main/docs/site/:path',
      text: 'Edit this page on GitHub',
    },

    search: {
      provider: 'local',
    },

    footer: {
      message: 'Released under the MIT License.',
      copyright: 'Copyright © 2026 Radix contributors',
    },
  },
})
