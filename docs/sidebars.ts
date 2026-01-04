import type { SidebarsConfig } from '@docusaurus/plugin-content-docs';

const sidebars: SidebarsConfig = {
  docs: [
    'intro',
    {
      type: 'category',
      label: 'Getting Started',
      link: {
        type: 'doc',
        id: 'quickstart/index',
      },
      items: [
        'quickstart/docker-compose',
        'quickstart/kubernetes',
        'quickstart/first-steps',
      ],
    },
    {
      type: 'category',
      label: 'Core Concepts',
      link: {
        type: 'doc',
        id: 'concepts/index',
      },
      items: [
        'concepts/workflow',
        'concepts/sources',
        'concepts/proxies',
        'concepts/channels',
      ],
    },
    {
      type: 'category',
      label: 'Filtering & Rules',
      link: {
        type: 'doc',
        id: 'rules/index',
      },
      items: [
        'rules/expression-editor',
        'rules/filters',
        'rules/data-mapping',
        'rules/client-detection',
      ],
    },
    {
      type: 'category',
      label: 'Transcoding',
      link: {
        type: 'doc',
        id: 'transcoding/index',
      },
      items: [
        'transcoding/encoding-profiles',
        'transcoding/ffmpegd',
        'transcoding/hardware-acceleration',
      ],
    },
    {
      type: 'category',
      label: 'User Interface',
      link: {
        type: 'doc',
        id: 'ui/index',
      },
      items: [
        'ui/dashboard',
        'ui/sources',
        'ui/channels',
        'ui/proxies',
        'ui/epg',
        'ui/admin',
        'ui/settings',
      ],
    },
    {
      type: 'category',
      label: 'Configuration',
      link: {
        type: 'doc',
        id: 'configuration/index',
      },
      items: [
        'configuration/environment',
        'configuration/database',
        'configuration/storage',
      ],
    },
    {
      type: 'category',
      label: 'Advanced',
      link: {
        type: 'doc',
        id: 'advanced/index',
      },
      items: [
        'advanced/pipeline',
        'advanced/relay-architecture',
        'advanced/distributed-transcoding',
        'advanced/api',
      ],
    },
    'debugging',
    {
      type: 'category',
      label: 'Changelog',
      link: {
        type: 'doc',
        id: 'changelog/index',
      },
      items: [
        'changelog/unreleased',
        'changelog/v0.0.1',
      ],
    },
  ],
};

export default sidebars;
