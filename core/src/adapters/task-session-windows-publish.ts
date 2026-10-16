import { dlopen, FFIType, ptr } from "bun:ffi";

function loadPublisher() {
  // kernel32 is a preloaded Windows KnownDLL; a project/PATH DLL cannot replace it.
  const library = dlopen("kernel32.dll", {
    MoveFileExW: { args: [FFIType.ptr, FFIType.ptr, FFIType.u32], returns: FFIType.u32 },
    GetLastError: { args: [], returns: FFIType.u32 },
  });
  return (source: string, target: string): void => {
    const from = Buffer.from(source + "\0", "utf16le");
    const to = Buffer.from(target + "\0", "utf16le");
    // Same-directory rename, without REPLACE_EXISTING or COPY_ALLOWED, publishes complete bytes once.
    if (library.symbols.MoveFileExW(ptr(from), ptr(to), 0) !== 0) return;
    const code = library.symbols.GetLastError();
    const error = new Error(`Windows task session publication failed (Win32 ${code})`);
    if (code === 80 || code === 183) Object.assign(error, { code: "EEXIST" });
    throw error;
  };
}

let publisher: ReturnType<typeof loadPublisher> | undefined;

export function publishWindowsTaskSession(source: string, target: string): void {
  (publisher ??= loadPublisher())(source, target);
}
