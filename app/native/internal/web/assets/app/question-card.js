"use strict";
(() => {
  const drafts = new Map();
  const el = (tag, text, cls) => {const n=document.createElement(tag);if(text!=null)n.textContent=text;if(cls)n.className=cls;return n};
  const data = (tag, text) => {const n=el(tag,text);n.dataset.noi18n="";return n};
  function create({message:m, pending, answered, workspaceId, storage, send, notice}) {
    const questions=m.questions, schema=JSON.stringify(questions);
    const key="aurora.questions.v1."+encodeURIComponent(JSON.stringify([workspaceId,m.agentId,m.taskId]));
    let state=drafts.get(key);
    if(!state){try{state=JSON.parse(storage.getItem(key))}catch{}}
    if(!state||state.schema!==schema||!Array.isArray(state.answers)||state.answers.length!==questions.length||!state.answers.every((a,i)=>a&&a.id===questions[i].id&&typeof a.text==="string"&&a.text.length<=8000&&Array.isArray(a.selected)&&a.selected.every(n=>Number.isInteger(n)&&n>=0&&n<questions[i].options.length)))state={schema,step:0,answers:questions.map(q=>({id:q.id,selected:[],text:"",own:false}))};
    state.step=Number.isInteger(state.step)?Math.max(0,Math.min(questions.length,state.step)):0;
    const root=el("section",null,"card question-card");root.dataset.id=m.taskId;root.setAttribute("aria-label","Câu hỏi cần bạn trả lời");
    const valid=a=>a.own?!!a.text.trim():a.selected.length>0;
    const answers=()=>state.answers.map(a=>({id:a.id,selected:a.own?[]:a.selected,text:a.own?a.text:""}));
    const text=as=>"Trả lời câu hỏi:\n\n"+questions.map((q,i)=>q.prompt+"\n"+(as[i].text.trim()||q.options.filter((_,n)=>as[i].selected.includes(n)).join("; "))).join("\n\n");
    function save(){drafts.set(key,state);try{const raw=JSON.stringify(state);storage.setItem(key,raw);if(storage.getItem(key)!==raw)throw Error("not_saved")}catch{notice("Câu trả lời mới chỉ được giữ trong cửa sổ này. Chưa lưu được trên thiết bị.")}}
    function summary(as){const dl=el("dl");questions.forEach((q,i)=>{dl.append(data("dt",q.prompt),data("dd",as[i]?.text?.trim()||q.options.filter((_,n)=>as[i]?.selected?.includes(n)).join("; ")))});root.append(dl)}
    function render(){
      root.replaceChildren();
      if(answered){root.append(el("p","Đã trả lời","question-status"));drafts.delete(key);try{storage.removeItem(key)}catch{};return}
      if(pending){root.append(el("p",pending.error?"Chưa được nhận":"Đang chờ xác nhận…","question-status"));summary(pending.questionAnswers||answers());return}
      const progress=el("p",state.step===questions.length?"Xem lại câu trả lời":`Câu ${state.step+1}/${questions.length}`,"caption");progress.setAttribute("aria-live","polite");progress.tabIndex=-1;root.append(progress);
      const controls=el("div",null,"question-controls"),back=el("button","Quay lại","pill soft"),next=el("button",state.step===questions.length?"Gửi câu trả lời":state.step===questions.length-1?"Xem lại":"Tiếp","pill");
      back.type=next.type="button";back.disabled=state.step===0;back.onclick=()=>{state.step--;save();render();root.querySelector(".caption")?.focus()};
      if(state.step===questions.length){summary(answers());next.disabled=!state.answers.every(valid);next.onclick=()=>{if(state.answers.every(valid))send(text(answers()),{answerFor:m.taskId,questionAnswers:answers()})}}
      else {
        const q=questions[state.step],a=state.answers[state.step],fieldset=el("fieldset"),legend=data("legend",q.prompt);fieldset.append(legend);
        if(q.multiple)fieldset.append(el("p","Có thể chọn nhiều phương án.","caption"));
        q.options.forEach((option,index)=>{const label=el("label",null,"question-option"),input=el("input");input.type=q.multiple?"checkbox":"radio";input.name=m.taskId+"-"+q.id;input.value=String(index);input.checked=!a.own&&a.selected.includes(index);
          input.onchange=()=>{a.own=false;a.text="";a.selected=q.multiple?(input.checked?[...new Set([...a.selected,index])]:a.selected.filter(n=>n!==index)):[index];save();render();root.querySelector(`input[value="${index}"]`)?.focus()};label.append(input,data("span",option));fieldset.append(label)});
        const own=el("button","Câu trả lời riêng","pill soft");own.type="button";own.setAttribute("aria-pressed",!!a.own);own.onclick=()=>{a.own=true;a.selected=[];save();render();root.querySelector("textarea")?.focus()};fieldset.append(own);
        if(a.own){const input=el("textarea");input.rows=3;input.maxLength=2000;input.value=a.text;input.setAttribute("aria-label","Câu trả lời riêng");input.oninput=()=>{a.text=input.value;save();next.disabled=!valid(a)};fieldset.append(input)}
        root.append(fieldset);next.disabled=!valid(a);next.onclick=()=>{if(valid(a)){state.step++;save();render();root.querySelector(".caption")?.focus()}};
      }
      controls.append(back,next);root.append(controls);
    }
    render();return root;
  }
  globalThis.AURORA_QUESTIONS={create};
})();
