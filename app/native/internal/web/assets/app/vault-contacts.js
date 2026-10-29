"use strict";
(() => {
  const t = s => window.__I18N?.t(s) || s;
  const el = (tag, text, cls) => { const n=document.createElement(tag);if(text)n.textContent=t(text);if(cls)n.className=cls;return n; };
  const button = (text, run) => {const b=el("button",text,"pill soft");b.type="button";b.onclick=run;return b;};
  function create({item, active, change, refresh, onBusy, onChange}) {
    const root=el("section",null,"vault-contacts"),error=el("p",null,"sec-err"),status=el("p",null,"caption");
    error.setAttribute("role","alert");status.setAttribute("role","status");
    let contacts=item.contacts||{revision:"0",emails:(item.filled||[]).includes("email")?[{id:"legacy-email",hint:"••••••••"}]:[],phones:(item.filled||[]).includes("phone")?[{id:"legacy-phone",hint:"••••••••"}]:[]},busy=false,external=false;
    const editors=[];
    const current=()=>active()&&root.isConnected;
    const hasDraft=()=>editors.some(e=>e.input.value.trim()||e.entryId);
    function disabled() {
      root.querySelectorAll("button,input").forEach(n=>n.disabled=busy||external||n.dataset.fixedDisabled==="true"||(n.dataset.edit==="true"&&hasDraft()));
    }
    async function run(operation, done) {
      if(busy||external||!current())return;
      const previous=document.activeElement;let focus=previous;
      busy=true;error.textContent="";status.textContent="";onBusy(true);disabled();
      try {
        const updated=await operation();
        if(!current())return;
        if(updated?.id!==item.id||updated.type!=="identity"||!updated.contacts||typeof updated.contacts.revision!=="string"||!Array.isArray(updated.contacts.emails)||!Array.isArray(updated.contacts.phones))throw Error("contact_response");
        contacts=updated.contacts;onChange(updated);focus=done?.()||previous;renderRows();status.textContent=t("Đã cập nhật liên hệ");
      } catch(e) {
        if(current())error.textContent=t(e.status===409?"Liên hệ đã đổi. Tải lại danh sách trước khi thử lại.":e.status===400?"Kiểm tra giá trị liên hệ, mục trùng hoặc giới hạn 10 mục.":"Chưa xác nhận được thay đổi. Tải lại danh sách để kiểm tra.");
      } finally {busy=false;if(current()){onBusy(false);disabled();if(focus?.isConnected&&(document.activeElement===previous||document.activeElement===document.body))focus.focus();}}
    }
    const reload=button("Tải lại danh sách liên hệ",()=>run(refresh));reload.dataset.contactReload="";
    root.append(el("p","Email và số điện thoại được lưu ngay khi bạn bấm thêm, cập nhật, xóa hoặc chọn làm chính.","caption"));
    for(const [kind,key,title,addTitle] of [["email","emails","Email","Thêm email"],["phone","phones","Điện thoại","Thêm số điện thoại"]]) {
      const group=el("fieldset"),rows=el("div",null,"contact-rows"),label=el("label",null,"fld"),input=el("input"),entry={kind,key,rows,input,entryId:""};
      group.dataset.contactKind=kind;group.append(el("legend",title));input.type="text";input.inputMode=kind==="email"?"email":"tel";input.autocomplete="off";input.autocapitalize="off";input.spellcheck=false;input.maxLength=kind==="email"?254:64;input.setAttribute("aria-label",t(addTitle));input.setAttribute("data-1p-ignore","");input.setAttribute("data-lpignore","true");
      const reset=()=>{entry.entryId="";input.value="";input.placeholder="";entry.save.textContent=t(addTitle);entry.save.dataset.fixedDisabled=String((contacts[key]||[]).length>=10);disabled();};
      const submit=()=>{
        if(!input.value.trim()||!current()||busy||external)return;
        const value=input.value.trim(),entryId=entry.entryId;
        run(()=>change({revision:contacts.revision,kind,action:entryId?"replace":"add",entryId,value}),()=>{reset();return input});
      };
      entry.save=button(addTitle,submit);entry.save.dataset.contactSave=kind;
      input.oninput=()=>disabled();input.onkeydown=e=>{if(e.key==="Enter"){e.preventDefault();if(!e.isComposing&&e.keyCode!==229)submit()}};
      label.append(el("span",addTitle),input);const controls=el("div",null,"contact-actions");controls.append(entry.save,button("Huỷ nhập",reset));
      group.append(rows,label,controls);editors.push(entry);root.append(group);
    }
    function renderRows() {
      for(const entry of editors) {
        const list=contacts[entry.key]||[];entry.rows.replaceChildren();
        entry.save.dataset.fixedDisabled=String(!entry.entryId&&list.length>=10);
        list.forEach((point,index)=>{
          const row=el("div",null,"contact-row"),hint=el("code"),actions=el("div",null,"contact-actions");row.dataset.contactId=point.id;hint.dataset.noi18n="";hint.textContent=point.hint;
          row.append(hint);if(index===0)row.append(el("small","Chính","contact-primary"));
          const primary=button("Làm chính",()=>run(()=>change({revision:contacts.revision,kind:entry.kind,action:"primary",entryId:point.id}),()=>reload));primary.dataset.contactPrimary="";primary.dataset.fixedDisabled=String(index===0);
          const edit=button("Sửa",()=>{if(busy||external||hasDraft()||!current())return;entry.entryId=point.id;entry.input.placeholder=point.hint;entry.save.textContent=t("Cập nhật giá trị");entry.save.dataset.fixedDisabled="false";entry.input.focus();disabled()});edit.dataset.edit="true";
          const remove=button("Xoá",()=>{if(busy||external||!current()||!confirm(t("Xóa giá trị liên hệ này?")))return;run(()=>change({revision:contacts.revision,kind:entry.kind,action:"remove",entryId:point.id}),()=>reload)});remove.dataset.contactRemove="";
          actions.append(primary,edit,remove);row.append(actions);entry.rows.append(row);
        });
      }
      disabled();
    }
    root.append(error,status,reload);renderRows();
    return {root,hasDraft,setBusy(value){external=value;disabled()},clear(){editors.forEach(e=>{e.input.value="";e.entryId=""})}};
  }
  window.AURORA_VAULT_CONTACTS={create};
})();
