# project-overview.html Style Guide

For people editing [project-overview.html](project-overview.html). Not a user guide.

Design decisions and conventions for the tinycode project overview page. Follow this guide when updating or extending the page to maintain visual consistency.

## 1. Color Palette

All colors are defined as CSS custom properties on `:root`. Never use raw hex values in components -- always reference the variable.

### Backgrounds

| Variable | Hex | Usage |
|----------|-----|-------|
| `--bg-deep` | `#0a0e14` | Page background, scrollbar track |
| `--bg-surface` | `#131922` | Card backgrounds, nav, code headers, flow diagram containers |
| `--bg-elevated` | `#1a2233` | Hover states, active tabs, expanded cards |
| `--bg-code` | `#0d1117` | Code blocks, terminal body, architecture detail panel |

### Borders

| Variable | Hex | Usage |
|----------|-----|-------|
| `--border` | `#1e2937` | Default border for cards, sections, dividers |
| `--border-bright` | `#2d3a4a` | Hover borders, dimmed flow step markers, connector lines |

### Text

| Variable | Hex | Usage |
|----------|-----|-------|
| `--text-primary` | `#c5cdd9` | Headings, names, primary content |
| `--text-secondary` | `#7d8a9a` | Descriptions, paragraphs, secondary labels |
| `--text-muted` | `#4a5568` | Captions, timestamps, disabled states, terminal comments |

### Accents

| Variable | Hex | Usage |
|----------|-----|-------|
| `--accent-green` | `#39d353` | Primary accent -- logo, badges, active states, CTA, step numbers |
| `--accent-green-dim` | `#26a641` | Reserved for subtle green variants |
| `--accent-blue` | `#58a6ff` | UI layer nodes, code references, Tool badges, explore/architect agents |
| `--accent-amber` | `#f0883e` | Extensions layer nodes, Hook badges, code-reviewer/git-master agents |
| `--accent-red` | `#f85149` | Error states, debugger/critic agents, terminal dot (close) |
| `--accent-purple` | `#bc8cff` | Infrastructure layer nodes, Planned badges, scientist/writer agents |

### Glow Effects

| Variable | Value | Usage |
|----------|-------|-------|
| `--glow-green` | `0 0 20px rgba(57,211,83,0.15), 0 0 40px rgba(57,211,83,0.05)` | Logo, selected architecture nodes, CTA hover |
| `--glow-blue` | `0 0 20px rgba(88,166,255,0.15)` | Reserved |
| `--glow-amber` | `0 0 20px rgba(240,136,62,0.15)` | Reserved |

## 2. Typography

### Font Stacks

| Variable | Stack | Usage |
|----------|-------|-------|
| `--font-mono` | `"Berkeley Mono", "SF Mono", "Fira Code", "Cascadia Code", "JetBrains Mono", "Menlo", monospace` | Navigation, labels, code, badges, architecture nodes, all mono-spaced UI elements |
| `--font-sans` | `-apple-system, "Segoe UI", system-ui, sans-serif` | Body text, descriptions, paragraphs |

### Size Scale

**Minimum readable size: 0.8rem.** Nothing in the page should be set below this.

| Size | Usage |
|------|-------|
| `clamp(2.4rem, 6vw, 3.8rem)` | Hero h1 |
| `clamp(1.5rem, 4vw, 2.2rem)` | Section titles |
| `1.4rem` | Tech stack icons |
| `1.2rem` | Hero tagline, wire protocol arrows |
| `1.15rem` | Plugin subsection titles |
| `1.1rem` | Nav logo |
| `1.05rem` | Section descriptions |
| `1rem` | Body text baseline |
| `0.95rem` | Architecture detail title, getting started step numbers |
| `0.92rem` | Getting started step titles |
| `0.9rem` | Card descriptions (pkg, plugin, agent, tech, flow step, arch detail), tech icon inline overrides |
| `0.88rem` | Hero features, tech card names |
| `0.85rem` | Navigation links, hero CTA, search input, getting started step desc, pkg card detail values, wire step descriptions, plugin table text |
| `0.82rem` | Architecture binary label, pkg group titles, plugin category/domain titles, agent names, plugin card names, flow step titles, pkg card names |
| `0.8rem` | Labels, badges, captions, code blocks, step numbers, node tags, file refs, muted text, footer. This is the **floor** -- nothing smaller. |

### Weight Hierarchy

