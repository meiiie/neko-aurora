# Sidebar and coworker presence study — 2026-11-01

Scope: improve conversation navigation and quiet avatar presence on the existing Aurora web/Tauri surface. Preserve chat routing, drafts, user pin choices, bot identities and provider selection. Baseline: `e9cc7f4f63ba45ac0e174f6790f26306536c3bdc`. Solo implementation and review following the local pstack adoption workflow; no independent-agent review is claimed.

## Reference findings

- The user's current Grok Bot screenshots put pinned coworkers and recent conversations in one vertical space. Neutral selection, an unobtrusive scroll thumb, compact search/create controls and a quiet footer leave most space for names and previews. A still screenshot does not prove scrollbar auto-hide timing or motion behavior.
- [Grok Bot UI/UX kit](https://github.com/xingyingyuzhui/grok-bot-ui-ux-kit), local reference commit `5bb4379e9ec1158b9ec29c3c5f1e2cc6604be5af`: reviewed avatar catalog/reference/audit documents and the sidebar styles. Its audit distinguishes website behavior from desktop behavior and describes smaller-avatar variants. Visual state names alone do not establish identical implementations.
- [Animation anatomy](https://github.com/ArvinYin1/grokbot-animation-anatomy), cloned into `reference/grokbot-animation-anatomy` at `debd41625da89b4c4fde43b3b1f0add6dc113c02`: statically reviewed README, `js/sections/states.js`, `springs.js`, `sphere.js` and `js/lib.js`. The useful principles are coordinated animation tracks, smooth state transitions, identity-dependent variation, eyes following head orientation, and explicit visibility/reduced-motion handling. The repository identifies extracted/ported original engine material. It was not executed, and no player, recipes, shapes or assets were incorporated into Aurora.
- [Grok Bots Studio](https://grokbots.ai/studio?color=ink&face=suspicious): observed the live Look/Move controls and wink preview. Separating expression, movement and cursor attention gives a character more range than rotating an otherwise rigid image. This is a third-party design reference, not a specification of the official app.

## Aurora implementation

The desktop header uses the existing quick-search dialog, accessible by its search button or Ctrl/Cmd+K. The mobile inline search remains. A nonempty mobile filter stays visible when the viewport becomes desktop-sized so no invisible filter can hide conversations. The existing filter menu, routines, creation and navigation remain accessible.

Pinned cards and conversation rows share one scroll container. A stable, thin scrollbar has a transparent track, a quiet default thumb and stronger hover/focus contrast; forced-colors mode uses the system scrollbar. Desktop rows are 60px high, with 38px avatars and two lines of text. Pinned avatars remain 62px. Neutral selection, a subtle sidebar tint, a footer fade and a smaller unread chip reduce visual competition with chat. Pins are never added automatically.

Avatar geometry remains Aurora's own. New transform layers coordinate the existing body and eyes without new libraries or external assets:

- Pointer attention eases into a bounded eye offset and a small head turn; leaving or scrolling returns it to neutral.
- A brief greeting responds to pointer entry or visible keyboard focus, with a cooldown to avoid repeated bouncing.
- Idle avatars occasionally glance or settle, then rest for much longer. One shared scheduler permits only one spontaneous gesture at a time. Identity seeds vary the timing and direction.
- Working motion applies to the entire character. Waiting/error expressions take priority over idle gestures. Idle animation never changes the actual bot status or claims work/completion.
- Offscreen/hidden avatars, the user's motion-off setting and OS reduced-motion preference stop movement. Idle clips use CSS timing; JavaScript animation frames run only while gaze is converging. Cached avatar nodes preserve continuity during sidebar updates.

This deliberately keeps the existing flat character style. It does not import the reference's 53-state engine, 3D projection or extracted animation curves.

## Validation record

Exact commands, environment, candidate hashes, final commit and screenshots are recorded locally under `.audit/2026-11-01-living-sidebar/`. Checks include the avatar scheduler tests, conversation/draft/outbox/stream/localization regressions, UX regression harness, Go embedded-web tests, syntax and documentation checks. Browser acceptance uses an isolated 24-agent fixture with nine pins and a three-member group: search by keyboard, draft retention, scroll continuity, offscreen pause, motion-off, dark theme, four locales, and 1280px/390px viewports. These are local UI checks, not provider quality or physical-phone acceptance.

The final local app is rebuilt from the checkpoint, served assets are compared with source hashes, and existing AI selections/settings are checked before and after relaunch. No model request, external deployment or push is part of this change. Issue-form files and the Rust trusted-host/desktop source are unchanged; the previously verified Rust binaries are reused with their manifest hashes checked.
