# API Contracts: Codebase Cleanup & Migration Compaction

**Feature**: 010-codebase-cleanup
**Date**: 2025-12-07

## No Contract Changes

This is a refactoring feature - **no API changes are introduced**.

### Preserved Contracts

All existing HTTP API endpoints remain unchanged:

#### Sources API
- `GET /api/v1/sources` - List all sources
- `GET /api/v1/stream-sources` - List stream sources
- `POST /api/v1/stream-sources` - Create stream source
- `GET /api/v1/stream-sources/{id}` - Get stream source
- `PUT /api/v1/stream-sources/{id}` - Update stream source
- `DELETE /api/v1/stream-sources/{id}` - Delete stream source
- `GET /api/v1/epg-sources` - List EPG sources
- `POST /api/v1/epg-sources` - Create EPG source
- `GET /api/v1/epg-sources/{id}` - Get EPG source
- `PUT /api/v1/epg-sources/{id}` - Update EPG source
- `DELETE /api/v1/epg-sources/{id}` - Delete EPG source

#### Channels API
- `GET /api/v1/channels` - List channels
- `GET /api/v1/channels/{id}` - Get channel

#### EPG API
- `GET /api/v1/epg` - Get EPG programs
- `GET /api/v1/epg/guide` - Get EPG guide data

#### Proxy API
- `GET /api/v1/proxies` - List proxies
- `POST /api/v1/proxies` - Create proxy
- `GET /api/v1/proxies/{id}` - Get proxy
- `PUT /api/v1/proxies/{id}` - Update proxy
- `DELETE /api/v1/proxies/{id}` - Delete proxy

#### Filters API
- `GET /api/v1/filters` - List filters
- `POST /api/v1/filters` - Create filter
- `GET /api/v1/filters/{id}` - Get filter
- `PUT /api/v1/filters/{id}` - Update filter
- `DELETE /api/v1/filters/{id}` - Delete filter
- `POST /api/v1/filters/test` - Test filter expression

#### Data Mapping API
- `GET /api/v1/data-mapping` - List rules
- `POST /api/v1/data-mapping` - Create rule
- `GET /api/v1/data-mapping/{id}` - Get rule
- `PUT /api/v1/data-mapping/{id}` - Update rule
- `DELETE /api/v1/data-mapping/{id}` - Delete rule
- `POST /api/v1/data-mapping/test` - Test mapping expression

#### Relay Profiles API
- `GET /api/v1/relay-profiles` - List profiles
- `POST /api/v1/relay-profiles` - Create profile
- `GET /api/v1/relay-profiles/{id}` - Get profile
- `PUT /api/v1/relay-profiles/{id}` - Update profile
- `DELETE /api/v1/relay-profiles/{id}` - Delete profile

#### Relay Profile Mappings API
- `GET /api/v1/relay-profile-mappings` - List mappings
- `POST /api/v1/relay-profile-mappings` - Create mapping
- `GET /api/v1/relay-profile-mappings/{id}` - Get mapping
- `PUT /api/v1/relay-profile-mappings/{id}` - Update mapping
- `DELETE /api/v1/relay-profile-mappings/{id}` - Delete mapping

#### Expression API
- `POST /api/v1/expressions/validate` - Validate expression
- `GET /api/v1/expressions/fields` - Get available fields
- `GET /api/v1/expressions/helpers` - Get available helpers

#### Config API
- `GET /api/v1/config` - Get all config
- `PUT /api/v1/config` - Update config
- `POST /api/v1/config/persist` - Save config to file

#### Health API
- `GET /livez` - Liveness probe
- `GET /readyz` - Readiness probe
- `GET /health` - Detailed health metrics

#### Output API
- `GET /api/v1/output/{slug}/playlist.m3u` - M3U playlist
- `GET /api/v1/output/{slug}/epg.xml` - XMLTV EPG

#### Relay Streaming API
- `GET /api/v1/relay/{slug}/{channelID}` - Stream channel

## Internal Interface Changes

### Methods to Remove

1. **`RelayProfileMappingRepository.GetEnabledByPriority`**
   - Duplicate of `GetEnabled` - remove from interface
   - Update 3 call sites to use `GetEnabled`

### Methods Unchanged

All other repository and service interfaces remain unchanged.

## Validation Approach

After cleanup:
1. All E2E tests pass
2. Frontend continues to work without modification
3. API response shapes unchanged
4. No breaking changes to external consumers
