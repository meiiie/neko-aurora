# Local runtime authority continuation

Baseline: e0e77ca. Preserve archives, other workers' assets and the unaccepted light UI branch.

1. Retain account nonce receipts after edit/removal. Separate account revision from credential generation. Pin a selected account at intake and issue a durable bounded attempt lease at claim. Hold a live account guard through journal publication and artifact write. Suspend on removal, revision/generation change or expiry; never choose another account implicitly. Replay historical descriptors without consulting today's catalog.
2. Keep local fixture execution explicit. This work does not verify a provider, unseal a saved key into Core, create a third-party grant or enable remote endpoints.
3. Preserve the source channel for colleague tags. Pin provenance and publication audience; include only history visible in that channel. Test two bots and two local channels, negative private-history and late-publication cases.
4. Import delivered light UI into a separate branch with existing font support. Record combined tests and preserve visual acceptance as pending. Checkpoint completed audit artifacts only after ownership/hash/secret review.
5. Investigate the Core writer EPERM in isolated clones using a reproducible failing case. Keep the current Core pin until the candidate meets its gates.

Validation: failing behavioral gates first, scoped Go suites/vet/build, native synthetic Core tests where applicable, independent authority audit, exact source commits and raw evidence. Release matrix distinguishes implementation, fixture verification, user visual review, physical phone and deferred provider/cloud gates.
