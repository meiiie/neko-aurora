# Local project archive and restore

Capability `local_project_states_v1` adds `projectStates` to snapshot: `{version:1,projectId,revision,archived}`. An unchanged project has lifecycle revision0. The authority `Project.revision` remains separate: archive/restore does not rotate source pins, revoke files, stop work, cancel groups, change ACLs or delete anything.

`POST /v1/projects/{projectId}/state` accepts `action:archive|restore`, `expectedRevision`, and `clientNonce`; response `{projectState,duplicate}`. A current-revision command advances lifecycle once; exact nonce retries return current metadata before a new mutation. Stale input and wrong command-kind nonce return409. At the journal bound a new action is refused while old receipts remain readable.

`GET /v1/command-receipts?kind=project_control&clientNonce=...` returns the original typed path-derived input and current projectState, without context, credential or file bytes. Receipt lookup never mutates state.

Fresh project intake, upload and group creation validate current lifecycle and refuse archived projects. Exact existing message/upload/group retries preserve acknowledged identity; typed old task controls and group stages continue from their original resource authority. Archived project files and public history remain readable under existing current ACL/access checks. A restore explicitly reopens fresh intake. This version does not advertise a separate lifecycle CAS on intake/upload; their authority revisions remain frozen and current lifecycle is checked at new admission.

Local API and synthetic ledger receipts do not establish physical phone, cloud, real provider, native owner dialog or complete product acceptance.
