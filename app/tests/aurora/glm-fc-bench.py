import json,os,time,urllib.request,re
URL="https://api.z.ai/api/coding/paas/v4/chat/completions";K=os.environ["ZAI_API_KEY"]
tools=[{"type":"function","function":{"name":"web_search","description":"Tìm trên web","parameters":{"type":"object","properties":{"q":{"type":"string"}},"required":["q"]}}},
{"type":"function","function":{"name":"gold_price","description":"Giá vàng thế giới và SJC hiện tại","parameters":{"type":"object","properties":{}}}},
{"type":"function","function":{"name":"web_fetch","description":"Đọc trang https","parameters":{"type":"object","properties":{"url":{"type":"string"}},"required":["url"]}}}]
SYS_DIR='Bạn là trợ lý tiếng Việt. Khi cần dữ liệu hiện tại, in dòng TOOL: {"name":"web_search","q":"..."} | {"name":"gold_price"} | {"name":"web_fetch","url":"https://..."}; tối đa 3 dòng TOOL, rồi dừng. Không cần dữ liệu thì trả lời luôn.'
SYS_FC='Bạn là trợ lý tiếng Việt. Gọi công cụ khi cần dữ liệu hiện tại (có thể gọi song song). Không cần thì trả lời luôn.'
tasks=[("Giá vàng SJC hôm nay bao nhiêu?",{"gold_price"}),("Tỷ giá USD/VND và giá iPhone 17 Pro ở VN hôm nay?",{"web_search"}),("Viết 2 câu chúc sinh nhật mẹ.",set()),("Tóm tắt trang https://vnexpress.net/kinh-doanh",{"web_fetch"}),("So sánh giá vàng SJC và giá bitcoin hôm nay",{"gold_price","web_search"}),("2+2 bằng mấy?",set())]
def call(sys,u,fc):
  body={"model":"glm-5.3","messages":[{"role":"system","content":sys},{"role":"user","content":u}],"temperature":0.6,"thinking":{"type":"disabled"}}
  if fc: body["tools"]=tools
  r=urllib.request.Request(URL,json.dumps(body).encode(),{"Content-Type":"application/json","Authorization":"Bearer "+K})
  t=time.time();d=json.load(urllib.request.urlopen(r,timeout=120));dt=time.time()-t
  m=d["choices"][0]["message"]
  if fc: got={c["function"]["name"] for c in (m.get("tool_calls") or [])}
  else: got={json.loads(x)["name"] for x in re.findall(r'TOOL:\s*(\{.*\})',m.get("content") or "") if x}
  return got,dt
res={"directive":[0,0],"native":[0,0]}
for u,want in tasks:
  for mode in ("directive","native"):
    try: got,dt=call(SYS_FC if mode=="native" else SYS_DIR,u,mode=="native")
    except Exception as e: got,dt={"ERR:"+str(e)[:60]},0
    ok=(got==want) if want else not got
    res[mode][0]+=ok;res[mode][1]+=dt
    print(mode,"|",u[:40],"| want",sorted(want),"got",sorted(got),"ok",ok,"%.1fs"%dt)
print("SCORE",{k:(v[0],round(v[1],1)) for k,v in res.items()})
