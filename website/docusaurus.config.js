// @ts-check
const {themes} = require('prism-react-renderer');

/** @type {import('@docusaurus/types').Config} */
const config = {
  title: 'gotq',
  tagline: 'Safe, schema-first HTTP queries for GORM.',
  favicon: 'img/favicon.svg',

  url: 'https://dmedovich.github.io',
  baseUrl: '/gotq/',
  organizationName: 'dmedovich',
  projectName: 'gotq',
  trailingSlash: false,

  onBrokenLinks: 'throw',

  i18n: {
    defaultLocale: 'en',
    locales: ['en'],
  },

  presets: [
    [
      'classic',
      /** @type {import('@docusaurus/preset-classic').Options} */
      ({
        docs: {
          routeBasePath: 'docs',
          sidebarPath: require.resolve('./sidebars.js'),
          editUrl: 'https://github.com/dmedovich/gotq/tree/main/website/',
        },
        blog: false,
        theme: {
          customCss: require.resolve('./src/css/custom.css'),
        },
      }),
    ],
  ],

  themeConfig:
    /** @type {import('@docusaurus/preset-classic').ThemeConfig} */
    ({
      metadata: [
        {
          name: 'description',
          content: 'Schema-validated HTTP query parameters for GORM.',
        },
      ],
      navbar: {
        title: 'gotq',
        items: [
          {
            type: 'docSidebar',
            sidebarId: 'docs',
            position: 'left',
            label: 'Docs',
          },
          {to: '/docs/getting-started', label: 'Quickstart', position: 'left'},
          {to: '/docs/examples', label: 'Examples', position: 'left'},
          {
            href: 'https://pkg.go.dev/github.com/dmedovich/gotq',
            label: 'Go API',
            position: 'right',
          },
          {
            href: 'https://github.com/dmedovich/gotq',
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
              {label: 'Getting started', to: '/docs/getting-started'},
              {label: 'Query language', to: '/docs/query-language'},
              {label: 'Security', to: '/docs/errors-and-security'},
            ],
          },
          {
            title: 'Project',
            items: [
              {label: 'GitHub', href: 'https://github.com/dmedovich/gotq'},
              {label: 'Go reference', href: 'https://pkg.go.dev/github.com/dmedovich/gotq'},
              {label: 'License', href: 'https://github.com/dmedovich/gotq/blob/main/LICENSE'},
            ],
          },
        ],
        copyright: `Copyright © ${new Date().getFullYear()} gotq.`,
      },
      prism: {
        theme: themes.github,
        darkTheme: themes.dracula,
        additionalLanguages: ['go', 'bash', 'json'],
      },
      colorMode: {
        respectPrefersColorScheme: true,
      },
    }),
};

module.exports = config;
