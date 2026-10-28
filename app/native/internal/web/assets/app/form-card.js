"use strict";
(() => {
  const drafts = new Map(), PREFIX = "aurora.forms.v1.";
  const el = (tag, text, cls) => {const n=document.createElement(tag);if(text!=null)n.textContent=text;if(cls)n.className=cls;return n};
  const data = (tag, text) => {const n=el(tag,text);n.dataset.noi18n="";return n};
  function create(options) {
    const {message:m,pending,answered,answerText,answerValues,workspaceId,storage,send,active}=options, f=m.form;
    const prefix=PREFIX+encodeURIComponent(workspaceId)+".", key=prefix+encodeURIComponent(JSON.stringify([m.agentId,m.taskId]));
    const schema=JSON.stringify([f.answerVersion||0,f.title,f.fields]), done=!!(f.done||answered), locked=!!(done||pending);
    const root=el("form",null,"card form-card");root.dataset.id=m.taskId;root.setAttribute("aria-label","Biểu mẫu cần bạn điền");
    const defaults=()=>f.fields.map(x=>!f.answerVersion&&x.type==="select"?(x.options?.[0]||""):"");
    let raw=null, values=defaults(), blocked=false, readFailed=false, problem="", inputs=[], status, go, clear;
    const valid=s=>s&&s.schema===schema&&Array.isArray(s.values)&&s.values.length===f.fields.length&&s.values.every(x=>typeof x==="string"&&x.length<=2000);
    if(!locked) {
      try {
        raw=storage.getItem(key);
        const memory=drafts.get(key);
        let state=memory?.base===raw?memory:null;
        if(!state&&raw!=null){try{state=raw.length<=100000?JSON.parse(raw):false}catch{state=false}}
        if(state!=null||raw!=null) {
          if(!valid(state)){blocked=true;problem="Bản nháp cũ không khớp biểu mẫu hoặc không đọc được. Xem lại trước khi bỏ."}
          else {values=[...state.values];if(memory?.failed)problem="Chưa lưu được trên thiết bị. Nội dung đang được giữ trong cửa sổ này."}
        }
      } catch {blocked=true;readFailed=true;problem="Chưa đọc được bản nháp trên thiết bị. Tải lại khi bộ nhớ khả dụng."}
    }
    if(pending?.formValues?.length===f.fields.length) values=pending.formValues.map(x=>typeof x==="string"?x:"");
    if(done){drafts.delete(key);try{storage.removeItem(key)}catch{}}
    function remember(failed) {
      if(!drafts.has(key)&&drafts.size>=128){const saved=[...drafts].find(([,draft])=>!draft.failed);if(saved)drafts.delete(saved[0])}
      if(!drafts.has(key)&&drafts.size>=128)return false;
      drafts.set(key,{schema,values:[...values],base:raw,failed});return true;
    }
    function save() {
      if(blocked||!active()||!root.isConnected)return false;
      values=inputs.map(i=>i.value);const remembered=remember(true);
      try {
        if(storage.getItem(key)!==raw){blocked=true;throw Error("changed")}
        if(raw==null){let count=0;for(let i=0;i<storage.length;i++)if(storage.key(i)?.startsWith(prefix))count++;if(count>=128)throw Error("full")}
        const text=JSON.stringify({schema,values});
        if(text.length>100000)throw Error("full");
        storage.setItem(key,text);if(storage.getItem(key)!==text)throw Error("not_saved");
        raw=text;remember(false);if(clear)clear.hidden=false;status.textContent="Đã lưu bản nháp trên thiết bị.";return true;
      } catch {
        status.textContent=blocked?"Bản nháp đã đổi ở cửa sổ khác. Tải lại để xem trước khi gửi.":remembered?"Chưa lưu được trên thiết bị. Nội dung đang được giữ trong cửa sổ này.":"Chưa lưu được: bộ nhớ tạm đã đầy. Sao chép nội dung trước khi rời biểu mẫu.";
        if(clear)clear.hidden=false;
        if(blocked)go.disabled=true;
        return false;
      }
    }
    function render() {
      root.replaceChildren();inputs=[];root.append(data("h4",f.title));
      if(done){
        if(Array.isArray(answerValues)&&answerValues.length===f.fields.length&&answerValues.every(v=>typeof v==="string")){
          const result=el("dl",null,"form-result");result.tabIndex=0;result.setAttribute("aria-label","Câu trả lời đã gửi");
          f.fields.forEach((field,i)=>{const value=data("dd",answerValues[i]);if(!answerValues[i].trim()){value.replaceChildren(el("em","Bỏ trống"))}result.append(data("dt",field.label),value)});root.append(result);
        }else if(typeof answerText==="string"&&answerText){const prefix="📋 "+f.title+"\n",result=data("pre",answerText.startsWith(prefix)?answerText.slice(prefix.length):answerText);result.className="form-result";result.tabIndex=0;result.setAttribute("aria-label","Câu trả lời đã gửi");root.append(result)}
        go=el("button","Đã gửi","pill");go.type="submit";go.disabled=true;root.append(go);return;
      }
      f.fields.forEach((x,i)=>{
        const label=el("label",null,"fm-row");label.append(data("span",x.label+(x.required?" *":"")));
        let input;
        if(x.type==="select"){
          input=el("select");if(f.answerVersion===1){const empty=el("option","Chưa chọn");empty.value="";input.append(empty)}
          for(const option of x.options||[]){const node=data("option",option);node.value=option;input.append(node)}
        }else if(x.type==="textarea"){input=el("textarea");input.rows=3}
        else{input=el("input");input.type=x.type==="number"?"number":x.type==="date"?"date":"text";if(x.type==="number")input.step="any";if(x.type==="date"){input.min="0001-01-01";input.max="9999-12-31"}}
        input.name="f"+i;input.maxLength=2000;input.required=!!x.required;input.disabled=locked||blocked;input.value=values[i]||"";
        input.oninput=save;input.onchange=save;label.append(input);inputs.push(input);root.append(label);
      });
      go=el("button",done?"Đã gửi":pending?(pending.error?"Chưa được nhận":"Đang chờ xác nhận…"):"Gửi","pill");go.type="submit";go.disabled=locked||blocked;root.append(go);
      status=el("p",problem,"caption form-draft-status");status.setAttribute("role","status");status.setAttribute("aria-live","polite");root.append(status);
      if(!locked&&!blocked){
        clear=el("button","Xóa bản nháp","pill soft");clear.type="button";clear.hidden=raw==null&&!drafts.has(key);root.append(clear);
        clear.onclick=()=>{
          if(!active()||!root.isConnected)return;
          try{if(storage.getItem(key)!==raw)throw Error("changed");storage.removeItem(key);if(storage.getItem(key)!=null)throw Error("not_removed");drafts.delete(key);raw=null;values=defaults();problem="Đã xóa bản nháp trên thiết bị.";render();inputs[0]?.focus()}
          catch{status.textContent="Chưa xóa được bản nháp. Tải lại để kiểm tra."}
        };
      }
      if(blocked&&!readFailed){
        const details=el("details"),summary=el("summary","Xem bản nháp cũ"),preview=el("pre",raw?.slice(0,100000)||"", "form-draft-preview");preview.dataset.noi18n="";details.append(summary,preview);root.append(details);
        const reset=el("button","Bỏ bản nháp cũ","pill soft");reset.type="button";reset.onclick=()=>{
          if(!active()||!root.isConnected)return;
          try{if(storage.getItem(key)!==raw)throw Error("changed");storage.removeItem(key);if(storage.getItem(key)!=null)throw Error("not_removed");drafts.delete(key);raw=null;values=defaults();blocked=false;problem="";render();inputs[0]?.focus()}
          catch{status.textContent="Chưa bỏ được bản nháp. Tải lại để kiểm tra."}
        };root.append(reset);
      }
    }
    root.onsubmit=event=>{
      event.preventDefault();if(locked||blocked||!active()||!root.isConnected||!root.reportValidity()||!save())return;
      const text="📋 "+f.title+"\n"+f.fields.map((x,i)=>"- "+x.label+": "+(values[i].trim()||"(bỏ trống)")).join("\n");
      send(text,{formFor:m.taskId,formValues:[...values],formAnswerVersion:f.answerVersion===1?1:undefined});
    };
    render();return root;
  }
  globalThis.AURORA_FORMS={create};
})();
