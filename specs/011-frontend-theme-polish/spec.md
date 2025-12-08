# Feature Specification: Frontend Theme Polish

**Feature Branch**: `011-frontend-theme-polish`
**Created**: 2025-12-08
**Status**: Draft
**Input**: User description: "Frontend theme polish: fix page navigation flashbang, consistent component styling, custom theme file support"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Seamless Dark Mode Navigation (Priority: P1)

As a user navigating between pages, I want the interface to maintain my chosen theme (typically dark mode) without any bright white flashes or visual disruptions, so that my viewing experience remains comfortable and consistent.

**Why this priority**: The "flashbang" effect when navigating pages in dark mode is the most disruptive user experience issue. It causes eye strain and breaks immersion, making the application feel unpolished.

**Independent Test**: Can be fully tested by navigating between any two pages while in dark mode - success is measured by absence of any white/bright background flash during the transition.

**Acceptance Scenarios**:

1. **Given** the user has dark mode enabled, **When** they navigate from the Dashboard to Channels page, **Then** the background remains dark throughout the transition with no visible white flash.
2. **Given** the user has dark mode enabled and navigates rapidly between multiple pages, **When** clicking through Dashboard, Sources, Proxies in quick succession, **Then** no white flashes occur during any transition.
3. **Given** the user refreshes the page (F5) while in dark mode, **When** the page reloads, **Then** the dark theme is applied before any content becomes visible.
4. **Given** the application is loading for the first time, **When** the user has dark mode as their system preference, **Then** the page renders with dark background from the first paint.

---

### User Story 2 - Custom Theme File Support (Priority: P2)

As a power user who wants to personalize my interface, I want to upload custom CSS theme files to a designated directory and have them automatically appear in the theme selector, so that I can customize the application's appearance to match my preferences.

**Why this priority**: Custom themes enable personalization and community theme sharing, adding value for power users who want to customize their experience beyond the built-in themes.

**Independent Test**: Can be tested by placing a properly formatted CSS theme file in the data/themes directory and verifying it appears in the theme selector dropdown.

**Acceptance Scenarios**:

1. **Given** a valid CSS theme file exists in the `$DATA/themes/` directory, **When** the user opens the theme selector, **Then** the custom theme appears in the list with its name derived from the filename.
2. **Given** a custom theme file named `my-custom-theme.css` is in the themes directory, **When** the user selects it from the dropdown, **Then** the theme is applied correctly to all UI elements.
3. **Given** an invalid or malformed CSS file is placed in the themes directory, **When** the theme list is loaded, **Then** the invalid file is skipped without causing errors and built-in themes remain functional.
4. **Given** a custom theme is currently active, **When** the user removes the theme file from the directory and refreshes, **Then** the application falls back to the default theme gracefully.

---

### User Story 3 - Consistent Component Styling (Priority: P3)

As a user interacting with various UI elements, I want all components (buttons, inputs, cards, dialogs) to have consistent sizes, spacing, and visual styling across all pages, so that the interface feels cohesive and professional.

**Why this priority**: Visual consistency improves perceived quality and usability. While not as immediately disruptive as the flashbang issue, inconsistent styling creates a sense of unfinished work.

**Independent Test**: Can be tested by visually auditing component styles across multiple pages to verify buttons, cards, inputs maintain identical sizing and styling.

**Acceptance Scenarios**:

1. **Given** any page with a primary action button, **When** compared to primary buttons on other pages, **Then** they have identical padding, font size, border radius, and color.
2. **Given** input fields on the Settings page, **When** compared to input fields on the Sources page, **Then** they have identical height, border style, and focus states.
3. **Given** card components used on Dashboard, **When** compared to cards on other pages, **Then** they have consistent padding, shadow, and border radius.
4. **Given** dialog/modal windows on any page, **When** opened, **Then** they have consistent styling including backdrop blur, padding, and animation.

---

### User Story 4 - Intuitive Theme Management UI (Priority: P4)