| Weight | Usage |
|--------|-------|
| `800` | Hero h1, getting started numbers |
| `700` | Section titles, arch detail title, step numbers, badges, pattern icons, plugin subsection titles |
| `600` | Component names, card names, tab text, group titles, category titles, wire step methods, plugin card names |
| `400` | Body text, descriptions (default) |

### Letter Spacing

| Value | Usage |
|-------|-------|
| `-0.04em` | Hero h1 |
| `-0.03em` | Section titles |
| `-0.02em` | Nav logo, agent avatars, plugin subsection titles |
| `0.01em` | CTA button |
| `0.02em` | Navigation links |
| `0.03em` | Plugin badges |
| `0.04em` | Hero badge |
| `0.05em` | Node tags |
| `0.08em` | Detail labels, code language labels, plugin category titles |
| `0.1em` | Binary label, group titles |
| `0.12em` | Layer labels |
| `0.15em` | Section labels |

## 3. Component Patterns

### Cards

Cards follow a consistent structure: `background: var(--bg-surface)`, `border: 1px solid var(--border)`, `border-radius: var(--radius)` (6px), `padding: 14-20px 18-24px`.

Hover behavior: `border-color: var(--border-bright)`, `transform: translateY(-1px)`.

Expanded state (package cards): `border-color: var(--accent-green)`, `background: var(--bg-elevated)`.

Card variants:
- **Package cards** (`pkg-card`): Clickable, expandable. Header (name + arrow), description, collapsible detail section with labels and values.
- **Agent cards** (`agent-card`): Non-expandable. Avatar + info layout with flex gap.
- **Tech cards** (`tech-card`): Non-expandable. Icon + text info layout.
- **Plugin cards** (`plugin-card`): Non-expandable. Header (name + badge), description.

### Badges

Badges use `font-family: var(--font-mono)`, `font-size: 0.8rem`, `padding: 2px 6-8px`, `border-radius: 3px`.

Badge types with their background/color:
- **Tool** (`.badge-tool`): `rgba(88,166,255,0.1)` / `--accent-blue`
- **Hook** (`.badge-hook`): `rgba(240,136,62,0.1)` / `--accent-amber`
- **Both** (`.badge-both`): `rgba(57,211,83,0.1)` / `--accent-green`
- **Planned** (`.badge-planned`): `rgba(188,140,255,0.08)` / `--accent-purple`
- **Converted** (`.badge-converted`): `rgba(57,211,83,0.08)` / `--accent-green`
- **read-only** (`.agent-badge.readonly`): `rgba(88,166,255,0.08)` / `--accent-blue`

Architecture node tags: same badge pattern but positioned absolutely at `top: -8px; right: -8px`.
- **UI** (`.tag-ui`): solid `--accent-blue` background, dark text
- **CORE** (`.tag-core`): solid `--accent-green` background, dark text
- **EXT** (`.tag-ext`): solid `--accent-amber` background, dark text
- **INFRA** (`.tag-infra`): solid `--accent-purple` background, dark text

### Interactive Nodes (Architecture Diagram)

Architecture nodes use `cursor: pointer`, `tabindex="0"`, `role="button"`, `aria-label`. On hover: green border, green background tint, upward translate, green glow. On select (`.selected`): same as hover but persistent. Click toggles a detail panel below with `aria-live="polite"`.

### Tabs (Data Flow)

Tab bar: flex container with `background: var(--bg-surface)`, `border: 1px solid var(--border)`, inner `padding: 4px`. Tabs are buttons styled with `border: none; background: none`. Active tab: `background: var(--bg-elevated); color: var(--accent-green)`.

ARIA: `role="tablist"` on container, `role="tab"` with `aria-selected` on each tab, `role="tabpanel"` on content panels, `aria-controls` linking tabs to panels.

### Flow Diagrams

Vertical step sequence with numbered circles connected by lines. Step states:
- **Active** (`.active-step`): green step number with glow and scale(1.1), green gradient line
- **Dimmed** (`.dimmed`): gray step number, used during animation
- **Default**: green step number, no glow

Each step has: number circle (28px diameter), vertical connector line (2px wide), content card with title/description/code reference.

### Red Hat Plugin Tables

Compact `<table>` layout with three columns: name (mono, 180px width), description (sans), badge (right-aligned, 80px width). Rows have bottom borders and hover highlighting. At 900px breakpoint, name column width becomes auto.

## 4. Animation

### Scroll Animations (Intersection Observer)

