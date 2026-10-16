# Plugin catalog focus during server updates

The unsolicited current catalog/SSE update can remove a focused action even when the UI did not submit a command. Before hiding or disabling that exact focused action, the renderer now places focus on the existing connections dialog Close control. Capability removal handles focused plugin actions, status-command actions, import and refresh the same way. It does not restore focus after an ACK or move unrelated focus.

Bounded RED: exact e2d08ea actual owned Go journal removal/SSE hid a focused builtin Remove and left BODY focused outside the open dialog. Capability-removal fixture reproduced the same outcome. The initial probe omitted required input version and timed out; it is retained separately. Corrected RED uses version1 and actual accepted POST from a separate owner API caller, not the focused UI action.

Validation is owned Go HTTP/journal plus headless Chrome and original renderer, no Core/provider/Tauri/physical phone/AT speech. Exact commands, commit, runtime hashes and individual PASS/FAIL are recorded in the sealed packet. Approved CSS/index/connections/layout are unchanged.
