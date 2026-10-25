# Third-party configuration

`seccomp.playwright.json` is the unmodified Chromium sandbox profile from Microsoft Playwright, tag `v1.64.1`:

https://github.com/microsoft/playwright/blob/v1.64.1/utils/docker/seccomp_profile.json

The upstream Apache License 2.0 is preserved in `LICENSE.playwright.txt`. That license applies to the imported configuration, not to the whole Aurora repository.

The profile keeps a default-deny syscall policy and permits user namespace creation for Chromium sandboxing. Compose uses it instead of disabling seccomp or Chromium's sandbox.