Elements with class `.animate-in` start with `opacity: 0; transform: translateY(24px)`. When they enter the viewport (threshold: 0.1, rootMargin: `0px 0px -40px 0px`), they receive class `.visible` which transitions to `opacity: 1; transform: translateY(0)` over 0.6s ease.

### Data Flow Auto-Play

The `playFlow(flowId)` function advances through steps every 1200ms. Each step highlights the current step circle (green glow, scale), dims future steps, and shows previous steps as completed. When all steps finish, all are shown as active. Toggle behavior: clicking Play again stops the animation.

### Hover Transitions

All interactive elements use `transition: all var(--transition)` where `--transition: 0.2s ease`. Hover effects include:
- Cards: `translateY(-1px)`, border color change
- Architecture nodes: `translateY(-2px)`, green border + glow
- CTA button: `translateY(-1px)`, lighter green, green glow
- Buttons (flow, copy): border/text color change to green

### Keyframe Animations

| Name | Duration | Usage |
|------|----------|-------|
| `fadeInUp` | 0.3s | Architecture detail panel appearance, expanded card details, tab panel transitions |
| `blink` | 1s step-end | Terminal cursor |
| `pulseGlow` | 2s ease | Hero badge dot, indicates "live" status |
| `typing` | defined but unused | Reserved for typing effect |
| `scanline` | defined but unused | Reserved for terminal scanline effect |
| `flowDot` | defined but unused | Reserved for animated flow dots |

## 5. Layout

### Container

Max-width: 1200px, centered with `margin: 0 auto`. Horizontal padding: `clamp(16px, 4vw, 40px)`.

### Sections

Vertical padding: 100px (desktop), 60px (mobile at 640px). Separated by `border-bottom: 1px solid var(--border)`.

### Grid Patterns

| Component | Grid | Min Column Width |
|-----------|------|-----------------|
| Hero | `1fr 1fr` (stacks on mobile) | - |
| Package cards | `repeat(auto-fill, minmax(260px, 1fr))` | 260px |
| Plugin cards | `repeat(auto-fill, minmax(280px, 1fr))` | 280px |
| Agent cards | `repeat(auto-fill, minmax(270px, 1fr))` | 270px |
| Tech cards | `repeat(auto-fill, minmax(340px, 1fr))` | 340px |
| Hero features | `1fr 1fr` (stacks on mobile) | - |

### Breakpoints

| Breakpoint | Changes |
|------------|---------|
| `900px` | Hero grid stacks to single column. Plugin card grid stacks. Architecture nodes wrap. Tech/agent grids single column. Wire protocol min-width reduced to 600px. |
| `640px` | Nav height reduced to 50px. Nav links hidden (hamburger menu). Section padding reduced to 60px. Hero h1 to 2rem, tagline to 1rem. Terminal font to 0.8rem. Architecture diagram padding reduced. Package/agent grids single column. Getting started steps stack vertically. Flow tabs wrap. |

### Spacing Rhythm

Consistent gap values: 4px (tabs, nav links), 8px (inline elements), 10px (card grids, architecture rows), 12px (hero features, agent cards), 14px (tech cards), 20px (flow steps, start steps, category/group spacing), 24px (architecture layers, category spacing), 32px (flow/wire diagrams), 40px (hero gap mobile), 48px (section description margin-bottom, plugin subsections), 60px (hero gap desktop).

## 6. Accessibility

### ARIA Attributes

- Navigation: `role="navigation"`, `aria-label="Main navigation"`
- Hamburger: `aria-label="Toggle navigation menu"`
- Tab system: `role="tablist"`, `role="tab"` with `aria-selected`, `role="tabpanel"`, `aria-controls`
- Architecture nodes: `tabindex="0"`, `role="button"`, `aria-label="[Component] component"`
- Architecture detail: `role="region"`, `aria-live="polite"` for dynamic content updates
- Terminal mockup: `role="img"`, `aria-label="Terminal mockup showing tinycode in action"`
- Search input: `aria-label="Filter packages by name or description"`
- Copy buttons: `aria-label="Copy code to clipboard"`

### Keyboard Navigation

Architecture nodes respond to Enter and Space key presses (same as click). All interactive elements are focusable via `tabindex="0"` where not natively focusable.

### Focus Styles

Default browser focus outlines are preserved. Interactive elements receive visual feedback via border color changes.

### Color Contrast

