---
title: Pipeline Architecture
description: How the processing pipeline works
sidebar_position: 1
---

# Pipeline Architecture

tvarr processes data through a multi-stage pipeline.

## Pipeline Stages

When you generate a proxy, these stages run in order:

### Stage 0: Ingestion Guard

Waits for any active ingestion jobs to complete before starting generation. This ensures you're working with complete data.

### Stage 1: Load Channels

Loads all channels from the database for the proxy's linked stream sources.

```
Input: Source IDs
Output: []Channel
```

### Stage 2: Load Programs

Loads EPG programs from the proxy's linked EPG sources.

```
Input: EPG Source IDs, date range
Output: []Program
```

### Stage 3: Data Mapping

Applies data mapping rules in priority order:

```
Input: []Channel, []DataMappingRule
Output: []Channel (transformed)
```

Rules can:
- Rename fields
- Fix logos
- Extract data with regex
- Set EPG IDs

### Stage 4: Filtering

Applies filter rules:

```
Input: []Channel, []Filter
Output: []Channel (filtered)
```

1. Include filters narrow down to matching channels
2. Exclude filters remove unwanted channels

### Stage 5: Numbering

Assigns channel numbers based on mode:

| Mode | Behavior |
|------|----------|
| `preserve` | Keep original numbers |
| `sequential` | Number 1, 2, 3... |
| `source_based` | Each source gets a range |

### Stage 6: Logo Caching

Downloads and caches channel logos:

```
Input: []Channel with logo URLs
Output: []Channel with cached logo paths
```

- Respects HTTP cache headers
- SHA256-addressed storage
- Skips already-cached logos

### Stage 7: Generation

Writes output files:

- `playlist.m3u8` - M3U8 format playlist
- `epg.xml` - XMLTV format guide

Files are written with streaming I/O to handle large datasets.

### Stage 8: Publish

Atomic move of generated files to final location. This ensures the proxy URL always serves complete files.

## Memory Efficiency

The pipeline uses:

- **Streaming writes** - Files written incrementally, not buffered
- **JSONL intermediate format** - Large datasets serialized to disk
- **Configurable batch sizes** - Control memory usage

## Parallelization

Some stages can run in parallel:

- Logo downloads (configurable concurrency)
- Independent source ingestions

## Error Handling

- Each stage can fail independently
- Errors are logged with context
- Failed generations don't replace existing outputs
