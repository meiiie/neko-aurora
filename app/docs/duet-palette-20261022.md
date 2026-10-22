# Neko Duet palette: "Cream paper, amber and Wiii's red scarf"

The palette comes from live brand assets, pulled on 2026-10-22. Duet keeps the warm cream and ink of HoLiLiHu and Neko, uses Neko amber for "waiting on you", and uses Wiii's red scarf as the one loud accent. Dark mode uses Neko's ink-violet night with a winter-snow off-white.

| Token | Light | Dark | Source |
|---|---|---|---|
| bg | `#f4efe4` | `#131018` | `--cream` on holilihu.online and neko.holilihu.online (also their theme-color) / Neko `--ink-900` |
| card | `#fbf8f2` | `#1b1722` | holilihu `--paper`, Neko `--cream-2` / Neko `--ink-800` |
| sunk / chip | `#e9e2d2` | `#272232` | Neko `--cream-3` / Neko `--ink-700` |
| ink | `#17130e` | `#f4efe4` | Neko `--fg` / cream |
| mute | `#5b544a` | `#a59eae` | Neko `--fg-2` (light) / Neko `#8b8496`, lightened for AA |
| line, line-2 | `#ddd6c5`, `#cbc2ac` | `#272232`, `#3a3348` | Neko `--hair` and `--hair-2` |
| accent (fill) | `#e0261f` | `#e0261f` | Wiii anime `--red`, the scarf |
| accent press, accent text | `#c11f19` | `#ff3b2e`, `#ff5a4e` | scarf darkened / anime `#ff3b2e` |
| waiting text / fill | `#96590a` / `#eda12c` | `#f0a94a` / `#eda12c` | Neko `--amber-deep` / `--amber`, `--amber-soft` |
| approve | `#2c7f52` | `#63cf8d` | Neko `--ok-cream` / `--ok-ink` |
| danger | `#a3160f` | `#ff8a7a` | anime `#7a1008` lifted / Neko `#e2827c` lifted |
| snow | `#f7f5eb` | | Wiii `weather.ts` snow `vec3(0.97,0.96,0.92)` |
| pixel outline | `#2a2928` | | Neko Peek lockup `#2A2928` (neko-bot brand asset) |

## Avatar shells
The 10 shells, in order:

| # | Hex | Source |
|---|---|---|
| 1 | `#e0261f` | Wiii scarf |
| 2 | `#e2827c` | Neko coral |
| 3 | `#eda12c` | Neko amber |
| 4 | `#f7b551` | `--amber-lift` |
| 5 | `#96590a` | `--amber-deep` |
| 6 | `#2c7f52` | ok green |
| 7 | `#74cfc6` | Neko teal |
| 8 | `#506c62` | neko-bot UI sage |
| 9 | `#8b8496` | Neko lilac grey |
| 10 | `#e8bd63` | Neko gold |

The "scarf" extra is drawn in Wiii red.

## WCAG AA contrast
All text and control pairs are at least 4.5:1.

| Mode | Pair | Ratio |
|---|---|---|
| Light | ink on bg | 16.1 |
| Light | mute on bg | 6.5 |
| Light | white on accent | 4.7 |
| Light | white on accent press | 6.0 |
| Light | accent text on bg | 5.3 |
| Light | waiting text on bg | 4.9 |
| Light | white on approve | 4.9 |
| Light | danger on card | 7.4 |
| Dark | ink on bg | 16.4 |
| Dark | mute on card | 6.8 |
| Dark | accent text on bg | 6.1 |
| Dark | waiting on card | 8.8 |
| Dark | approve on card | 9.1 |
| Dark | dark ink on approve fill | 9.7 |
| Dark | danger on card | 7.7 |

## iOS
- `theme-color` is set per `prefers-color-scheme` (`#f4efe4` and `#131018`), with `color-scheme: light dark`.
- `apple-mobile-web-app-status-bar-style=default`, so the status bar follows the system appearance over the safe-area padding.
- PNG `apple-touch-icon` at 180 px (iOS ignores SVG touch icons), plus a 512 px maskable manifest icon.
