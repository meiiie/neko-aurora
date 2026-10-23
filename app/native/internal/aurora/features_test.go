package aurora

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func getJSON(t *testing.T, url string, v any) {
	t.Helper()
	r, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	if r.StatusCode != 200 || json.Unmarshal(b, v) != nil {
		t.Fatalf("%s: %d %s", url, r.StatusCode, b)
	}
}

func TestSearchMemoryUsageRoutes(t *testing.T) {
	svc, u, bot := liveSvc(t, "Mình đã tóm tắt bài báo về Đà Nẵng.")
	post(t, u+"/app/api/agents/"+bot+"/messages", `{"text":"Tóm tắt giúp mình bài báo này","clientNonce":"s1"}`)
	waitTask(t, svc, bot)
	var sr struct{ Hits []searchHit }
	getJSON(t, u+"/app/api/search?q=da%20nang", &sr)
	if len(sr.Hits) != 1 || sr.Hits[0].Role != "agent" || sr.Hits[0].AgentID != bot {
		t.Fatalf("%+v", sr.Hits)
	}
	getJSON(t, u+"/app/api/search?q=tom%20tat", &sr)
	if len(sr.Hits) != 2 {
		t.Fatalf("%+v", sr.Hits)
	}
	// memory panel: add own + team, edit, delete, journal
	if r := post(t, u+"/app/api/agents/"+bot+"/memory", `{"text":"Darren thích bảng"}`); r.StatusCode != 200 {
		t.Fatal(r.StatusCode)
	}
	post(t, u+"/app/api/agents/"+bot+"/memory", `{"text":"Gọi chủ nhân là anh Darren","team":true}`)
	if r := post(t, u+"/app/api/agents/"+bot+"/memory", `{"text":"api key là sk-1"}`); r.StatusCode < 400 {
		t.Fatal("secret saved")
	}
	post(t, u+"/app/api/agents/"+bot+"/memory/0", `{"text":"Darren thích bảng ngắn"}`)
	req, _ := http.NewRequest("DELETE", u+"/app/api/agents/"+bot+"/memory/0?scope=team", nil)
	http.DefaultClient.Do(req)
	var mem struct {
		Items, Team []memItem
		Journal     []memChange
	}
	getJSON(t, u+"/app/api/agents/"+bot+"/memory", &mem)
	if len(mem.Items) != 1 || mem.Items[0].Text != "Darren thích bảng ngắn" || len(mem.Team) != 0 || len(mem.Journal) < 4 {
		t.Fatalf("%+v", mem)
	}
	var us struct {
		Total usageSum
		Kinds []usageSum
		Names map[string]string
	}
	getJSON(t, u+"/app/api/usage?range=1", &us)
	if us.Total.Calls < 1 || us.Total.Tokens == 0 || !strings.Contains(us.Names[bot], "") {
		t.Fatalf("%+v", us)
	}
}