As a user wanting to change my theme, I want the theme selector to clearly show available themes with visual previews of their color schemes, so that I can quickly find and select a theme that suits my preference.

**Why this priority**: While functional theme switching exists, improving the theme selector UI enhances discoverability of custom themes and makes the feature more accessible.

**Independent Test**: Can be tested by opening the theme selector and verifying each theme shows a visual preview of its color palette before selection.

**Acceptance Scenarios**:

1. **Given** the theme selector is open, **When** viewing the available themes, **Then** each theme shows a color swatch preview representing its primary colors.
2. **Given** custom themes exist in the themes directory, **When** viewing the theme selector, **Then** custom themes are visually distinguished from built-in themes (e.g., "Custom" label).
3. **Given** a theme is selected, **When** hovering over other themes, **Then** no preview is shown (to avoid confusion) but the color swatches are visible.

---

### Edge Cases

- What happens when a user's stored theme preference references a theme file that no longer exists?
  - The system falls back to the default theme (graphite) and clears the invalid preference.

- How does the system handle theme files with duplicate names (same name as built-in theme)?
  - Custom themes with names matching built-in themes are ignored to prevent confusion.

- What happens when the $DATA/themes directory doesn't exist?
  - The backend creates the directory on startup if it doesn't exist; only built-in themes are shown.

- How are theme file updates handled (user edits a custom theme file)?
  - Changes are detected on page refresh or when the theme selector is opened.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST apply the user's theme preference before any visible content renders (no FOUC - Flash of Unstyled Content).
- **FR-002**: System MUST maintain the selected theme during client-side navigation without any visible background color changes.
- **FR-003**: System MUST load custom theme CSS files from the `$DATA/themes/` directory on the backend.
- **FR-004**: System MUST provide an endpoint to list available themes including both built-in and custom themes.
- **FR-005**: System MUST validate custom theme files by checking for required CSS variables (--background, --foreground, --primary at minimum) before including them in the available themes list.
- **FR-006**: System MUST ensure all shadcn/ui components use theme CSS variables consistently.
- **FR-007**: System MUST persist theme preference in browser local storage.
- **FR-008**: System MUST support light/dark/system mode toggle independently from theme color palette selection.
- **FR-009**: System MUST gracefully handle missing or invalid theme files without crashing.
- **FR-010**: System MUST display visual color previews for each theme in the theme selector.
- **FR-011**: System MUST serve custom theme CSS with browser-cacheable headers and file modification time-based cache invalidation to ensure edits are detected on refresh.

### Key Entities

- **Theme**: Represents a color palette with id, name, description, light mode colors, dark mode colors, and source (built-in vs custom).
- **User Preference**: Stores theme id and mode (light/dark/system) in browser local storage.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Zero white flashes occur during any page navigation when using dark mode (100% of navigation events).
- **SC-002**: First contentful paint shows correct theme colors without any color shift for returning users.
- **SC-003**: Custom theme files placed in `$DATA/themes/` appear in the theme selector within one page refresh.
- **SC-004**: All primary buttons across the application have identical visual styling (verifiable through visual regression testing).
- **SC-005**: Theme selector loads and displays theme previews within 500ms of opening.
- **SC-006**: Application remains functional even when all custom theme files are malformed or missing.

## Clarifications

### Session 2025-12-08

- Q: What validation is sufficient for custom theme CSS files? → A: Required variables check (must define --background, --foreground, --primary at minimum)
- Q: How should custom theme CSS files be cached? → A: Browser cache with file modification time-based invalidation

## Assumptions

- The existing shadcn/ui component library and Tailwind CSS configuration will be retained.
- The current theme structure (CSS custom properties with :root and .dark selectors) will be maintained.
- The backend has access to a writable `$DATA` directory for user content.
- Custom themes must follow the existing CSS variable naming convention to work correctly.
- The inline script approach for preventing FOUC is acceptable from a CSP (Content Security Policy) perspective.
