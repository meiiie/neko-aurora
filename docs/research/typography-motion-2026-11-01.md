# Aurora typography and conversation navigation

Base commit: `70934be909a3d9d55b0c1e398be0c039309abf0f`.
Scope: local frontend experience, one implementer, following the repository's
pstack workflow (premise, evidence, focused implementation, verification).

## Evidence and choice

The user's current Grok Bot screenshots dated 2026-11-01 show a three-column
pinned grid, compact conversation rows, hover previews, group avatars and a
search list with message snippets. They are newer visual evidence than the
reconstructed kit at `reference/grok-bot-ui-ux-kit` (`5bb4379`). Static screenshots
do not establish the current application's animation timing or backend behavior.
The older kit's avatar and visual-system documentation was studied for motion
principles: move body and gaze separately, keep identity stable, respect reduced
motion, and avoid animating offscreen avatars. No extracted code, artwork,
proprietary XVF font or generated avatar engine was copied.

The previous app combined Be Vietnam Pro, system sans and a serif display face
through multiple stylesheet overrides. Be Vietnam Pro shipped 400/600/700 while
UI styles also requested 500/550/650. Sidebar names were 13px and snippets 11px.
That combination made the hierarchy and density inconsistent.

Be Vietnam Pro, Inter and Source Sans 3 were compared locally at equal 16px/18px
sizes, with Vietnamese diacritics, long chat text, labels and mixed Chinese text.
Source Sans 3 was selected after this visual comparison: a less geometric rhythm
for long passages, readable small UI labels, and continuous weight selection.
This is an implementation/design judgment, not a controlled reader study.

- [Adobe Source Sans](https://github.com/adobe-fonts/source-sans) describes its UI
  purpose. The pinned upright and italic files have a 200–900 weight axis.
- [Inter](https://rsms.me/inter/) remains a capable UI alternative; it was not
  rejected for lacking Vietnamese support.
- [Be Vietnam Pro](https://github.com/bettergui/BeVietnamPro) remains appropriate
  for Vietnamese; its existing outlined Aurora wordmark is unchanged.
- [W3C motion guidance](https://www.w3.org/WAI/WCAG22/Understanding/animation-from-interactions.html)
  informs the motion preference and reduced-motion behavior.

The new font files are unmodified OFL assets from Adobe commit
`87b37a2daaed80fcb8e8ccb0085c4d72ddade12e`. Upright is 170,188 bytes and italic
137,996 bytes, loaded from the local application. The provenance JSON records
exact URLs and SHA-256 values. The complete license ships beside the fonts.

## Implemented behavior

`aurora/typography.css` is the final typography layer for the application and
localized standalone pages. Source Sans 3 is used for UI, chat and headings;
JetBrains Mono remains for code. Desktop chat is 18px/1.6, names 16px/23px,
snippets 14px/21px, with lighter intermediate weights. CJK chat uses 1.75 line
height. Source Sans is not claimed to contain CJK glyphs: Simplified Chinese
uses bundled Noto Sans SC Variable and Traditional Chinese Noto Sans TC Variable.
Platform fonts remain the last fallback for unsupported characters. Emoji fonts
precede the CJK fallback so color emoji are preserved where available.

The first browser pass exposed an inconsistent system CJK fallback. Both Noto
families are therefore bundled from Fontsource 5.3.1, with tarball SHA-512 and
per-file SHA-256 verification. Their 209 unmodified Unicode-range WOFF2 files
total 8,777,376 bytes on disk. Only ranges needed by rendered text are requested;
the service worker caches those font requests on demand instead of precaching
both complete families for every user. The browser's asset inventory confirms
requests for the primary font and the selected CJK family. FontTools checks
coverage of every Han character in each Chinese catalogue, in addition to the
Latin/Vietnamese checks. See [Fontsource subsets](https://fontsource.org/docs/getting-started/subsets).

Source Sans declarations live with the new typography layer. During testing an
older cached `fonts.css` still omitted the newly added declarations despite the
computed family name being correct. Browser CSS-rule and resource inspection
caught this; the final checks verify actual font declarations and requested
assets, not just the family string. The older font registry is unchanged.

The desktop sidebar uses a 320px list and 56px navigation rail, a three-column
pinned grid, compact local clock/date labels, and previews on hover or keyboard
focus. Escape dismisses a preview; it is also hoverable. Existing context-menu,
pinning, group membership and chat navigation operations are retained.

Group avatars show up to three distinct members and an overflow count. Search
shows names and current status/message previews and matches member names too.
User-authored names and snippets stay literal across language changes. Existing
keyboard selection and IME safeguards remain in place.

Avatar animation is authored in Aurora using its own existing SVG shapes.
Eyes blink; working state adds a small body/gaze movement; hover/focus greets.
It does not create task status. Automatic animation stops for reduced motion,
hidden documents and offscreen avatars. Detached nodes are unobserved, and
state refreshes use stable clock/identity phases. Settings offers automatic/off,
localized in Vietnamese, English, Simplified and Traditional Chinese. The
preference persists locally and syncs between windows; storage failure is shown
as temporary, following the existing settings convention.

## Validation scope

Receipts and screenshots are in `.audit/2026-11-01-typography-motion/` (ignored).
The manifest and checkpoint receipt tie validation to exact source hashes.
Commands, environment, duration and PASS/FAIL are recorded in `validation.jsonl`.

Checks cover JavaScript syntax; locale catalogue parity; Node regressions for
drafts, interruption, outbox, work status and avatar lifecycle; Go embedded
asset serving and build; font hashes, weight axes and Vietnamese glyph coverage;
and docs. Browser checks use the actual UI through CUA against an isolated
synthetic fixture: four locales at 390/866/1280px, dark theme, keyboard search,
group avatars, preview dismissal, draft retention and motion preference reload.
No model-provider call, cloud sync, physical-phone acceptance or native
screen-reader acceptance is claimed. The existing runtime architecture and
provider selections are outside this frontend change.
