# Exact synthetic export probe classification

Checkpoint cb69ddb ran the scanner but the PowerShell command continued to commit after a nonzero scanner exit. The matched value was a deliberately constructed synthetic assignment in the unchanged independent L04 regression probe, not a credential. This sequencing mistake is recorded; subsequent checkpoints gate Git mutation on the scanner result in a separate checked call.

Original private probe and tracked copy are byte-identical SHA256 `b214b0a94bc41a97210f0d59f45aea2cf7204d1ad072a16a8f77e75f6f44589f`. The flagged line builds a canary with `strings.Repeat`; the real test uses controlled local store records and prints no canary value. Original negative and corrected positive evidence remain separate. No auth files, credentials, raw state or private fixture data were staged.

The scanner accepts only that exact LF-normalized probe hash under its exact test path. Any changed source fails the reviewed-hash check. This does not exempt arbitrary tests, credential strings or journal data. Existing font hash policy and all other scanner rules remain unchanged.

The same narrow classification applies to the unchanged L05 metadata probe SHA256 `7d2b4759de4b32f8bccdab3cc07121cdecf6ff762b286457193c4c672fb85a1d`. Its flagged assignment at line140 is also built with strings.Repeat in synthetic local records; original private and tracked bytes match. No matched value is printed. Unlike the earlier sequencing mistake, this finding was classified before its Git checkpoint.
