# Vault

A Vault for each account, modeled on Grok Bot's Vault. It holds payment cards, logins (site, username, password, optional TOTP) and personal details (name, email, phone, address).

## Storage
- Each item is one AES-256-GCM entry in the account's existing vault (`internal/vault`). The name is `VI_<id>` and the kind is `vault:<type>`. The value is the JSON field map.
- In hosted mode the key is derived per user from `AURORA_SECRET`. No key file sits on the volume.
- `List()` (the named-secret API) never includes items. `Items()` returns metadata only: label, brand + last4, site, a masked username hint, and which fields are filled.
- Redaction is per field: the card number in any 4-4-4-4 grouping, the CVC only next to its label, plus passwords, TOTP secrets and identity values. It applies to model requests (`stream`), activity lines, the use log and the process log (`vault.LogWriter` redacts with every open vault). Memory refuses any fact that contains a stored value.

## UI
- Settings → Bảo mật → Vault: list, add, edit, delete. Values stay masked until the owner taps "Hiện giá trị". The reveal is logged and auto-hides after 30 s.
- An in-chat form card. The bot prints `SECRET: {"vault":"card|login|identity",...}` and the owner types the values into the card. The values go to `POST /app/api/vault` and never reach the transcript or the model. The bot gets a hidden follow-up that carries only the metadata.

## Bot tools
- `vault_list`: returns metadata only.
- `vault_use {item,url}` or `{item,cmd}`: parks a `kind:"vault"` approval card. The card can't be edited; it can only be approved or declined. After approval the server does one of two things:
  - fills the page's form in a fresh headless Chrome, driven over `--remote-debugging-pipe` and matched by autocomplete/name/label, then submits it;
  - or runs one sandbox command with `VAULT_*` env vars. That command is not added to the bot's command log.

  Either way the use is logged (`vault_uses.jsonl`, metadata only), and the bot gets a redacted result.
- Public https targets only. A live TOTP code is computed server-side.

## Test page
`/app/vault-demo.html` is a demo checkout. It checks the fields in the page and sends nothing. Use only the 4242 test card.
