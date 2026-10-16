import {randomUUID} from 'node:crypto';
export async function pluginSseFocusCases({evaluate,api,until,check,call,base}){
 await evaluate('document.getElementById("connections-open").click();document.getElementById("plugins-tab").click()');await until(()=>evaluate('pluginCards.has("neko-artifacts")'));
 await evaluate('pluginCards.get("neko-artifacts").remove.focus()');check('initial_remove_keyboard_focus',await evaluate('document.activeElement===pluginCards.get("neko-artifacts").remove'));
 const p=await evaluate('pluginRecords.get("neko-artifacts")');const result=await api('/v1/plugin-lifecycle',{action:'remove',version:1,id:p.id,expectedRevision:p.revision,clientNonce:randomUUID()});check('actual_external_api_remove_accepted',result.status===200);await until(()=>evaluate('pluginCards.get("neko-artifacts").remove.hidden'));
 check('actual_unsolicited_removed_sse_preserves_dialog_focus',result.status===200&&await evaluate('document.getElementById("connections-dialog").contains(document.activeElement)&&document.activeElement!==document.body'));
 await evaluate('pluginCards.get("neko-project-sources").toggle.focus();state.capabilities=state.capabilities.filter(c=>c!=="local_plugin_lifecycle_v1");renderPluginCatalog()');
 check('capability_hiding_focused_controls_preserves_dialog_focus',await evaluate('document.getElementById("connections-dialog").contains(document.activeElement)&&document.activeElement!==document.body'));
 await evaluate('state.capabilities.push("local_plugin_lifecycle_v1");renderPluginCatalog();document.getElementById("connections-close").click();document.getElementById("prompt").value="Unrelated draft";document.getElementById("prompt").focus();document.getElementById("prompt").setSelectionRange(2,4);acceptPluginRecord({...pluginRecords.get("neko-project-sources"),revision:1,status:"removed"});renderPluginCatalog()');
 check('unrelated_focus_draft_unchanged_by_catalog_update',await evaluate('document.activeElement.id==="prompt"&&document.getElementById("prompt").value==="Unrelated draft"&&document.getElementById("prompt").selectionStart===2&&document.getElementById("prompt").selectionEnd===4'));
}
