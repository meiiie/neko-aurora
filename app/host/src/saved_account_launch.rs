//! Private Go-to-host admission only. No CLI, scheduler selection, or agent loop.
//! The outer frame contains opaque secretRef and revisionKey authorization material.
use crate::{
    context_admission::{self, LaunchContext},
    executor::{self, Companion, Descriptor},
    saved_account_bootstrap::{self, PreparedBootstrap, ProtectedExecutables, WitnessMetadata},
    saved_key::{self, Binding},
};
use serde::{Deserialize, Serialize};
use serde_json::Value;
use std::{
    fs,
    path::{Path, PathBuf},
};
type Result<T> = std::result::Result<T, &'static str>;
const ERROR: &str = "saved_account_launch_invalid";

#[derive(Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct Launch {
    request_id: String,
    bot_id: String,
    text: String,
    context_protocol: u32,
    context_manifest: Value,
    #[serde(rename = "contextManifestSHA256")]
    context_manifest_sha256: String,
    #[serde(rename = "contextPinSHA256")]
    context_pin_sha256: String,
    executor_pin: Value,
    attempt_lease: Value,
}
#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct Envelope {
    version: u32,
    purpose: String,
    launch: Launch,
    request: saved_key::Request,
    witness: WitnessMetadata,
    root: PathBuf,
}
struct PrivateFrame(Vec<u8>);
impl Drop for PrivateFrame {
    fn drop(&mut self) {
        self.0.fill(0);
    }
}

fn same_leaf(path: &Path, expected: &Path) -> bool {
    path.is_absolute()
        && path.file_name() == expected.file_name()
        && path
            .parent()
            .and_then(|p| fs::canonicalize(p).ok())
            .as_deref()
            == expected.parent()
}

/// Only construction through parse binds the private request to a validated
/// executor companion, full context manifest and trusted install/task root.
/// No Debug/Serialize implementation: this object owns private authorization.
pub struct AdmittedLaunch {
    request: PrivateFrame,
    binding: Binding,
    witness: WitnessMetadata,
    root: PathBuf,
    companion: Companion,
    context: LaunchContext,
    descriptor: Descriptor,
}
impl AdmittedLaunch {
    pub fn parse(raw: &[u8], install_root: &Path, actual: &Descriptor, now: u64) -> Result<Self> {
        if raw.is_empty() || raw.len() > 64000 {
            return Err(ERROR);
        }
        // canonical rejects duplicate keys, invalid Unicode and unsafe numbers.
        let canonical = PrivateFrame(crate::canonical(raw).map_err(|_| ERROR)?);
        if canonical.0.len() > 64000 {
            return Err(ERROR);
        }
        let envelope: Envelope = serde_json::from_slice(&canonical.0).map_err(|_| ERROR)?;
        if envelope.version != 1 || envelope.purpose != "saved_account_launch_v1" {
            return Err(ERROR);
        }
        saved_key::validate_request(&envelope.request, now).map_err(|_| ERROR)?;
        let binding = &envelope.request.binding;
        if binding.fixture_only {
            return Err(ERROR);
        }
        let launch = serde_json::to_value(&envelope.launch).map_err(|_| ERROR)?;
        let companion = Companion::validate(
            &launch["executorPin"],
            &launch["attemptLease"],
            actual,
            &envelope.launch.request_id,
            now,
        )
        .map_err(|_| ERROR)?;
        // proof is derived from the validated typed lease, not an untrusted copy.
        let proof = companion.proof();
        if binding.request_id != envelope.launch.request_id
            || proof["attemptId"] != binding.attempt_id
            || proof["workEpoch"] != binding.epoch
            || proof["bootId"] != envelope.witness.service_generation
            || proof["authoritySha256"] != envelope.witness.authority_sha256
        {
            return Err(ERROR);
        }
        // Account leaseFence and executor proof.fence are separate authorities.
        // Never replace one with the other, or require accidental numeric equality.
        let context = LaunchContext::parse(&launch)
            .map_err(|_| ERROR)?
            .ok_or(ERROR)?;
        let manifest = &envelope.launch.context_manifest;
        if context_admission::manifest_digest(manifest).map_err(|_| ERROR)?
            != binding.context_sha256
            || envelope.launch.text != manifest["primaryMessage"]["text"]
            || envelope.launch.bot_id != manifest["scope"]["botId"]
        {
            return Err(ERROR);
        }
        let install = fs::canonicalize(install_root).map_err(|_| ERROR)?;
        if executor::root_hash(&install).map_err(|_| ERROR)? != actual.root_sha256 {
            return Err(ERROR);
        }
        let tasks = fs::canonicalize(install.join("tasks")).map_err(|_| ERROR)?;
        if tasks != install.join("tasks") {
            return Err(ERROR);
        }
        let root = fs::canonicalize(&envelope.root).map_err(|_| ERROR)?;
        if root != tasks.join(&binding.request_id) {
            return Err(ERROR);
        }
        let home = fs::canonicalize(&envelope.request.state_home).map_err(|_| ERROR)?;
        if home != root.join(".core-home") {
            return Err(ERROR);
        }
        let private = fs::canonicalize(
            install
                .join("saved-accounts")
                .join(&binding.request_id)
                .join(&binding.attempt_id),
        )
        .map_err(|_| ERROR)?;
        if private
            != install
                .join("saved-accounts")
                .join(&binding.request_id)
                .join(&binding.attempt_id)
            || fs::canonicalize(&envelope.request.connections_root).map_err(|_| ERROR)?
                != install.join("connections")
            || !same_leaf(
                Path::new(&envelope.request.capsule_path),
                &private.join("capsule.json"),
            )
            || !same_leaf(&envelope.witness.path, &private.join("witness.json"))
        {
            return Err(ERROR);
        }
        let request = PrivateFrame(serde_json::to_vec(&envelope.request).map_err(|_| ERROR)?);
        if request.0.len() > 16384 {
            return Err(ERROR);
        }
        Ok(Self {
            request,
            binding: binding.clone(),
            witness: envelope.witness,
            root,
            companion,
            context,
            descriptor: actual.clone(),
        })
    }
    pub fn context(&self) -> &LaunchContext {
        &self.context
    }
    pub fn root(&self) -> &Path {
        &self.root
    }

    /// Internal preparation composition. Still unreachable from run()/CLI.
    /// Live authorization/revocation callbacks follow saved_account_process's
    /// nonblocking, nonpanicking, private-data contract.
    pub fn prepare(
        self,
        core: &Path,
        authorize: &dyn Fn(&Binding) -> bool,
        revoke: &dyn Fn(),
    ) -> Result<PreparedBootstrap> {
        let revoked = std::cell::Cell::new(false);
        let revoke_once = || {
            if !revoked.replace(true) {
                revoke();
            }
        };
        let result = (|| {
            let now = executor::now_ms().map_err(|_| ERROR)?;
            self.companion.live(now).map_err(|_| ERROR)?;
            let images = ProtectedExecutables::open(core, &self.descriptor).map_err(|_| ERROR)?;
            let live_authorize = |binding: &Binding| {
                executor::now_ms().is_ok_and(|n| self.companion.live(n).is_ok())
                    && authorize(binding)
            };
            let current = || live_authorize(&self.binding);
            saved_account_bootstrap::prepare(
                &self.request.0,
                now,
                Some(&live_authorize),
                &self.witness,
                &self.root,
                images,
                |images, frame| {
                    crate::saved_account_process::prepare(images, frame, &current, &revoke_once)
                },
            )
            .map_err(|_| ERROR)
        })();
        if result.is_err() {
            revoke_once();
        }
        result
    }
}
