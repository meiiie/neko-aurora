package documentworker

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if len(os.Args) == 2 && os.Args[1] == "--document-worker-v1" {
		input, _ := io.ReadAll(io.LimitReader(os.Stdin, MaxRawBytes+10))
		if len(input) >= 9 {
			switch string(input[9:]) {
			case "stall":
				time.Sleep(time.Minute)
				os.Exit(3)
			case "crash":
				os.Exit(3)
			case "stdout":
				os.Stdout.Write(bytes.Repeat([]byte{'x'}, MaxStdoutBytes+1))
				os.Exit(0)
			case "stderr":
				os.Stderr.Write(bytes.Repeat([]byte{'x'}, MaxStderrBytes+1))
				os.Exit(0)
			case "malformed":
				os.Stdout.Write([]byte("malformed"))
				os.Exit(0)
			case "bad-digest":
				output := successFrame([]byte("valid text"))
				output[9] ^= 1
				os.Stdout.Write(output)
				os.Exit(0)
			case "trailing":
				os.Stdout.Write(append(successFrame([]byte("valid text")), 0))
				os.Exit(0)
			case "text":
				if _, found := os.LookupEnv("NEKO_TEST_SECRET_CANARY"); found {
					os.Exit(3)
				}
				os.Stdout.Write(successFrame([]byte("valid text")))
				os.Exit(0)
			}
		}
		os.Exit(Run(bytes.NewReader(input), os.Stdout))
	}
	os.Exit(m.Run())
}

func successFrame(text []byte) []byte {
	out := make([]byte, 41+len(text))
	copy(out, "NDR1")
	out[4] = 1
	binary.BigEndian.PutUint32(out[5:9], uint32(len(text)))
	sum := sha256.Sum256(text)
	copy(out[9:41], sum[:])
	copy(out[41:], text)
	return out
}

func TestWorkerProtocolRejectsMalformedFrames(t *testing.T) {
	for name, input := range map[string][]byte{"magic": []byte("BAD1"), "format": append([]byte("NDW1\x09\x00\x00\x00\x01"), 'x'), "quota": []byte("NDW1\x01\x00\x10\x00\x01"), "short": []byte("NDW1\x01\x00\x00\x00\x02x"), "trailing": append(requestFrame("docx", []byte("x")), 0)} {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			Run(bytes.NewReader(input), &out)
			result, err := decodeResult(out.Bytes())
			if err == nil || result.Text != "" || !bytes.Equal(out.Bytes(), rejectFrame()) {
				t.Fatal("malformed request released output")
			}
		})
	}
}

func TestWorkerResultRejectsAlteredFrames(t *testing.T) {
	good := successFrame([]byte("tiếng Việt"))
	if out, err := decodeResult(good); err != nil || out.Text != "tiếng Việt" {
		t.Fatal("positive result", err)
	}
	for name, raw := range map[string][]byte{"digest": append([]byte(nil), good...), "extra": append(append([]byte(nil), good...), 0), "utf8": successFrame([]byte{0xff}), "status": append([]byte(nil), good...), "rejecttext": append(rejectFrame(), 0), "size": successFrame(bytes.Repeat([]byte{'a'}, MaxTextBytes+1))} {
		if name == "digest" {
			raw[9] ^= 1
		}
		if name == "status" {
			raw[4] = 2
		}
		t.Run(name, func(t *testing.T) {
			if out, err := decodeResult(raw); err == nil || out.Text != "" {
				t.Fatal("altered response released text")
			}
		})
	}
}

func testClient(t *testing.T) *Client {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	c, err := newClient(exe, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func TestWorkerClientProcessFailuresAndReuse(t *testing.T) {
	t.Setenv("NEKO_TEST_SECRET_CANARY", "synthetic-value")
	c := testClient(t)
	for _, mode := range []string{"stall", "crash", "stdout", "stderr", "malformed", "bad-digest", "trailing"} {
		t.Run(mode, func(t *testing.T) {
			c.timeout = 150 * time.Millisecond
			start := time.Now()
			out, err := c.Extract("docx", []byte(mode))
			if err == nil || out.Text != "" || time.Since(start) > 2*time.Second {
				t.Fatal("failure did not remain bounded")
			}
			// Reuse must succeed within the production deadline; the injected
			// 150 ms failure deadline also includes Windows process startup.
			c.timeout = Deadline
			out, err = c.Extract("docx", []byte("text"))
			if err != nil || out.Text != "valid text" {
				t.Fatal("worker slot not reusable", err)
			}
		})
	}
}

func TestWorkerClientCapacityAndClose(t *testing.T) {
	c := testClient(t)
	c.timeout = time.Second
	done := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { _, err := c.Extract("docx", []byte("stall")); done <- err }()
	}
	deadline := time.Now().Add(time.Second)
	for len(c.slots) != 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(c.slots) != 2 {
		t.Fatal("workers not active")
	}
	start := time.Now()
	if _, err := c.Extract("docx", []byte("text")); err == nil || time.Since(start) > 100*time.Millisecond {
		t.Fatal("capacity queued")
	}
	c.Close()
	for i := 0; i < 2; i++ {
		select {
		case err := <-done:
			if err == nil {
				t.Fatal("close released text")
			}
		case <-time.After(2 * time.Second):
			t.Fatal("worker not reaped")
		}
	}
	if _, err := c.Extract("docx", []byte("text")); err == nil {
		t.Fatal("closed client launched")
	}
}

func TestWorkerInputPolicy(t *testing.T) {
	c := testClient(t)
	for _, tc := range []struct {
		format string
		raw    []byte
	}{{"pdf", []byte("x")}, {"docx", nil}, {"docx", bytes.Repeat([]byte{'x'}, MaxRawBytes+1)}} {
		if _, err := c.Extract(tc.format, tc.raw); err == nil {
			t.Fatal("invalid input accepted")
		}
	}
	if Deadline != 15*time.Second || MaxWorkers != 2 || !strings.HasPrefix(string(requestFrame("docx", []byte("x"))), "NDW1") {
		t.Fatal("policy changed")
	}
}

func TestWorkerCleanupOnlyFreshPrivateDirectory(t *testing.T) {
	parent := t.TempDir()
	private := filepath.Join(parent, "private")
	sibling := filepath.Join(parent, "preserved")
	if err := os.Mkdir(private, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sibling, []byte("preserved bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	client, err := newClient(executable, private)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err = client.Extract("docx", []byte("text")); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(private)
	if err != nil || len(entries) != 0 {
		t.Fatal("call directory not cleaned")
	}
	client.Close()
	if _, err = os.Stat(private); !os.IsNotExist(err) {
		t.Fatal("private directory not cleaned")
	}
	body, err := os.ReadFile(sibling)
	if err != nil || string(body) != "preserved bytes" {
		t.Fatal("cleanup touched sibling")
	}
}
