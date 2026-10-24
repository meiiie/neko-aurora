package aurora

// Server-side form fill for an approved Vault use: a fresh headless Chrome
// driven over --remote-debugging-pipe (CDP on fds 3/4, NUL-framed JSON, no
// websocket or extra dependency). Values go straight from the vault into the
// page; only field names, the resulting title and redacted text come back.

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"nekoaurora.local/native/internal/tools"
)

// fillURLOK guards the target (tests point it at a local page).
var fillURLOK = func(u string) bool { return tools.PublicURL(u, true) }

type fillResult struct {
	Filled    []string
	Submitted bool
	Title     string
	Text      string
	URL       string
}

type cdp struct {
	w    *os.File
	mu   sync.Mutex
	id   int
	wait map[int]chan json.RawMessage
	errs map[int]string
}

func (c *cdp) call(ctx context.Context, session, method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	c.id++
	id := c.id
	ch := make(chan json.RawMessage, 1)
	c.wait[id] = ch
	c.mu.Unlock()
	msg := map[string]any{"id": id, "method": method, "params": params}
	if session != "" {
		msg["sessionId"] = session
	}
	b, _ := json.Marshal(msg)
	if _, err := c.w.Write(append(b, 0)); err != nil {
		return nil, err
	}
	select {
	case r := <-ch:
		c.mu.Lock()
		e := c.errs[id]
		c.mu.Unlock()
		if e != "" {
			return nil, errors.New(e)
		}
		return r, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *cdp) read(r *os.File) {
	br := bufio.NewReaderSize(r, 1<<20)
	for {
		b, err := br.ReadBytes(0)
		if err != nil {
			return
		}
		var m struct {
			ID     int             `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(b[:len(b)-1], &m) != nil || m.ID == 0 {
			continue
		}
		c.mu.Lock()
		if m.Error != nil {
			c.errs[m.ID] = m.Error.Message
		}
		ch := c.wait[m.ID]
		delete(c.wait, m.ID)
		c.mu.Unlock()
		if ch != nil {
			ch <- m.Result
		}
	}
}

// fillJS fills matching inputs (autocomplete/name/id/label heuristics) with
// native setters + input/change events, then submits the form it touched.
const fillJS = `(function(T,V){
const R={card:{number:/cc-number|card.?num|cardnumber|số thẻ|so the/i,name:/cc-name|name.?on|card.?holder|chủ thẻ|tên trên thẻ/i,exp:/cc-exp|expir|exp.?date|mm.?\/?.?yy|hết hạn|het han/i,cvc:/cvc|cvv|csc|security|mã bảo mật/i,zip:/postal|zip/i},
login:{username:/user|email|login|account|tài khoản|tai khoan/i,password:/pass|mật khẩu|mat khau/i,totp:/one-time|otp|totp|2fa|verification|code|mã xác/i},
identity:{email:/e-?mail/i,phone:/tel|phone|mobile|điện thoại/i,postal:/postal|zip/i,city:/city|locality|address-level2|thành phố/i,country:/country|quốc gia/i,address:/street|address|địa chỉ/i,name:/name|họ.?tên|ho.?ten/i}}[T]||{};
const desc=e=>{let l="";if(e.id){const x=document.querySelector('label[for="'+CSS.escape(e.id)+'"]');if(x)l=x.textContent}if(!l&&e.closest("label"))l=e.closest("label").textContent;return [e.getAttribute("autocomplete"),e.name,e.id,e.placeholder,e.getAttribute("aria-label"),l].filter(Boolean).join(" ")};
const els=[...document.querySelectorAll("input,select,textarea")].filter(e=>!e.disabled&&!e.readOnly&&!/^(hidden|submit|button|checkbox|radio|file)$/.test(e.type));
const done=[],used=new Set();let form=null;
for(const k of Object.keys(R)){if(!V[k])continue;const e=els.find(e=>!used.has(e)&&(k==="password"?e.type==="password":(R[k].test(desc(e))&&!(k!=="password"&&e.type==="password"))));if(!e)continue;used.add(e);
const p=Object.getOwnPropertyDescriptor(Object.getPrototypeOf(e),"value");p&&p.set?p.set.call(e,V[k]):(e.value=V[k]);e.dispatchEvent(new Event("input",{bubbles:true}));e.dispatchEvent(new Event("change",{bubbles:true}));done.push(k);form=form||e.form}
let sub=false;if(form&&done.length){const b=form.querySelector('[type=submit],button:not([type])');if(form.requestSubmit){form.requestSubmit(b||undefined)}else if(b){b.click()}else{form.submit()}sub=true}
return JSON.stringify({done,sub})})`

func (s *Service) browserFill(ctx context.Context, raw, typ string, vals map[string]string) (fillResult, error) {
	var out fillResult
	if !fillURLOK(raw) {
		return out, errors.New("chỉ điền vào trang web công khai (https)")
	}
	bin := browserBin()
	if bin == "" {
		return out, errors.New("máy chủ này không có trình duyệt để điền form")
	}
	dir, err := os.MkdirTemp("", "aurora-fill-")
	if err != nil {
		return out, err
	}
	defer os.RemoveAll(dir)
	inR, inW, _ := os.Pipe()   // we write, Chrome reads fd 3
	outR, outW, _ := os.Pipe() // Chrome writes fd 4, we read
	cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	args := []string{"--headless=new", "--disable-gpu", "--no-first-run", "--no-default-browser-check", "--disable-extensions", "--disable-sync", "--mute-audio", "--password-store=basic", "--disable-dev-shm-usage", "--user-data-dir=" + dir, "--remote-debugging-pipe", "about:blank"}
	if os.Getuid() == 0 || Env("BROWSER_NOSANDBOX") == "1" {
		args = append([]string{"--no-sandbox"}, args...)
	}
	if hosted() {
		args = append([]string{"--host-resolver-rules=MAP localhost ~NOTFOUND, MAP *.local ~NOTFOUND, MAP *.internal ~NOTFOUND"}, args...)
	}
	cmd := exec.CommandContext(cctx, bin, args...)
	cmd.Env = []string{"HOME=" + dir, "PATH=" + os.Getenv("PATH"), "LANG=C.UTF-8"}
	cmd.ExtraFiles = []*os.File{inR, outW}
	if err := cmd.Start(); err != nil {
		return out, errors.New("trình duyệt không mở được")
	}
	inR.Close()
	outW.Close()
	defer func() { inW.Close(); outR.Close(); _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	c := &cdp{w: inW, wait: map[int]chan json.RawMessage{}, errs: map[int]string{}}
	go c.read(outR)
	var tg struct {
		TargetID string `json:"targetId"`
	}
	r, err := c.call(cctx, "", "Target.createTarget", map[string]any{"url": "about:blank"})
	if err != nil || json.Unmarshal(r, &tg) != nil {
		return out, errors.New("trình duyệt không mở được trang")
	}
	var at struct {
		SessionID string `json:"sessionId"`
	}
	r, err = c.call(cctx, "", "Target.attachToTarget", map[string]any{"targetId": tg.TargetID, "flatten": true})
	if err != nil || json.Unmarshal(r, &at) != nil {
		return out, errors.New("trình duyệt không mở được trang")
	}
	sid := at.SessionID
	eval := func(expr string) (string, error) {
		r, err := c.call(cctx, sid, "Runtime.evaluate", map[string]any{"expression": expr, "returnByValue": true, "awaitPromise": true})
		if err != nil {
			return "", err
		}
		var x struct {
			Result struct {
				Value any `json:"value"`
			} `json:"result"`
		}
		_ = json.Unmarshal(r, &x)
		if v, ok := x.Result.Value.(string); ok {
			return v, nil
		}
		return "", nil
	}
	settle := func() {
		for i := 0; i < 40; i++ {
			if st, _ := eval("document.readyState"); st == "complete" {
				break
			}
			time.Sleep(250 * time.Millisecond)
		}
		time.Sleep(700 * time.Millisecond)
	}
	if _, err := c.call(cctx, sid, "Page.navigate", map[string]any{"url": raw}); err != nil {
		return out, errors.New("không mở được trang")
	}
	time.Sleep(500 * time.Millisecond)
	settle()
	vb, _ := json.Marshal(vals)
	tb, _ := json.Marshal(typ)
	res, err := eval(fillJS + "(" + string(tb) + "," + string(vb) + ")")
	vb = nil
	if err != nil {
		return out, errors.New("không điền được form trên trang")
	}
	var fr struct {
		Done []string `json:"done"`
		Sub  bool     `json:"sub"`
	}
	_ = json.Unmarshal([]byte(res), &fr)
	if len(fr.Done) == 0 {
		return out, errors.New("không tìm thấy ô phù hợp trên trang")
	}
	for _, k := range fr.Done {
		out.Filled = append(out.Filled, vaultFieldLabel[k])
	}
	out.Submitted = fr.Sub
	time.Sleep(800 * time.Millisecond)
	settle()
	pg, _ := eval(`JSON.stringify({t:document.title,u:location.href,x:(document.body&&document.body.innerText||"").slice(0,4000)})`)
	var p struct{ T, U, X string }
	_ = json.Unmarshal([]byte(pg), &p)
	out.Title, out.URL = p.T, p.U
	out.Text = strings.Join(strings.Fields(p.X), " ")
	return out, nil
}
