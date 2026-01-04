# API Contracts: Frontend Theme Polish

**Feature**: 011-frontend-theme-polish
**Date**: 2025-12-08

## Endpoints

### GET /api/v1/themes

List all available themes (built-in and custom).

**Request**:
```http
GET /api/v1/themes HTTP/1.1
Accept: application/json
```

**Response** (200 OK):
```json
{
  "themes": [
    {
      "id": "graphite",
      "name": "Graphite",
      "description": "A modern dark gray theme with clean lines",
      "source": "builtin",
      "colors": {
        "light": {
          "background": "oklch(0.9551 0 0)",
          "foreground": "oklch(0.3211 0 0)",
          "primary": "oklch(0.4891 0 0)"
        },
        "dark": {
          "background": "oklch(0.2178 0 0)",
          "foreground": "oklch(0.8853 0 0)",
          "primary": "oklch(0.7058 0 0)"
        }
      }
    },
    {
      "id": "my-custom-theme",
      "name": "My Custom Theme",
      "description": "",
      "source": "custom",
      "modified_at": "2024-12-08T10:30:00Z",
      "colors": {
        "light": {
          "background": "oklch(0.95 0.01 250)",
          "foreground": "oklch(0.25 0.02 250)",
          "primary": "oklch(0.55 0.15 250)"
        },
        "dark": {
          "background": "oklch(0.20 0.01 250)",
          "foreground": "oklch(0.90 0.01 250)",
          "primary": "oklch(0.65 0.15 250)"
        }
      }
    }
  ],
  "default": "graphite"
}
```

**Error Responses**:
- 500 Internal Server Error: Failed to read theme directory

---

### GET /api/v1/themes/{themeId}.css

Serve a theme CSS file.

**Request**:
```http
GET /api/v1/themes/graphite.css HTTP/1.1
Accept: text/css
```

**Response** (200 OK):
```http
HTTP/1.1 200 OK
Content-Type: text/css; charset=utf-8
Cache-Control: public, max-age=3600, must-revalidate
ETag: "builtin-graphite"
Last-Modified: Mon, 08 Dec 2024 00:00:00 GMT

:root {
  --background: oklch(0.9551 0 0);
  --foreground: oklch(0.3211 0 0);
  /* ... rest of theme CSS ... */
}

.dark {
  --background: oklch(0.2178 0 0);
  /* ... */
}
```

**Response** (200 OK, custom theme):
```http
HTTP/1.1 200 OK
Content-Type: text/css; charset=utf-8
Cache-Control: public, max-age=3600, must-revalidate
ETag: "mtime-1733650200"
Last-Modified: Sun, 08 Dec 2024 10:30:00 GMT

/* Custom theme CSS content */
```

**Error Responses**:
- 404 Not Found: Theme does not exist
- 400 Bad Request: Invalid theme ID format

---

## Data Types (OpenAPI Schema)

```yaml
openapi: 3.0.3
info:
  title: Tvarr Theme API
  version: 1.0.0

components:
  schemas:
    ThemeSource:
      type: string
      enum: [builtin, custom]
      description: Indicates whether theme is built-in or user-provided

    ThemePalette:
      type: object
      required: [background, foreground, primary]
      properties:
        background:
          type: string
          description: Background color value (CSS)
        foreground:
          type: string
          description: Foreground/text color value (CSS)
        primary:
          type: string
          description: Primary accent color value (CSS)
        secondary:
          type: string
          description: Secondary color value (CSS)
        accent:
          type: string
          description: Accent color value (CSS)

    ThemeColors:
      type: object
      required: [light, dark]
      properties:
        light:
          $ref: '#/components/schemas/ThemePalette'
        dark:
          $ref: '#/components/schemas/ThemePalette'

    Theme:
      type: object
      required: [id, name, source]
      properties:
        id:
          type: string
          description: Unique theme identifier (filename without .css)
          pattern: ^[a-zA-Z0-9_-]+$
        name:
          type: string
          description: Human-readable theme name
        description:
          type: string
          description: Optional theme description
        source:
          $ref: '#/components/schemas/ThemeSource'
        modified_at:
          type: string
          format: date-time
          description: File modification time (custom themes only)
        colors:
          $ref: '#/components/schemas/ThemeColors'

    ThemeListResponse:
      type: object
      required: [themes, default]
      properties:
        themes:
          type: array
          items:
            $ref: '#/components/schemas/Theme'
        default:
          type: string
          description: ID of the default theme

paths:
  /api/v1/themes:
    get:
      summary: List all available themes
      operationId: listThemes
      responses:
        '200':
          description: List of themes
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ThemeListResponse'
        '500':
          description: Server error

  /api/v1/themes/{themeId}.css:
    get:
      summary: Get theme CSS file
      operationId: getThemeCSS
      parameters:
        - name: themeId
          in: path
          required: true
          schema:
            type: string
            pattern: ^[a-zA-Z0-9_-]+$
      responses:
        '200':
          description: Theme CSS content
          content:
            text/css:
              schema:
                type: string
          headers:
            Cache-Control:
              schema:
                type: string
            ETag:
              schema:
                type: string
            Last-Modified:
              schema:
                type: string
        '400':
          description: Invalid theme ID
        '404':
          description: Theme not found
```

## Caching Behavior

### Built-in Themes
- **ETag**: `"builtin-{themeId}"`
- **Cache-Control**: `public, max-age=86400` (24 hours)
- Built-in themes never change at runtime

### Custom Themes
- **ETag**: `"mtime-{unix_timestamp}"`
- **Cache-Control**: `public, max-age=3600, must-revalidate`
- ETag changes when file is modified
- Browser will revalidate after 1 hour

## Backward Compatibility

The existing `/themes/themes.json` and `/themes/{themeId}.css` static file endpoints will continue to work for built-in themes. The new API endpoint provides:
1. Unified access to both built-in and custom themes
2. Proper caching headers for custom themes
3. Color extraction for theme previews

Frontend should prefer the new API but fall back to static files if API unavailable.
