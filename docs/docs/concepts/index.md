---
title: Core Concepts
description: Understand how tvarr works
sidebar_position: 2
---

# Core Concepts

Understanding how tvarr's components work together.

## The Big Picture

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Sources   │ ──▶ │   Filters   │ ──▶ │   Proxies   │ ──▶ Players
│  (M3U/EPG)  │     │  & Mapping  │     │  (Output)   │
└─────────────┘     └─────────────┘     └─────────────┘
```

1. **Sources** - Where your streams and EPG data come from
2. **Filters & Mapping** - Rules to curate and transform
3. **Proxies** - Output playlists for your players

import DocCardList from '@theme/DocCardList';

<DocCardList />
