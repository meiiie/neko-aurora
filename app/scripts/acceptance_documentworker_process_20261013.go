package documentworker

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if len(os.Args) == 2 && os.Args[1] == "--document-worker-v1" {
		os.Exit(independentWorkerHelper())
	}
	os.Exit(m.Run())
}

func independentResponse(text []byte) []byte {
	frame := make([]byte, 41+len(text))
	copy(frame, "NDR1")
	frame[4] = 1
	binary.BigEndian.PutUint32(frame[5:9], uint32(len(text)))
	sum := sha256.Sum256(text)
	copy(frame[9:41], sum[:])
	copy(frame[41:], text)
	return frame
}

func independentWorkerHelper() int {
	frame, err := io.ReadAll(io.LimitReader(os.Stdin, (1<<20)+10))
	if err != nil || len(frame) < 9 || string(frame[:4]) != "NDW1" || frame[4] != 1 || int(binary.BigEndian.Uint32(frame[5:9])) != len(frame)-9 {
		return 93
	}
	mode := string(frame[9:])
	if mode == "hold" {
		if os.WriteFile("worker.pid.pending", []byte(strconv.Itoa(os.Getpid())), 0600) != nil {
			return 94
		}
		if os.Rename("worker.pid.pending", "worker.pid") != nil {
			return 94
		}
		time.Sleep(30 * time.Second)
	}
	text := []byte("independent helper \u03b1 \u1ec7")
	if mode == "environment" {
		cwd, _ := os.Getwd()
		audit := map[string]bool{}
		for _, name := range []string{"NEKO_QA_SYNTHETIC_PROVIDER_KEY", "PATH", "APPDATA", "LOCALAPPDATA", "HOME", "HTTP_PROXY", "HTTPS_PROXY", "GOENV"} {
			_, audit[name] = os.LookupEnv(name)
		}
		for _, name := range []string{"TEMP", "TMP", "USERPROFILE"} {
			audit["owned_"+name] = os.Getenv(name) == cwd
		}
		audit["sole_private_argument"] = len(os.Args) == 2 && os.Args[1] == "--document-worker-v1"
		text, _ = json.Marshal(audit)
	}
	if mode == "stderr-at-limit" || mode == "stderr-over-limit" {
		size := 64 << 10
		if mode == "stderr-over-limit" {
			size++
		}
		if _, err := os.Stderr.Write(bytes.Repeat([]byte{'s'}, size)); err != nil {
			return 95
		}
	}
	if mode == "stdout-over-limit" {
		_, _ = os.Stdout.Write(bytes.Repeat([]byte{'o'}, (4<<20)+1))
		return 0
	}
	if mode == "trailing-frame" {
		_, _ = os.Stdout.Write(append(independentResponse(text), independentResponse(text)...))
		return 0
	}
	_, err = os.Stdout.Write(independentResponse(text))
	if err != nil {
		return 96
	}
	return 0
}

func independentClient(t *testing.T) *Client {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	client, err := newClient(executable, directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	return client
}

func independentMustReject(t *testing.T, client *Client, mode string) {
	t.Helper()
	result, err := client.Extract("docx", []byte(mode))
	if err != ErrExtraction || result.Text != "" || result.TextSHA256 != "" || result.TextBytes != 0 {
		t.Fatal("worker refusal exposed a partial result or nonfixed error")
	}
}

func TestIndependentWorkerClosedDecodeCombinations(t *testing.T) {
	valid := independentResponse([]byte("\u03b1 \u1ec7"))
	mutations := map[string]func([]byte) []byte{
		"valid-body-foreign-digest":         func(frame []byte) []byte { frame[9] ^= 1; return frame },
		"valid-digest-short-body":           func(frame []byte) []byte { return frame[:len(frame)-1] },
		"multiple-valid-successes":          func(frame []byte) []byte { return append(frame, frame...) },
		"reject-with-success-material":      func(frame []byte) []byte { frame[4] = 0; return frame },
		"foreign-status-success-material":   func(frame []byte) []byte { frame[4] = 2; return frame },
		"declared-over-text-cap":            func(frame []byte) []byte { binary.BigEndian.PutUint32(frame[5:9], (512<<10)+1); return frame },
		"invalid-UTF8-with-matching-digest": func(frame []byte) []byte { return independentResponse([]byte{0xc0, 0xaf}) },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			result, err := decodeResult(mutate(bytes.Clone(valid)))
			if err != ErrExtraction || result.Text != "" || result.TextBytes != 0 {
				t.Fatal("closed response combination accepted or returned partial content")
			}
		})
	}
	result, err := decodeResult(valid)
	sum := sha256.Sum256([]byte(result.Text))
	if err != nil || result.Text != "\u03b1 \u1ec7" || result.TextBytes != len([]byte(result.Text)) || result.TextSHA256 != hex.EncodeToString(sum[:]) {
		t.Fatal("independent Unicode witness did not round-trip")
	}
}

