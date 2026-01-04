# Data Model: Codebase Cleanup & Migration Compaction

**Feature**: 010-codebase-cleanup
**Date**: 2025-12-07

## Overview

This is a refactoring feature - **no new entities are introduced**. This document captures the existing data model that must be preserved through migration compaction.

## Entities Preserved (17 Tables)

### Core Entities

#### StreamSource
- **Purpose**: Upstream M3U/Xtream stream sources
- **Key Fields**: id, name, source_type, url, username, password, enabled
- **Relationships**: Has many Channels

#### Channel
- **Purpose**: Parsed channels from stream sources
- **Key Fields**: id, source_id, ext_id, channel_name, group_title, stream_url, logo_url
- **Constraints**: Composite unique index on (source_id, ext_id)
- **Relationships**: Belongs to StreamSource

#### ManualStreamChannel
- **Purpose**: User-defined channels for manual sources
- **Key Fields**: id, source_id, channel_name, stream_url

#### EpgSource
- **Purpose**: EPG program guide sources (XMLTV, Xtream)
- **Key Fields**: id, name, source_type, url, enabled, original_timezone, time_offset, api_method
- **Relationships**: Has many EpgPrograms

#### EpgProgram
- **Purpose**: Individual TV program entries
- **Key Fields**: id, source_id, channel_id, title, description, start_time, end_time
- **Relationships**: Belongs to EpgSource

### Proxy Entities

#### StreamProxy
- **Purpose**: Proxy configurations combining sources and filters
- **Key Fields**: id, name, slug, proxy_mode (direct/smart), relay_profile_id, hls_collapse
- **Relationships**: Has many ProxySources, ProxyEpgSources, ProxyFilters, ProxyMappingRules

#### ProxySource (Join Table)
- **Purpose**: Links proxies to stream sources
- **Key Fields**: id, proxy_id, source_id, priority

#### ProxyEpgSource (Join Table)
- **Purpose**: Links proxies to EPG sources
- **Key Fields**: id, proxy_id, epg_source_id, priority

#### ProxyFilter (Join Table)
- **Purpose**: Links proxies to filters with ordering
- **Key Fields**: id, proxy_id, filter_id, priority

#### ProxyMappingRule (Join Table)
- **Purpose**: Links proxies to data mapping rules with ordering
- **Key Fields**: id, proxy_id, mapping_rule_id, priority

### Rule Entities

#### Filter
- **Purpose**: Expression-based stream/EPG filtering
- **Key Fields**: id, name, source_type, action (include/exclude), expression, priority, is_enabled, is_system
- **System Records**: 2 default filters

#### DataMappingRule
- **Purpose**: Expression-based field transformation
- **Key Fields**: id, name, source_type, expression, priority, stop_on_match, is_enabled, is_system
- **System Records**: 1 default rule

### Relay Entities

#### RelayProfile
- **Purpose**: Transcoding configuration profiles
- **Key Fields**: id, name, description, is_default, enabled, is_system
- **Video Fields**: video_codec, video_bitrate, video_preset, video_profile, force_video_transcode
- **Audio Fields**: audio_codec, audio_bitrate, audio_sample_rate, audio_channels, force_audio_transcode
- **Format Fields**: container_format, segment_duration, playlist_size
- **HW Accel Fields**: hw_accel, fallback_enabled, hw_accel_output_format, hw_accel_decoder_codec, gpu_index
- **Custom Fields**: custom_input_flags, custom_output_flags, custom_flags_validated, custom_flags_warnings
- **Stats Fields**: success_count, failure_count, last_used_at, last_error_at, last_error_msg
- **System Records**: 6 default profiles

#### RelayProfileMapping
- **Purpose**: Client detection rules for automatic codec selection
- **Key Fields**: id, name, description, priority, expression, is_enabled, is_system
- **Codec Fields**: accepted_video_codecs, accepted_audio_codecs, accepted_containers
- **Preference Fields**: preferred_video_codec, preferred_audio_codec, preferred_container
- **System Records**: 23 client detection rules

### Scheduling Entities

#### Job
- **Purpose**: Active job queue for scheduling
- **Key Fields**: id, job_type, status, owner_type, owner_id, scheduled_at, started_at

#### JobHistory
- **Purpose**: Historical job execution records
- **Key Fields**: id, job_type, status, owner_type, owner_id, started_at, completed_at, error_message, details

### Cache Entities

#### LastKnownCodec
- **Purpose**: FFprobe codec cache for streams
- **Key Fields**: id, stream_url_hash, video_codec, audio_codec, detected_at

## System Data (Seed Records)

### Filters (is_system=true)
1. "Include All Valid Stream URLs"
2. "Exclude Adult Content"

### Data Mapping Rules (is_system=true)
1. "Default Timeshift Detection (Regex)"

### Relay Profiles (is_system=true)
1. "Automatic" (is_default=true)
2. "h264/AAC"
3. "h265/AAC"
4. "Passthrough"
5. "VP9/Opus"
6. "AV1/Opus"

### Relay Profile Mappings (is_system=true)
23 client detection rules organized by priority:
- 10-13: Browsers (Safari, Chrome, Edge, Firefox)
- 20-22: Media Players (VLC, MPV, ffmpeg)
- 30-33: Media Servers (Jellyfin, Plex, Emby, Kodi)
- 40-45: Streaming Devices (Android TV, Roku, Apple TV, Fire TV, Tizen, webOS)
- 50-51: Mobile (iOS, Android)
- 60-62: IPTV Apps (TiviMate, IPTV Smarters, GSE)
- 999: Default (Universal) fallback

## Migration Compaction Impact

### Before: 34 Migrations
- Schema creation scattered across migrations
- Data manipulation interspersed with schema changes
- Column renames and additions in separate migrations
- Profile evolution through 12 different migrations

### After: 2 Migrations
- **Migration 001**: Create all tables with final schema (GORM AutoMigrate)
- **Migration 002**: Insert all system data (32 records total)

### Schema Preservation Checklist
- [x] All 17 tables defined
- [x] Composite unique index on channels(source_id, ext_id)
- [x] proxy_mode values: direct, smart (not legacy redirect/proxy/relay)
- [x] Codec values: abstract types (h264, h265, vp9, av1, copy, auto, none)
- [x] All is_system columns present
- [x] All foreign key relationships maintained
- [x] All seed data preserved
