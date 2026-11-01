# Aurora localization

Settings → Language and formats changes the interface immediately, without
reloading the page. Supported languages are Vietnamese (`vi`), English (`en`),
Simplified Chinese (`zh-Hans`) and Traditional Chinese (`zh-Hant`). Language names
are shown in their own language, without country flags. Match device checks the
ordered browser language list, recognizes Chinese script and region variants,
and falls back to English if none are supported.

Date, time and number formats can follow the interface language or use an
explicit region. These are display preferences on the current device/browser
profile. They do not change scheduled tasks' time zones, send any message,
translate conversation content, or set the model's reply language. No account
sync is claimed. The previous `neko-aurora-lang` values `auto`, `vi` and `en`
remain valid. Formatting uses `neko-aurora-region`. Blocked/full browser storage
keeps the choice in memory and shows that it could not be saved.

## Translator workflow

The source of truth is `app/locales/{vi,en,zh-Hans,zh-Hant}.json`. Edit the four
catalogues, then run:

```sh
python tools/build_locales.py
python tools/build_locales.py --check
node --test app/tests/aurora/localization.test.cjs
```

The build rejects missing/extra keys, empty messages and changed placeholders.
It emits the checked-in offline browser bundle
`app/native/internal/web/assets/app/aurora/locales.js`. Go embeds this bundle and
the pinned MIT i18next library locally. There is no translation service, CDN or
request containing user text. HTML entry points load the engine, resources and
adapter in that order; the service worker includes the same assets.

New code uses descriptive keys in the `ui` namespace. Translate whole sentences;
do not concatenate translated fragments. Use named interpolation and CLDR plural
forms (`_one`, `_other`) where needed. Values must be assigned to `textContent`
or an approved text attribute, never to `innerHTML`.

```js
const i18n = window.__I18N;
i18n.bind(label, "settings.locale.language");
status.textContent = i18n.message("count.bots", { count: bots.length });
i18n.bindAttribute(reply, "aria-label", "chat.quote", () => ({ name: author }));
time.textContent = i18n.date(timestamp, { dateStyle: "medium", timeStyle: "short" });
```

`bind` updates a text-only UI element in place. `bindAttribute` owns a translated
text attribute and can resolve current interpolation values on each change.
Dynamic views can listen to `aurora:localechange`; update labels and formatting
without replacing draft fields, pending forms or streaming message bodies.
`number`, `relative`, `compare` and `speechLocale` centralize the corresponding
locale behavior. Region preferences never implicitly set an IANA time zone.

Existing views still use a **transitional `legacy` namespace**, indexed by their
original Vietnamese phrases. The DOM adapter remembers original values in weak
maps so switching is reversible, including when a view updates a label later.
New features must use semantic keys rather than expanding this migration bridge.
Do not rename legacy keys merely to improve Vietnamese wording: change their
Vietnamese values instead, so existing call sites continue to match.

## Content boundaries and language style

- User/model text, bot names, descriptions, page titles, snippets, code, drafts
  and secrets stay literal. Mark user-controlled UI content `data-noi18n`;
  inherited protection includes attributes. Use explicit attribute bindings when
  a translated action label contains a literal name. Never translate saved data.
- Vietnamese uses a conversational but clear tone, `bạn`, and consistent terms:
  *mô hình*, *nhà cung cấp*, *tệp*, *kho bảo mật*, *máy tính đám mây*. Product and
  protocol names such as ChatGPT, GLM, MCP and API remain recognizable.
- English uses sentence case and concise actions. Singular/plural counts use
  message variants, not an appended `s` or a permanently plural label.
- Simplified Chinese uses terms such as 设置、保存、文件、服务器.
- Traditional Chinese uses Taiwan-oriented terminology such as 設定、儲存、檔案、
  伺服器、帳戶、訊息、排程、登出. It is independently editable; it is not converted
  from Simplified Chinese at runtime. Hong Kong is a formatting-region choice,
  not a claim of a separately reviewed Hong Kong vocabulary pack.
- Chinese uses system CJK font fallbacks. Date order, separators, time periods
  and numbers come from `Intl` locale data. Do not hardcode commas, dots or AM/PM.

Initial Traditional Chinese drafting used local OpenCC 1.4.2 `s2twp`, followed by
explicit terminology corrections. This is translation engineering review, not
external native-speaker certification. Future editorial reviews should review
each Chinese catalogue independently, including error recovery and accessibility
labels, rather than assuming script conversion alone is sufficient.

## Scope of the first migration

This pass covers Settings, navigation, the main conversation controls, core AI
setup and the existing translation catalogue, with added vocabulary for bot
profiles, memory, schedules, pages and the vault. The catalogue parity check
means every **catalogued** message has four versions; it does not prove every
literal in the repository has been migrated.

Remaining migration work includes the long-form legal/onboarding copy, parts of
the cloud-computer and agent-network panels, and some generated operational
messages or compound labels in older views. These remain visible in their
original language if not in the catalogue. Provider/tool output and existing
conversation text intentionally keep their original language. Migrate these
surfaces with explicit keys as they are reviewed, rather than applying machine
translation to arbitrary DOM text or model responses.

Validation must include desktop and narrow browser layouts, live changes in all
four languages, reload persistence, literal content that looks like a UI label,
draft preservation, accessible labels and format-region overrides. Browser
fixtures test UI behavior; they are not live model tests or physical-phone
acceptance. See `docs/research/localization-2026-11-01.md` for this pass's evidence.

## Primary references

- [W3C: choosing language tags](https://www.w3.org/International/questions/qa-choosing-language-tags)
  — distinguish language, script and region with BCP 47 identifiers.
- [W3C: language preferences](https://www.w3.org/International/questions/qa-lang-priorities)
  — respect the ordered preference list and an explicit choice.
- [Unicode CLDR](https://cldr.unicode.org/) and [Intl](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/Intl)
  — use locale data for formatting and plurals.
- [i18next best practices](https://www.i18next.com/principles/best-practices) and
  [fallback](https://www.i18next.com/principles/fallback) — complete messages,
  named runtime values and predictable fallback.
- [OpenCC](https://github.com/BYVoid/OpenCC) — offline script and regional-wording
  conversion as a drafting aid, with separately maintained final catalogues.