func TestIndependentWorkerProcessEnvironmentAndClosedFormat(t *testing.T) {
	t.Setenv("NEKO_QA_SYNTHETIC_PROVIDER_KEY", "published-synthetic-not-a-secret")
	client := independentClient(t)
	result, err := client.Extract("docx", []byte("environment"))
	if err != nil {
		t.Fatal(err)
	}
	var audit map[string]bool
	if json.Unmarshal([]byte(result.Text), &audit) != nil {
		t.Fatal("invalid authored environment audit")
	}
	for _, name := range []string{"NEKO_QA_SYNTHETIC_PROVIDER_KEY", "PATH", "APPDATA", "LOCALAPPDATA", "HOME", "HTTP_PROXY", "HTTPS_PROXY", "GOENV"} {
		if audit[name] {
			t.Fatalf("unexpected inherited named environment %s", name)
		}
	}
	for _, name := range []string{"owned_TEMP", "owned_TMP", "owned_USERPROFILE", "sole_private_argument"} {
		if !audit[name] {
			t.Fatalf("missing authored ownership condition %s", name)
		}
	}
	for _, format := range []string{"../docx", "DOCX", "docx --state=foreign", "https://fixture.invalid/docx", ""} {
		if _, err := client.Extract(format, []byte("environment")); err != ErrExtraction {
			t.Fatal("format became a path/flag/URL choice")
		}
	}
	command := exec.Command(client.executable, "--document-worker-v1")
	hideWindow(command)
	if command.SysProcAttr == nil || !command.SysProcAttr.HideWindow {
		t.Fatal("Windows worker launch is not hidden")
	}
}

func TestIndependentWorkerStreamingEdgesReleaseSlot(t *testing.T) {
	client := independentClient(t)
	result, err := client.Extract("docx", []byte("stderr-at-limit"))
	if err != nil || !strings.Contains(result.Text, "independent helper") {
		t.Fatal("exact stderr bound incorrectly rejected fixture")
	}
	for _, mode := range []string{"stderr-over-limit", "stdout-over-limit", "trailing-frame"} {
		t.Run(mode, func(t *testing.T) {
			independentMustReject(t, client, mode)
			if len(client.slots) != 0 {
				t.Fatal("refusal retained a worker slot")
			}
			if _, err := client.Extract("docx", []byte("good")); err != nil {
				t.Fatal("worker slot was not reusable after refused output")
			}
		})
	}
}

