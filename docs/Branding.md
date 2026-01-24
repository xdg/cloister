# Cloister brand guidelines

This document outlines the visual identity direction for Cloister, an AI agent sandboxing system.

## Brand essence

**Cloister** evokes the protected inner courtyards of medieval monasteries—enclosed, tranquil spaces where focused work happens safely, shielded from the chaos outside. The name suggests:

- **Protection without isolation** — A cloister isn't a prison; it's a sanctuary
- **Calm control** — Security that enables focus rather than creating anxiety
- **Architectural strength** — Stone walls, arched passages, enduring structure
- **Curated access** — Selective openings to the outside world

### Tagline options

- "Secure sandboxing for AI agents"
- "Let AI agents work. Safely."
- "Containment with clarity"
- "The walled garden for AI development"

---

## Color palette

### Primary colors

| Name | Hex | RGB | Usage |
|------|-----|-----|-------|
| **Cloister Stone** | `#2D3748` | 45, 55, 72 | Primary brand color, headers, CLI output |
| **Archway Blue** | `#4A6FA5` | 74, 111, 165 | Links, interactive elements, proxy status |
| **Courtyard White** | `#F7FAFC` | 247, 250, 252 | Backgrounds, safe/allowed states |

### Accent colors

| Name | Hex | RGB | Usage |
|------|-----|-----|-------|
| **Approval Green** | `#48BB78` | 72, 187, 120 | Approved commands, allowed traffic, success |
| **Pending Amber** | `#ECC94B` | 236, 201, 75 | Awaiting approval, warnings, caution |
| **Blocked Rust** | `#C53030` | 197, 48, 48 | Denied requests, blocked domains, errors |

### Extended palette

| Name | Hex | Usage |
|------|-----|-------|
| **Monastery Slate** | `#1A202C` | Dark mode backgrounds |
| **Cloister Shadow** | `#718096` | Secondary text, borders |
| **Parchment** | `#FFFAF0` | Documentation, light accents |

### Color philosophy

The palette draws from medieval stone architecture:
- **Cool, grounded blues and grays** convey stability and trustworthiness
- **Warm amber** signals "attention needed" without alarm
- **Muted green and rust** for clear approve/deny semantics without traffic-light clichés

---

## Logo

A romanesque arch—the defining architectural feature of cloisters—with AI sparkles nestled within. The arch represents the controlled gateway and protective structure; the sparkles represent the AI agent working safely inside the sandboxed environment. The combination conveys that AI creativity and capability are preserved, not stifled, by thoughtful containment.

**Variations:**
- Full color with Archway Blue arch and Pending Amber sparkles
- Monochrome outline (works at small sizes)
- Simplified icon version for favicons (arch silhouette with single sparkle)

---

## Typography

### Primary typeface

**Inter** — Clean, modern, excellent for both UI and documentation. Open source.

- Headers: Inter Semi-Bold (600)
- Body: Inter Regular (400)
- Code: JetBrains Mono or Fira Code

### Alternative stacks

For contexts where Inter isn't available:
- System: `-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif`
- Monospace: `"SF Mono", Monaco, "Cascadia Code", monospace`

---

## Voice and tone

### In documentation

- **Direct and clear** — Security tools shouldn't be confusing
- **Confident but not arrogant** — "Cloister prevents X" not "Cloister is the only way to prevent X"
- **Educational** — Help users understand *why*, not just *how*

### In CLI output

- **Concise** — Respect terminal real estate
- **Actionable** — Clear next steps when something is blocked
- **Calm** — Don't use alarming language for routine security controls

**Examples:**

```
# Good
Blocked: pypi.org not in allowlist. Add to .cloister.yaml to allow.

# Avoid
SECURITY ALERT: Unauthorized network access attempt detected!
```

---

## Usage notes

### Logo clear space

Maintain padding equal to the height of the arch element on all sides.

### Minimum sizes

- Print: 0.5 inches / 12mm
- Digital: 24px height
- Favicon: Simplified arch with single sparkle

### Color usage

- **Light backgrounds:** Use Cloister Stone or Archway Blue
- **Dark backgrounds:** Use Courtyard White or Archway Blue
- **Monochrome contexts:** Cloister Stone only

---

## File naming convention

```
cloister-logo-color.svg       # Full color version
cloister-logo-mono.svg        # Single color version
cloister-icon-color.svg       # Square icon (for favicons, app icons)
cloister-icon-mono.svg        # Monochrome icon
cloister-wordmark.svg         # Text logotype only
```

---

## Attribution

Colors inspired by traditional stone masonry and manuscript illumination palettes. The brand identity aims to feel both ancient (trustworthy, enduring) and modern (clean, technical).
