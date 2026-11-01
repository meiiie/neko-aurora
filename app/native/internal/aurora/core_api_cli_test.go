package aurora

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nekoaurora.local/native/internal/privatefs"
)

func TestCoreActualCLIAPIProviderRoutes(t *testing.T) {
	bun := findCoreBun()
	if bun == "" {
		t.Skip("Bun unavailable; real Core API CLI not exercised")
	}
	entry, err := filepath.Abs("../../../../core/bin/neko.ts")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		id, model, keyEnv, catalog, completion, header string
		anthropic                                      bool
	}{
		{"zai", "glm-account", "ZAI_API_KEY", "https://api.z.ai/api/coding/paas/v4/models", "https://api.z.ai/api/anthropic/v1/messages", "x-api-key", true},
		{"zai-openai", "glm-api-account", "ZAI_API_KEY", "https://api.z.ai/api/paas/v4/models", "https://api.z.ai/api/paas/v4/chat/completions", "authorization", false},
		{"openai", "gpt-account", "OPENAI_API_KEY", "https://api.openai.com/v1/models", "https://api.openai.com/v1/chat/completions", "authorization", false},
		{"claude", "claude-account", "ANTHROPIC_API_KEY", "https://api.anthropic.com/v1/models", "https://api.anthropic.com/v1/messages", "x-api-key", true},
		{"gemini-api", "gemini-account", "GEMINI_API_KEY", "https://generativelanguage.googleapis.com/v1beta/models", "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions", "authorization", false},
		{"deepseek", "deepseek-account", "DEEPSEEK_API_KEY", "https://api.deepseek.com/models", "https://api.deepseek.com/chat/completions", "authorization", false},
	} {
		t.Run(tc.id, func(t *testing.T) {
			root := t.TempDir()
			home, bin := filepath.Join(root, "home"), filepath.Join(root, "bin")
			for _, dir := range []string{home, bin} {
				if err := os.MkdirAll(dir, 0700); err != nil {
					t.Fatal(err)
				}
			}
			if err := privatefs.ProtectDirectory(home); err != nil {
				t.Fatal(err)
			}
			fixture, _ := json.Marshal(map[string]any{"provider": tc.id, "model": tc.model, "catalog": tc.catalog, "completion": tc.completion, "header": tc.header, "anthropic": tc.anthropic})
			entryJSON, _ := json.Marshal(entry)
			setupJSON, _ := json.Marshal(filepath.Join(filepath.Dir(entry), "aurora-setup.ts"))
			bootstrap := `import {pathToFileURL} from "node:url";
import {writeFileSync} from "node:fs";
import {join} from "node:path";
const f=` + string(fixture) + `;
globalThis.fetch=async(input,init)=>{
 const url=new URL(String(input)),headers=new Headers(init?.headers);
 if(url.origin+url.pathname===f.catalog){
  const h=f.provider==="gemini-api"?"x-goog-api-key":f.provider==="claude"?"x-api-key":"authorization";
  if(headers.get(h)!==(h==="authorization"?"Bearer fixture-api-secret":"fixture-api-secret"))throw Error("wrong catalog credential");
  return Response.json(f.provider==="gemini-api"?{models:[{name:"models/"+f.model,supportedGenerationMethods:["generateContent"]}]}:{data:[{id:f.model}]});
 }
 if(url.href!==f.completion)throw Error("Unexpected fixture destination");
 const body=JSON.parse(String(init?.body));
 if(body.model!==f.model||(body.tools?.length??0)!==0||headers.get(f.header)!==(f.header==="authorization"?"Bearer fixture-api-secret":"fixture-api-secret"))throw Error("wrong model, tools or credential");
 writeFileSync(join(process.env.HOME,"reached.json"),JSON.stringify({model:body.model}));
 const events=f.anthropic?[
  {type:"message_start",message:{id:"fixture",type:"message",role:"assistant",model:f.model,content:[],usage:{input_tokens:1,output_tokens:0}}},
  {type:"content_block_start",index:0,content_block:{type:"text",text:""}},
  {type:"content_block_delta",index:0,delta:{type:"text_delta",text:"Provider fixture reply."}},
  {type:"content_block_stop",index:0},
  {type:"message_delta",delta:{stop_reason:"end_turn"},usage:{output_tokens:4}},
  {type:"message_stop"}
 ]:[{choices:[{index:0,delta:{content:"Provider fixture reply."}}]},{choices:[{index:0,delta:{},finish_reason:"stop"}],usage:{prompt_tokens:1,completion_tokens:4}}];
 return new Response(events.map(e=>(f.anthropic?"event: "+e.type+"\n":"")+"data: "+JSON.stringify(e)+"\n\n").join("")+(f.anthropic?"":"data: [DONE]\n\n"),{headers:{"content-type":"text/event-stream"}});
};
await import(pathToFileURL(` + string(entryJSON) + `).href);
`
			fixtureEntry := filepath.Join(bin, "neko-fixture.ts")
			for name, data := range map[string][]byte{fixtureEntry: []byte(bootstrap), filepath.Join(root, "bunfig.neko.toml"): {}, filepath.Join(bin, "aurora-setup.ts"): []byte(strings.Replace(bootstrap, string(entryJSON), string(setupJSON), 1))} {
				if err := os.WriteFile(name, data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			c := &Core{Bun: bun, Entry: fixtureEntry, Home: home, Profile: tc.id, Model: tc.model, KeyEnv: tc.keyEnv, key: "fixture-api-secret", Quick: true}
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			var streamed strings.Builder
			reply, err := c.Stream(ctx, []ChatMsg{{Role: "user", Content: "Fixture only."}}, func(s string) { streamed.WriteString(s) })
			if err != nil || strings.TrimSpace(reply) != "Provider fixture reply." || reply != streamed.String() {
				t.Fatalf("CLI reply %q, error %v", reply, err)
			}
			if _, err := os.Stat(filepath.Join(home, "reached.json")); err != nil {
				t.Fatal("completion route not reached", err)
			}
		})
	}
}
