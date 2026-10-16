use sha2::{Digest, Sha256};
use std::{
    fs::{self, OpenOptions},
    io::Write,
    path::Path,
};

#[cfg(windows)]
mod platform {
    use std::{ptr, slice};
    #[repr(C)]
    struct Blob {
        size: u32,
        data: *mut u8,
    }
    #[link(name = "crypt32")]
    unsafe extern "system" {
        fn CryptProtectData(
            input: *const Blob,
            description: *const u16,
            entropy: *const Blob,
            reserved: *mut u8,
            prompt: *mut u8,
            flags: u32,
            output: *mut Blob,
        ) -> i32;
        fn CryptUnprotectData(
            input: *const Blob,
            description: *mut *mut u16,
            entropy: *const Blob,
            reserved: *mut u8,
            prompt: *mut u8,
            flags: u32,
            output: *mut Blob,
        ) -> i32;
    }
    #[link(name = "kernel32")]
    unsafe extern "system" {
        fn LocalFree(memory: *mut u8) -> *mut u8;
    }
    fn crypt(bytes: &[u8], entropy: &[u8], seal: bool) -> Result<Vec<u8>, &'static str> {
        let input = Blob {
            size: bytes.len() as u32,
            data: bytes.as_ptr() as *mut u8,
        };
        let entropy = Blob {
            size: entropy.len() as u32,
            data: entropy.as_ptr() as *mut u8,
        };
        let mut output = Blob {
            size: 0,
            data: ptr::null_mut(),
        };
        let success = unsafe {
            if seal {
                CryptProtectData(
                    &input,
                    ptr::null(),
                    &entropy,
                    ptr::null_mut(),
                    ptr::null_mut(),
                    1,
                    &mut output,
                )
            } else {
                CryptUnprotectData(
                    &input,
                    ptr::null_mut(),
                    &entropy,
                    ptr::null_mut(),
                    ptr::null_mut(),
                    1,
                    &mut output,
                )
            }
        };
        if success == 0 {
            return Err("vault_protection_failed");
        }
        if output.data.is_null() || output.size > 16384 {
            if !output.data.is_null() {
                unsafe {
                    LocalFree(output.data);
                }
            }
            return Err("vault_output_invalid");
        }
        let mut result =
            unsafe { slice::from_raw_parts(output.data, output.size as usize).to_vec() };
        unsafe {
            ptr::write_bytes(output.data, 0, output.size as usize);
            LocalFree(output.data);
        }
        if !seal && result.len() > 4096 {
            result.fill(0);
            return Err("vault_output_invalid");
        }
        Ok(result)
    }
    pub fn seal(b: &[u8], e: &[u8]) -> Result<Vec<u8>, &'static str> {
        crypt(b, e, true)
    }
    pub fn open(b: &[u8], e: &[u8]) -> Result<Vec<u8>, &'static str> {
        crypt(b, e, false)
    }
}
pub fn matches(
    state: &Path,
    binding: &str,
    reference: &str,
    secret: &mut [u8],
) -> Result<bool, &'static str> {
    if reference.len() != 39
        || !reference.starts_with("secret-")
        || !reference[7..]
            .bytes()
            .all(|c| c.is_ascii_digit() || (b'a'..=b'f').contains(&c))
        || binding.len() > 256
        || secret.len() > 4096
    {
        return Err("vault_input_invalid");
    }
    let cipher = fs::read(state.join("secrets").join(format!("{reference}.dpapi")))
        .map_err(|_| "vault_storage_unavailable")?;
    if cipher.len() > 16384 {
        return Err("vault_output_invalid");
    }
    let entropy = Sha256::digest(format!("neko-vault-v1\0{binding}\0{reference}"));
    #[cfg(windows)]
    let mut plain = platform::open(&cipher, &entropy)?;
    #[cfg(not(windows))]
    return Err("vault_platform_unavailable");
    #[cfg(windows)]
    {
        let mut different = plain.len() ^ secret.len();
        for i in 0..plain.len().max(secret.len()) {
            different |= usize::from(
                plain.get(i).copied().unwrap_or(0) ^ secret.get(i).copied().unwrap_or(0),
            );
        }
        plain.fill(0);
        secret.fill(0);
        Ok(different == 0)
    }
}

pub fn seal(
    state: &Path,
    binding: &str,
    reference: &str,
    secret: &mut [u8],
) -> Result<(), &'static str> {
    if secret.is_empty()
        || secret.len() > 4096
        || binding.is_empty()
        || binding.len() > 256
        || reference.len() != 39
        || !reference.starts_with("secret-")
        || !reference[7..]
            .bytes()
            .all(|c| c.is_ascii_digit() || (b'a'..=b'f').contains(&c))
    {
        return Err("vault_input_invalid");
    }
    let entropy = Sha256::digest(format!("neko-vault-v1\0{binding}\0{reference}"));
    #[cfg(windows)]
    let cipher = platform::seal(secret, &entropy);
    #[cfg(not(windows))]
    let cipher: Result<Vec<u8>, &'static str> = Err("vault_platform_unavailable");
    secret.fill(0);
    let cipher = cipher?;
    let directory = state.join("secrets");
    fs::create_dir_all(&directory).map_err(|_| "vault_storage_unavailable")?;
    let mut file = OpenOptions::new()
        .write(true)
        .create_new(true)
        .open(directory.join(format!("{reference}.dpapi")))
        .map_err(|_| "vault_storage_unavailable")?;
    file.write_all(&cipher)
        .and_then(|_| file.sync_all())
        .map_err(|_| "vault_storage_unavailable")
}

#[cfg(all(test, windows))]
mod tests {
    use super::*;
    #[test]
    fn dpapi_roundtrip_and_wrong_account_rejected() {
        let original = b"fixture-only-secret-not-a-provider-key";
        let entropy = Sha256::digest("local-owner:account-fixture:revision-1");
        let cipher = platform::seal(original, &entropy).unwrap();
        assert!(!cipher.windows(original.len()).any(|w| w == original));
        let mut plain = platform::open(&cipher, &entropy).unwrap();
        assert_eq!(&plain, original);
        plain.fill(0);
        assert!(platform::open(&cipher, &Sha256::digest("another-account")).is_err());
    }
}
