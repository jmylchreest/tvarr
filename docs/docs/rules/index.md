---
title: Filtering & Rules
description: Expression-based filtering and data transformation
sidebar_position: 3
---

# Filtering & Rules

tvarr uses a powerful expression language to filter channels and transform data.

## Two Types of Rules

| Type | Purpose | Has SET? |
|------|---------|----------|
| **Filters** | Include or exclude channels | No |
| **Data Mapping** | Transform channel fields | Yes |

## How They're Applied

During proxy generation:

1. **Data Mapping** runs first - transforms field values
2. **Filters** run second - includes/excludes based on transformed values

This means you can map messy data first, then filter on clean values.

import DocCardList from '@theme/DocCardList';

<DocCardList />
