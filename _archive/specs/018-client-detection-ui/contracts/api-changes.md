# API Contract Changes: Client Detection UI Improvements

**Feature**: 018-client-detection-ui
**Date**: 2025-12-15

## Overview

This feature requires minimal API changes. Most work is frontend URL fixes and relay logic improvements.

## Existing Endpoints (No Changes Required)

### Export/Import Endpoints

Backend endpoints are correctly implemented. Only frontend URL construction needs fixing.

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/export/filters` | POST | Export filters |
| `/api/v1/export/data-mapping-rules` | POST | Export data mapping rules |
| `/api/v1/export/client-detection-rules` | POST | Export client detection rules |
| `/api/v1/export/encoding-profiles` | POST | Export encoding profiles |
| `/api/v1/import/filters/preview` | POST | Preview filter import |
| `/api/v1/import/filters` | POST | Execute filter import |
| `/api/v1/import/data-mapping-rules/preview` | POST | Preview data mapping import |
| `/api/v1/import/data-mapping-rules` | POST | Execute data mapping import |
| `/api/v1/import/client-detection-rules/preview` | POST | Preview client detection import |
| `/api/v1/import/client-detection-rules` | POST | Execute client detection import |
| `/api/v1/import/encoding-profiles/preview` | POST | Preview encoding profile import |
| `/api/v1/import/encoding-profiles` | POST | Execute encoding profile import |

### Expression Validation Endpoint

Already supports client detection domain:

```
POST /api/v1/expressions/validate?domain=client_detection
```

### Client Detection Rules Endpoints

All existing endpoints remain unchanged:

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/client-detection-rules` | GET | List rules (supports `is_system` filter) |
| `/api/v1/client-detection-rules` | POST | Create rule |
| `/api/v1/client-detection-rules/{id}` | GET | Get rule by ID |
| `/api/v1/client-detection-rules/{id}` | PUT | Update rule |
| `/api/v1/client-detection-rules/{id}` | DELETE | Delete rule (blocked for system rules) |
| `/api/v1/client-detection-rules/test` | POST | Test expression against user agent |

## New/Enhanced Endpoints

### 1. Client Detection Helpers Endpoint

**Purpose**: Provide available helpers for client detection expressions

**Endpoint**: `GET /api/v1/client-detection-rules/helpers`

**Response**:
```json
{
  "success": true,
  "helpers": [
    {
      "name": "dynamic",
      "prefix": "@dynamic(",
      "description": "Access request data dynamically",
      "example": "@dynamic(request.headers, user-agent)",
      "completion": {
        "type": "static",
        "options": [
          {
            "label": "request.headers",
            "value": "request.headers",
            "description": "HTTP request headers"
          },
          {
            "label": "request.query",
            "value": "request.query",
            "description": "URL query parameters"
          },
          {
            "label": "request.path",
            "value": "request.path",
            "description": "URL path segments"
          }
        ]
      }
    }
  ]
}
```

**Note**: This can be implemented as a static endpoint or the frontend can hardcode the helper configuration.

### 2. Client Detection Fields Endpoint

**Purpose**: Provide available fields for client detection expressions

**Endpoint**: `GET /api/v1/client-detection-rules/fields`

**Response**:
```json
{
  "success": true,
  "fields": [
    {
      "name": "user_agent",
      "description": "HTTP User-Agent header value",
      "type": "string",
      "aliases": ["ua"]
    }
  ]
}
```

### 3. Enhanced Channel Search (Existing Endpoint)

**Endpoint**: `GET /api/v1/channels`

**Existing Parameters** (no changes):
- `page` (int, default: 1)
- `limit` (int, default: 50, max: 500)
- `search` (string) - LIKE pattern matching
- `source_id` (string)
- `group` (string)
- `sort_by` (string)
- `sort_order` (string)

**Backend Enhancement**: Expand search to include more fields

Current:
```sql
WHERE channel_name LIKE ? OR tvg_name LIKE ? OR tvg_id LIKE ?
```

Enhanced:
```sql
WHERE channel_name LIKE ?
   OR tvg_name LIKE ?
   OR tvg_id LIKE ?
   OR group_title LIKE ?
   OR channel_number LIKE ?
   OR ext_id LIKE ?
```

