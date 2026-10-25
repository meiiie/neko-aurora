const matches = {
  username: t => t.role === 'textbox' && !t.protected && /email|user|phone|tài khoản|điện thoại|số điện/i.test(`${t.name} ${t.type}`),
  password: t => t.type === 'password',
  totp: t => t.protected && t.type !== 'password',
};
export function loginFieldPins(targets) {
  return Object.fromEntries(Object.entries(matches).map(([field, match]) => {
    const t = targets.find(match);
    return [field, t ? { key: t.key, role: t.role, name: t.name, type: t.type, protected: Boolean(t.protected) } : null];
  }));
}
export function sameFieldSignature(expected, actual) {
  return Boolean(expected && actual && ['role', 'name', 'type', 'protected'].every(k => (expected[k] ?? '') === (actual[k] ?? '')));
}
export function sameFieldPin(expected, actual) {
  return !expected && !actual || expected?.key === actual?.key && sameFieldSignature(expected, actual);
}
