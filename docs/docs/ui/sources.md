---
title: Sources
description: Managing stream and EPG sources
sidebar_position: 2
---

# Sources

The Sources section is where you add your M3U playlists and EPG data.

## Stream Sources

**Sources > Streams** manages your channel playlists.

### Adding a Source

Click **Add Source** and choose:

- **M3U URL** - Enter a URL to an M3U/M3U8 playlist
- **M3U File** - Upload a playlist file
- **Xtream** - Enter Xtream Codes credentials
- **Manual** - Create channels manually

### Source Options

| Option | Description |
|--------|-------------|
| Name | Display name for the source |
| URL/Credentials | Location of the playlist |
| Schedule | Cron expression for auto-ingestion |
| Auto-generate | Regenerate proxies after ingestion |

### Actions

- **Ingest** - Pull latest channels from source
- **Edit** - Modify source settings
- **Delete** - Remove source and its channels

## EPG Sources

**Sources > EPG** manages your program guide data.

### Adding EPG

Click **Add Source** and choose:

- **XMLTV URL** - URL to XMLTV guide
- **XMLTV File** - Upload an XMLTV file
- **Xtream** - Use same Xtream credentials

### EPG Options

| Option | Description |
|--------|-------------|
| Name | Display name |
| URL/Credentials | Location of EPG data |
| Schedule | Auto-ingestion schedule |
| Days | How many days of guide to import |

## Ingestion Status

Each source shows:

- Last ingestion time
- Channel/program count
- Any errors from last ingestion

Click a source to see detailed ingestion history.