**Note**: Fuzzy matching will be handled client-side using Fuse.js

### 4. Enhanced EPG Search (Existing Endpoint)

**Endpoint**: `GET /api/v1/epg/search`

**Existing Parameters** (no changes):
- `q` (string, required, min: 2)
- `page` (int, default: 1)
- `limit` (int, default: 50, max: 200)
- `category` (string, optional)
- `on_air` (bool, optional)

**Backend Enhancement**: Already searches title, sub_title, description. Add channel_id:

Current:
```sql
WHERE title LIKE ? OR sub_title LIKE ? OR description LIKE ?
```

Enhanced:
```sql
WHERE title LIKE ?
   OR sub_title LIKE ?
   OR description LIKE ?
   OR channel_id LIKE ?
   OR category LIKE ?
```

## Frontend API Client Fixes

### Current (Incorrect)

```typescript
// api-client.ts lines 1195-1233
async exportFilters(request: ExportRequest) {
  return this.request(`${API_CONFIG.endpoints.filters}/export`, ...);
  // Generates: /api/v1/filters/export (WRONG)
}

async importFiltersPreview(file: File) {
  return this.request(`${API_CONFIG.endpoints.filters}/import?preview=true`, ...);
  // Generates: /api/v1/filters/import?preview=true (WRONG)
}

async importFilters(file: File, ...) {
  return this.request(`${API_CONFIG.endpoints.filters}/import`, ...);
  // Generates: /api/v1/filters/import (WRONG)
}
```

### Fixed

```typescript
// api-client.ts - Fixed export/import URLs
async exportFilters(request: ExportRequest) {
  return this.request('/api/v1/export/filters', ...);
}

async exportDataMappingRules(request: ExportRequest) {
  return this.request('/api/v1/export/data-mapping-rules', ...);
}

async exportClientDetectionRules(request: ExportRequest) {
  return this.request('/api/v1/export/client-detection-rules', ...);
}

async exportEncodingProfiles(request: ExportRequest) {
  return this.request('/api/v1/export/encoding-profiles', ...);
}

async importFiltersPreview(file: File) {
  return this.request('/api/v1/import/filters/preview', ...);
}

async importFilters(file: File, conflicts, bulkResolution) {
  return this.request('/api/v1/import/filters', ...);
}

// Same pattern for data-mapping-rules, client-detection-rules, encoding-profiles
```

## Error Responses

All endpoints follow the existing error response pattern:

```json
{
  "success": false,
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Expression validation failed",
    "details": {
      "field": "expression",
      "errors": ["Invalid operator at position 15"]
    }
  }
}
```

### System Rule Deletion Error

```json
{
  "success": false,
  "error": {
    "code": "FORBIDDEN",
    "message": "System rules cannot be deleted",
    "details": {
      "rule_id": "01HXYZ...",
      "is_system": true
    }
  }
}
```

## OpenAPI Specification Additions

### Helper Response Schema

```yaml
components:
  schemas:
    HelperCompletion:
      type: object
      properties:
        type:
          type: string
          enum: [static, search, function]
        options:
          type: array
          items:
            $ref: '#/components/schemas/CompletionOption'

    CompletionOption:
      type: object
      properties:
        label:
          type: string
        value:
          type: string
        description:
          type: string

    Helper:
      type: object
      properties:
        name:
          type: string
        prefix:
          type: string
        description:
          type: string
        example:
          type: string
        completion:
          $ref: '#/components/schemas/HelperCompletion'

    HelpersResponse:
      type: object
      properties:
        success:
          type: boolean
        helpers:
          type: array
          items:
            $ref: '#/components/schemas/Helper'
```

## Summary

| Change Type | Endpoint | Description |
|-------------|----------|-------------|
| Frontend Fix | Export/Import | Fix URL construction in api-client.ts |
| Backend Enhancement | `/api/v1/channels` | Expand LIKE search to more fields |
| Backend Enhancement | `/api/v1/epg/search` | Add channel_id and category to search |
| New Endpoint | `/api/v1/client-detection-rules/helpers` | Return available helpers (optional, can be frontend-only) |
| New Endpoint | `/api/v1/client-detection-rules/fields` | Return available fields |
