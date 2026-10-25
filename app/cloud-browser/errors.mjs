export function fault(code, status = 409) {
  return Object.assign(new Error(code), { code, status, safeRejection: true });
}