func independentStarted(t *testing.T, directory string, count int) []syscall.Handle {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		entries, _ := filepath.Glob(filepath.Join(directory, "call-*", "worker.pid"))
		if len(entries) == count {
			handles := []syscall.Handle{}
			for _, entry := range entries {
				raw, err := os.ReadFile(entry)
				if err != nil {
					t.Fatal(err)
				}
				pid, err := strconv.Atoi(string(raw))
				if err != nil {
					t.Fatal(err)
				}
				handle, err := syscall.OpenProcess(0x00100000|0x0400, false, uint32(pid))
				if err != nil {
					t.Fatal("cannot capture original owned direct-child handle")
				}
				handles = append(handles, handle)
			}
			return handles
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("authored direct workers did not signal startup within fixture headroom")
	return nil
}

func independentExited(t *testing.T, handles []syscall.Handle) {
	t.Helper()
	for _, handle := range handles {
		status, err := syscall.WaitForSingleObject(handle, 2000)
		syscall.CloseHandle(handle)
		if err != nil || status != 0 {
			t.Fatal("owned original process handle was not signaled after cancellation/reap")
		}
	}
}

func TestIndependentWorkerTwoSlotsCloseAndReap(t *testing.T) {
	client := independentClient(t)
	finished := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { _, err := client.Extract("docx", []byte("hold")); finished <- err }()
	}
	handles := independentStarted(t, client.directory, 2)
	start := time.Now()
	independentMustReject(t, client, "good")
	if time.Since(start) > 300*time.Millisecond {
		t.Fatal("busy failure queued behind occupied slots")
	}
	start = time.Now()
	client.Close()
	if time.Since(start) > 3*time.Second {
		t.Fatal("close exceeded bounded authored fixture headroom")
	}
	for i := 0; i < 2; i++ {
		select {
		case err := <-finished:
			if err != ErrExtraction {
				t.Fatal("closed worker did not return fixed refusal")
			}
		case <-time.After(time.Second):
			t.Fatal("closed worker goroutine not reaped")
		}
	}
	independentExited(t, handles)
	if len(client.slots) != 0 {
		t.Fatal("close retained worker slots")
	}
	if _, err := os.Stat(client.directory); !os.IsNotExist(err) {
		t.Fatal("closed owned worker directory was not removed")
	}
	independentMustReject(t, client, "good")
	client.Close()
}

func TestIndependentWorkerDeadlineReapAndSlotReuse(t *testing.T) {
	client := independentClient(t)
	client.timeout = 2 * time.Second
	finished := make(chan error, 1)
	start := time.Now()
	go func() { _, err := client.Extract("docx", []byte("hold")); finished <- err }()
	handles := independentStarted(t, client.directory, 1)
	select {
	case err := <-finished:
		if err != ErrExtraction {
			t.Fatal("deadline returned nonfixed refusal")
		}
	case <-time.After(4 * time.Second):
		t.Fatal("short authored deadline did not bound worker completion")
	}
	if time.Since(start) > 4*time.Second {
		t.Fatal("authored deadline exceeded bounded reap headroom")
	}
	independentExited(t, handles)
	client.timeout = Deadline
	if Deadline != 15*time.Second {
		t.Fatal("configured production deadline changed")
	}
	if len(client.slots) != 0 {
		t.Fatal("deadline retained worker slot")
	}
	if _, err := client.Extract("docx", []byte("good")); err != nil {
		t.Fatal("deadline did not leave slot reusable")
	}
}

func TestIndependentWorkerDigestPinRejectsChangedHelper(t *testing.T) {
	executable, _ := os.Executable()
	bytes, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	copyPath := filepath.Join(t.TempDir(), "owned-helper.exe")
	if os.WriteFile(copyPath, bytes, 0700) != nil {
		t.Fatal("cannot write owned helper copy")
	}
	client, err := newClient(copyPath, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if os.WriteFile(copyPath, append(bytes, 0), 0700) != nil {
		t.Fatal("cannot change unlaunched owned helper copy")
	}
	independentMustReject(t, client, "good")
	if len(client.slots) != 0 {
		t.Fatal("digest refusal retained a worker slot")
	}
	if _, err := newClient("owned-helper.exe", t.TempDir()); err != ErrExtraction {
		t.Fatal("relative helper path accepted")
	}
	if _, err := newClient(executable, "."); err != ErrExtraction {
		t.Fatal("relative private directory accepted")
	}
}
