import { DatabaseSync } from 'node:sqlite';
import { open, readFile, unlink, mkdir, lstat } from 'node:fs/promises';
import { join } from 'node:path';
import { randomUUID } from 'node:crypto';
import { fault } from './errors.mjs';

async function processStart(pid) {
  try { return (await readFile(`/proc/${pid}/stat`, 'utf8')).split(') ')[1].split(' ')[19]; }
  catch { return null; }
}
export async function privateDirectory(path) {
  await mkdir(path, { recursive: true, mode: 0o700 });
  if (!(await lstat(path)).isDirectory() || (await lstat(path)).isSymbolicLink()) throw fault('unsafe_profile_directory');
}
export async function openJournal(dataDir) {
  await privateDirectory(dataDir);
  const lockPath = join(dataDir, 'worker.lock');
  const lockId = randomUUID();
  const owner = { pid: process.pid, start: await processStart(process.pid), id: lockId };
  let lock;
  try { lock = await open(lockPath, 'wx', 0o600); }
  catch (err) {
    if (err.code !== 'EEXIST') throw err;
    let prior;
    try { prior = JSON.parse(await readFile(lockPath, 'utf8')); } catch { throw fault('profile_volume_locked'); }
    let alive = true;
    try { process.kill(prior.pid, 0); } catch (e) { if (e.code === 'ESRCH') alive = false; }
    const start = await processStart(prior.pid);
    if (alive && !(start && prior.start && start !== prior.start)) throw fault('profile_volume_locked');
    await unlink(lockPath);
    lock = await open(lockPath, 'wx', 0o600);
  }
  await lock.writeFile(JSON.stringify(owner)); await lock.sync();
  const db = new DatabaseSync(join(dataDir, 'worker.sqlite'));
  db.exec(`PRAGMA journal_mode=WAL; PRAGMA synchronous=FULL;
    CREATE TABLE IF NOT EXISTS profiles (namespace TEXT NOT NULL, profile TEXT NOT NULL, epoch INTEGER NOT NULL,
      control TEXT NOT NULL, status TEXT NOT NULL, active INTEGER NOT NULL DEFAULT 0, saved TEXT,
      PRIMARY KEY(namespace,profile));
    CREATE TABLE IF NOT EXISTS receipts (namespace TEXT NOT NULL, profile TEXT NOT NULL, nonce TEXT NOT NULL,
      digest TEXT NOT NULL, status TEXT NOT NULL, code TEXT NOT NULL, PRIMARY KEY(namespace,profile,nonce));
    CREATE TABLE IF NOT EXISTS redactions (namespace TEXT NOT NULL, profile TEXT NOT NULL, sealed TEXT NOT NULL, PRIMARY KEY(namespace,profile));
    CREATE TABLE IF NOT EXISTS namespace_purges (namespace TEXT PRIMARY KEY, status TEXT NOT NULL);
    UPDATE receipts SET status='unknown',code='worker_restarted' WHERE status='started';
    UPDATE profiles SET epoch=epoch+1, control='paused', status=CASE WHEN active=1 THEN 'recovery_required' ELSE 'stopped' END, active=0;`);
  const get = db.prepare('SELECT * FROM profiles WHERE namespace=? AND profile=?');
  const put = db.prepare(`INSERT INTO profiles(namespace,profile,epoch,control,status,active,saved) VALUES(?,?,?,?,?,?,?)
    ON CONFLICT(namespace,profile) DO UPDATE SET epoch=excluded.epoch,control=excluded.control,status=excluded.status,active=excluded.active,saved=excluded.saved`);
  const receipt = db.prepare('SELECT digest,status,code FROM receipts WHERE namespace=? AND profile=? AND nonce=?');
  return {
    profile(ns, id) { return get.get(ns, id); },
    save(p) { put.run(p.namespace, p.profile, p.epoch, p.control, p.status, p.active ? 1 : 0, p.saved ?? null); },
    receipt(ns, id, nonce) { return receipt.get(ns, id, nonce); },
    start(ns, id, nonce, digest) { db.prepare('INSERT INTO receipts VALUES(?,?,?,?,?,?)').run(ns, id, nonce, digest, 'started', ''); },
    finish(ns, id, nonce, status, code = '') { db.prepare('UPDATE receipts SET status=?,code=? WHERE namespace=? AND profile=? AND nonce=?').run(status, code, ns, id, nonce); },
    redactions(ns, id) { return db.prepare('SELECT sealed FROM redactions WHERE namespace=? AND profile=?').get(ns, id)?.sealed; },
    saveRedactions(ns, id, sealed) { db.prepare('INSERT INTO redactions VALUES(?,?,?) ON CONFLICT(namespace,profile) DO UPDATE SET sealed=excluded.sealed').run(ns, id, sealed); },
    purgeStatus(ns) { return db.prepare('SELECT status FROM namespace_purges WHERE namespace=?').get(ns)?.status; },
    beginPurge(ns) {
      db.prepare("INSERT INTO namespace_purges(namespace,status) VALUES(?,'pending') ON CONFLICT(namespace) DO NOTHING").run(ns);
    },
    completePurge(ns) {
      db.exec('BEGIN IMMEDIATE');
      try {
        for (const table of ['profiles', 'receipts', 'redactions']) db.prepare(`DELETE FROM ${table} WHERE namespace=?`).run(ns);
        db.prepare("UPDATE namespace_purges SET status='purged' WHERE namespace=?").run(ns);
        db.exec('COMMIT');
      } catch (error) { db.exec('ROLLBACK'); throw error; }
    },
    async close() {
      db.close(); await lock.close();
      try { if (JSON.parse(await readFile(lockPath, 'utf8')).id === lockId) await unlink(lockPath); } catch {}
    },
  };
}
