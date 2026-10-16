import { expect, test } from "bun:test";
import { CredentialOutputFilter, redactCredentialJSON } from "../src/shared/credential-output.ts";

test("stream filter covers every split, repeated/overlapping prefixes and mixed channels without altering safe text", () => {
  for (const secret of ["fixture-long-secret", "aaaaab", "ababab", "🚀-fixture-key"]) {
    const text = `hello ${secret} then ${secret} done.`;
    for (let cut = 0; cut <= text.length; cut++) {
      const filter = new CredentialOutputFilter(secret);
      const out = [...filter.push(text.slice(0, cut), "content"), ...filter.push(text.slice(cut), "reasoning"), ...filter.flush()].map((p) => p.text).join("");
      expect(out).toBe(text.split(secret).join("[redacted credential]"));
      expect(out.includes(secret)).toBe(false);
    }
  }
});

test("unfinished credential prefix is never flushed raw on cancellation/error; absent key is transparent", () => {
  const filter = new CredentialOutputFilter("fixture-private-key");
  expect(filter.push("safe fixture-pri", "content").map((p) => p.text).join("")).toBe("safe ");
  expect(filter.flush()[0]?.text).toBe("[redacted credential]");
  const none = new CredentialOutputFilter("");
  expect(none.push("plain text", "content")).toEqual([{ text: "plain text", kind: "content" }]);
  expect(none.flush()).toEqual([]);
});

test("deep public strings are redacted, credential-bearing dictionary keys refuse, mask cannot repeat credential", () => {
  expect(redactCredentialJSON({ nested: ["before fixture-key after"] }, "fixture-key")).toEqual({ nested: ["before [redacted credential] after"] });
  expect(() => redactCredentialJSON({ "fixture-key": "value" }, "fixture-key")).toThrow("field name refused");
  const filter = new CredentialOutputFilter("redacted");
  expect(filter.push("redacted", "content").map((p) => p.text).join("").includes("redacted")).toBe(false);
});