Text colors against `--bg-deep` (#0a0e14):
- `--text-primary` (#c5cdd9): ~10:1 contrast ratio
- `--text-secondary` (#7d8a9a): ~5:1 contrast ratio
- `--text-muted` (#4a5568): ~3:1 contrast ratio (used only for non-essential decorative text)
- `--accent-green` (#39d353): ~7:1 contrast ratio

## 7. Color Coding by Domain

Architecture layers map to specific accent colors:

| Color | Variable | Domain | Nodes |
|-------|----------|--------|-------|
| Blue | `--accent-blue` | UI & Interfaces | TUI, Web UI, ACP/IDE, CLI |
| Green | `--accent-green` | Core Engine | HTTP Server, API Client, Session, LLM Client, Tools, Permissions |
| Amber | `--accent-amber` | Extensions | Agents, Providers, Plugins, MCP, Skills |
| Purple | `--accent-purple` | Infrastructure | Event Bus, SQLite, Config, VCS/Git |

Agent avatars also use these colors to indicate the agent's general role:
- Blue: read-only analytical (architect, explore, verifier)
- Amber: active modification (code-reviewer, git-master)
- Green: execution/creation (executor, plan, test-engineer)
- Red: defensive/critical (debugger, critic, security-reviewer)
- Purple: research/writing (scientist, writer, code-simplifier)

## 8. Icons and Badges

### Pattern Badges (Plugins)

General plugins display a pattern badge indicating their integration type:
- **Tool** (blue): Plugin provides LLM-callable tools only
- **Hook** (amber): Plugin observes/reacts to lifecycle events only
- **Both** (green): Plugin provides both tools and hooks

### Status Badges

- **Planned** (purple): Red Hat plugins not yet converted to Go
- **Converted** (green): Plugins that have been ported to Go
- **read-only** (blue): Agents that cannot modify files

### Agent Avatars

Two-letter abbreviation in a colored square. Background uses the accent color at 10% opacity. Size: 36x36px, `border-radius: var(--radius)`.

## 9. Adding Content

### Adding a New Section

1. Add the section HTML between the last section and the footer:
   ```html
   <section id="new-section">
     <div class="container">
       <div class="section-label animate-in">Label Text</div>
       <h2 class="section-title animate-in">Section Title</h2>
       <p class="section-desc animate-in">Description text.</p>
       <!-- Content here -->
     </div>
   </section>
   ```
2. Add a nav link in the `<ul class="nav-links">` list.
3. The Intersection Observer automatically picks up `.animate-in` elements.

### Adding a New General Plugin Card

Add inside the appropriate `.plugin-category` > `.plugin-card-grid`:

```html
<div class="plugin-card">
  <div class="plugin-card-header">
    <span class="plugin-card-name">plugin-name</span>
    <span class="plugin-badge badge-tool">Tool</span>  <!-- or badge-hook / badge-both -->
  </div>
  <div class="plugin-card-desc">One-line description of what the plugin does</div>
</div>
```

### Adding a New Red Hat Plugin Row

Add inside the appropriate `.plugin-rh-group` > `.plugin-rh-table`:

```html
<tr>
  <td class="rh-name">plugin-name</td>
  <td class="rh-desc">One-line description</td>
  <td class="rh-badge"><span class="plugin-badge badge-planned">Planned</span></td>
</tr>
```

### Adding a New Agent Card

Add inside `.agents-grid`:

```html
<div class="agent-card">
  <div class="agent-avatar" style="background: rgba(R, G, B, 0.1); color: var(--accent-COLOR);">XX</div>
  <div class="agent-info">
    <div class="agent-name">agent-name</div>
    <div class="agent-desc">Description of the agent's role and capabilities.</div>
    <!-- Optional: -->
    <span class="agent-badge readonly">read-only</span>
  </div>
</div>
```

Choose the avatar color based on the agent's role category (see Color Coding section).

### Adding a New Data Flow

1. Add a tab button in `.flow-tabs`:
   ```html
   <button class="flow-tab" data-flow="new-flow" role="tab" aria-selected="false" aria-controls="flow-new-flow">Flow Name</button>
   ```
2. Add the flow diagram panel after the existing panels:
   ```html
   <div class="flow-diagram" id="flow-new-flow" role="tabpanel">
     <div class="flow-steps" data-flow-id="new-flow">
       <!-- Add flow-step elements following the existing pattern -->
     </div>
     <div class="flow-controls">
       <button class="flow-btn" onclick="playFlow('new-flow')" id="play-new-flow">&#9654; Play</button>
       <button class="flow-btn" onclick="resetFlow('new-flow')">&#8634; Reset</button>
     </div>
   </div>
   ```
3. The tab switching and animation JS automatically handle new flows via `data-flow` and `data-flow-id` attributes.

### Adding a New Package Card

Add inside the appropriate `.pkg-group` > `.pkg-grid`:

```html
<div class="pkg-card" data-pkg="package-name" data-search="search keywords space separated">
  <div class="pkg-card-header">
    <span class="pkg-card-name">package/</span>
    <span class="pkg-card-arrow">&rsaquo;</span>
  </div>
  <div class="pkg-card-desc">One-line package description</div>
  <div class="pkg-card-details">
    <div class="pkg-card-detail-label">Key Types</div>
    <div class="pkg-card-detail-value"><code>Type1</code>, <code>Type2</code></div>
    <div class="pkg-card-detail-label">Key Files</div>
    <div class="pkg-card-detail-value"><code>file1.go</code>, <code>file2.go</code></div>
  </div>
</div>
```

The `data-search` attribute contains space-separated keywords for the filter input. The click-to-expand and filter JS automatically handle new cards.

## 10. File Structure

The entire page is a single HTML file with embedded CSS and JavaScript. No external dependencies.

```
project-overview.html
|
|-- <head>
|   |-- Meta tags (charset, viewport)
|   |-- <title>
|   |-- <style>
|       |-- Reset & Custom Properties    (CSS variables, box-sizing)
|       |-- Scrollbar                    (webkit scrollbar styles)
|       |-- Navigation                   (fixed nav, logo, links, hamburger)
|       |-- Layout                       (container, section, labels, titles)
|       |-- Animations                   (keyframes: fadeInUp, blink, pulseGlow, etc.)
|       |-- Hero                         (hero section, badge, features, CTA, terminal)
|       |-- Architecture Diagram         (layers, nodes, connectors, detail panel)
|       |-- Data Flow                    (tabs, steps, controls, animations)
|       |-- Package Explorer             (search, groups, cards, details)
|       |-- Plugins                      (subsections, categories, cards, RH tables, badges, wire protocol)
|       |-- Agents                       (grid, cards, avatars, badges)
|       |-- Tech Stack                   (grid, cards, icons)
|       |-- Getting Started              (steps, code blocks, copy button)
|       |-- Footer
|       |-- Responsive (900px)
|       |-- Responsive (640px)
|
|-- <body>
|   |-- <nav>                            Navigation bar
|   |-- <section id="hero">             Hero with terminal mockup
|   |-- <section id="architecture">     Interactive architecture diagram
|   |-- <section id="data-flow">        Tabbed flow diagrams with play/reset
|   |-- <section id="packages">         Filterable package explorer
|   |-- <section id="plugins">          Plugin collections (General + Red Hat)
|   |-- <section id="agents">           Agent card grid
|   |-- <section id="tech-stack">       Technology stack cards
|   |-- <section id="getting-started">  4-step setup guide with code blocks
|   |-- <footer>
|   |-- <script>
|       |-- Scroll Animations            (IntersectionObserver for .animate-in)
|       |-- Nav Active State             (IntersectionObserver for section tracking)
|       |-- Architecture Diagram         (click/keyboard handlers, component data map)
|       |-- Data Flow Tabs               (tab switching logic)
|       |-- Data Flow Animation          (playFlow, resetFlow, interval timers)
|       |-- Package Explorer             (click-to-expand, filter input handler)
|       |-- Copy Code                    (clipboard API with execCommand fallback)
```

### JavaScript Data

The architecture diagram component data is stored as a plain object (`componentData`) in the script section. Each entry maps a `data-component` value to `{title, desc, files}`. When adding a new architecture node, add both the HTML node element and the corresponding data entry.

### Conventions

- **No external dependencies.** The page must work when opened as a local file (`file://` protocol). The copy-to-clipboard function includes an `execCommand` fallback for this reason.
- **No build step.** Edit the HTML file directly. No preprocessors, bundlers, or frameworks.
- **Semantic HTML.** Use appropriate elements (`<nav>`, `<section>`, `<footer>`, `<button>`, `<table>`).
- **Progressive enhancement.** The page is readable without JavaScript. JS adds interactivity (expand, filter, animate) but content is visible without it.
- **CSS variables only.** Never use raw color values in component styles. All colors, radii, transitions, and font stacks come from `:root` variables.
