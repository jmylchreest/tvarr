---
title: Workflow
description: How data flows through tvarr
sidebar_position: 1
---

# Workflow

How tvarr processes your streams from source to player.

## Pipeline Stages

When you click **Generate** on a proxy, tvarr runs this pipeline:

```
1. Load Channels     ─▶ Pull channels from all linked stream sources
2. Load Programs     ─▶ Pull EPG data from all linked EPG sources
3. Data Mapping      ─▶ Apply transformation rules (rename, fix logos, etc.)
4. Filtering         ─▶ Include/exclude channels based on filter rules
5. Numbering         ─▶ Assign channel numbers
6. Logo Caching      ─▶ Download and cache logos locally
7. Generation        ─▶ Write M3U8 and XMLTV files
8. Publish           ─▶ Make files available at proxy URLs
```

## Automatic vs Manual

### Scheduled Ingestion

Sources can be set to ingest on a schedule:

- **Cron expression** - Standard cron format (e.g., `0 4 * * *` for 4am daily)
- **After ingestion** - Automatically regenerate linked proxies

### Manual Triggers

- Click **Ingest** on a source to pull latest data
- Click **Generate** on a proxy to rebuild its playlist
- Use the API for automation: `POST /api/v1/jobs/trigger/proxy/{id}`

## Data Flow Example

```
M3U Source "Provider A"     EPG Source "xmltv.se"
        │                           │
        ▼                           ▼
   ┌─────────┐               ┌─────────┐
   │ 500 ch  │               │  Guide  │
   └────┬────┘               └────┬────┘
        │                         │
        └──────────┬──────────────┘
                   ▼
            Filter: "Sports"
            (50 channels match)
                   │
                   ▼
            Data Mapping:
            - Fix logos
            - Clean names
                   │
                   ▼
            Proxy "Sports TV"
            - playlist.m3u8
            - epg.xml
```
