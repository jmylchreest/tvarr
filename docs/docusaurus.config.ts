import { themes as prismThemes } from 'prism-react-renderer';
import type { Config } from '@docusaurus/types';
import type * as Preset from '@docusaurus/preset-classic';

// Load versions if available, fallback to current only
let versions: string[] = [];
try {
  versions = require('./versions.json');
} catch {
  // No versions file yet
}

const config: Config = {
  title: 'tvarr',
  tagline: 'IPTV Proxy & Stream Aggregator',
  favicon: 'img/favicon.ico',

  // GitHub Pages deployment settings
  url: 'https://jmylchreest.github.io',
  baseUrl: '/tvarr/',
  organizationName: 'jmylchreest',
  projectName: 'tvarr',
  deploymentBranch: 'gh-pages',
  trailingSlash: false,

  onBrokenLinks: 'throw',
  onBrokenMarkdownLinks: 'warn',

  i18n: {
    defaultLocale: 'en',
    locales: ['en'],
  },

  presets: [
    [
      'classic',
      {
        docs: {
          sidebarPath: './sidebars.ts',
          editUrl: 'https://github.com/jmylchreest/tvarr/tree/main/docs/',
          includeCurrentVersion: true,
          versions: {
            current: {
              label: 'main',
              path: 'next',
              banner: 'unreleased',
            },
          },
          lastVersion: versions.length > 0 ? versions[0] : 'current',
        },
        blog: false,
        theme: {
          customCss: './src/css/custom.css',
        },
      } satisfies Preset.Options,
    ],
  ],

  plugins: [
    [
      '@cmfcmf/docusaurus-search-local',
      {
        indexDocs: true,
        indexBlog: false,
        indexPages: true,
        language: 'en',
        maxSearchResults: 8,
      },
    ],
  ],

  themeConfig: {
    colorMode: {
      defaultMode: 'dark',
      disableSwitch: false,
      respectPrefersColorScheme: true,
    },
    navbar: {
      title: 'tvarr',
      logo: {
        alt: 'tvarr Logo',
        src: 'img/logo.svg',
      },
      items: [
        {
          type: 'docSidebar',
          sidebarId: 'docs',
          position: 'left',
          label: 'Documentation',
        },
        {
          type: 'docsVersionDropdown',
          position: 'right',
        },
        {
          href: 'https://github.com/jmylchreest/tvarr',
          label: 'GitHub',
          position: 'right',
        },
      ],
    },
    footer: {
      style: 'dark',
      links: [
        {
          title: 'Documentation',
          items: [
            {
              label: 'Getting Started',
              to: '/docs/next/quickstart/',
            },
            {
              label: 'Configuration',
              to: '/docs/next/configuration/',
            },
          ],
        },
        {
          title: 'Community',
          items: [
            {
              label: 'GitHub',
              href: 'https://github.com/jmylchreest/tvarr',
            },
            {
              label: 'Issues',
              href: 'https://github.com/jmylchreest/tvarr/issues',
            },
          ],
        },
      ],
      copyright: `Copyright ${new Date().getFullYear()} tvarr. Built with Docusaurus.`,
    },
    prism: {
      theme: prismThemes.github,
      darkTheme: prismThemes.dracula,
      additionalLanguages: ['bash', 'yaml', 'json', 'go', 'docker'],
    },
  } satisfies Preset.ThemeConfig,
};

export default config;
