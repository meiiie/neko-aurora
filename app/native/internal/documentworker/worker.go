package documentworker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
	"unicode/utf8"

	"nekobot.local/native/internal/documents"
)

const (
	MaxRawBytes    = documents.MaxDOCXBytes
	MaxTextBytes   = documents.MaxTextBytes
	MaxStdoutBytes = 4 << 20
	MaxStderrBytes = 64 << 10
	MaxWorkers     = 2
	Deadline       = 15 * time.Second
)

var ErrExtraction = errors.New("document_extraction_unavailable")

func formatByte(format string) byte {
	switch format {
	case "docx":
		return 1
	case "xlsx":
		return 2
	case "pptx":
		return 3
	}
	return 0
}

func requestFrame(format string, raw []byte) []byte {
	out := make([]byte, 9+len(raw))
	copy(out, "NDW1")
	out[4] = formatByte(format)
	binary.BigEndian.PutUint32(out[5:9], uint32(len(raw)))
	copy(out[9:], raw)
	return out
}

func rejectFrame() []byte { out := make([]byte, 41); copy(out, "NDR1"); return out }

func Run(input io.Reader, output io.Writer) int {
	frame, err := io.ReadAll(io.LimitReader(input, MaxRawBytes+10))
	out := rejectFrame()
	code := 2
	if err == nil && len(frame) >= 9 && string(frame[:4]) == "NDW1" && frame[4] >= 1 && frame[4] <= 3 {
		size := binary.BigEndian.Uint32(frame[5:9])
		if size > 0 && size <= MaxRawBytes && len(frame) == 9+int(size) {
			var result documents.Result
			switch frame[4] {
			case 1:
				result, err = documents.ExtractDOCX(frame[9:])
			case 2:
				result, err = documents.ExtractXLSX(frame[9:])
			case 3:
				result, err = documents.ExtractPPTX(frame[9:])
			}
			code = 0
			if err == nil && result.TextBytes > 0 && result.TextBytes <= MaxTextBytes && len(result.Text) == result.TextBytes && utf8.ValidString(result.Text) {
				sum := sha256.Sum256([]byte(result.Text))
				if result.TextSHA256 == hex.EncodeToString(sum[:]) {
					out = make([]byte, 41+result.TextBytes)
					copy(out, "NDR1")
					out[4] = 1
					binary.BigEndian.PutUint32(out[5:9], uint32(result.TextBytes))
					copy(out[9:41], sum[:])
					copy(out[41:], result.Text)
				}
			}
		}
	}
	if _, err = output.Write(out); err != nil {
		return 2
	}
	return code
}

func decodeResult(frame []byte) (documents.Result, error) {
	if len(frame) < 41 || len(frame) > MaxStdoutBytes || string(frame[:4]) != "NDR1" || frame[4] != 1 {
		return documents.Result{}, ErrExtraction
	}
	size := binary.BigEndian.Uint32(frame[5:9])
	if size == 0 || size > MaxTextBytes || len(frame) != 41+int(size) || !utf8.Valid(frame[41:]) {
		return documents.Result{}, ErrExtraction
	}
	sum := sha256.Sum256(frame[41:])
	if !bytes.Equal(sum[:], frame[9:41]) {
		return documents.Result{}, ErrExtraction
	}
	return documents.Result{Text: string(frame[41:]), TextSHA256: hex.EncodeToString(sum[:]), TextBytes: int(size)}, nil
}

type Client struct {
	executable string
	digest     [32]byte
	directory  string
	slots      chan struct{}
	timeout    time.Duration
	mu         sync.Mutex
	closed     bool
	ctx        context.Context
	cancel     context.CancelFunc
	running    sync.WaitGroup
}

func executableDigest(path string) ([32]byte, error) {
	var zero [32]byte
	file, err := os.Open(path)
	if err != nil {
		return zero, ErrExtraction
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > 128<<20 {
		return zero, ErrExtraction
	}
	hash := sha256.New()
	size, err := io.Copy(hash, io.LimitReader(file, (128<<20)+1))
	if err != nil || size != info.Size() {
		return zero, ErrExtraction
	}
	copy(zero[:], hash.Sum(nil))
	return zero, nil
}

func NewSelf() (*Client, error) {
	executable, err := os.Executable()
	if err != nil || !filepath.IsAbs(executable) {
		return nil, ErrExtraction
	}
	directory, err := os.MkdirTemp("", "neko-document-worker-")
	if err != nil {
		return nil, ErrExtraction
	}
	client, err := newClient(executable, directory)
	if err != nil {
		os.Remove(directory)
	}
	return client, err
}

func newClient(executable, directory string) (*Client, error) {
	if !filepath.IsAbs(executable) || !filepath.IsAbs(directory) {
		return nil, ErrExtraction
	}
	digest, err := executableDigest(executable)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Client{executable: executable, digest: digest, directory: directory, slots: make(chan struct{}, MaxWorkers), timeout: Deadline, ctx: ctx, cancel: cancel}, nil
}

func (c *Client) Close() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	c.cancel()
	c.mu.Unlock()
	c.running.Wait()
	os.RemoveAll(c.directory)
}

type boundedOutput struct {
	limit    int
	count    int
	data     bytes.Buffer
	retain   bool
	cancel   context.CancelFunc
	exceeded bool
}

func (out *boundedOutput) Write(data []byte) (int, error) {
	if len(data) > out.limit-out.count {
		out.exceeded = true
		out.cancel()
		return 0, ErrExtraction
	}
	out.count += len(data)
	if out.retain {
		out.data.Write(data)
	}
	return len(data), nil
}

func (c *Client) Extract(format string, raw []byte) (documents.Result, error) {
	if c == nil || formatByte(format) == 0 || len(raw) == 0 || len(raw) > MaxRawBytes {
		return documents.Result{}, ErrExtraction
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return documents.Result{}, ErrExtraction
	}
	select {
	case c.slots <- struct{}{}:
	default:
		c.mu.Unlock()
		return documents.Result{}, ErrExtraction
	}
	c.running.Add(1)
	c.mu.Unlock()
	defer func() { <-c.slots; c.running.Done() }()
	ctx, cancel := context.WithTimeout(c.ctx, c.timeout)
	defer cancel()
	digest, err := executableDigest(c.executable)
	if err != nil || digest != c.digest || ctx.Err() != nil {
		return documents.Result{}, ErrExtraction
	}
	directory, err := os.MkdirTemp(c.directory, "call-")
	if err != nil {
		return documents.Result{}, ErrExtraction
	}
	defer os.RemoveAll(directory)
	command := exec.CommandContext(ctx, c.executable, "--document-worker-v1")
	hideWindow(command)
	command.Dir = directory
	command.WaitDelay = 250 * time.Millisecond
	for _, name := range []string{"SystemRoot", "WINDIR", "COMSPEC"} {
		if value, ok := os.LookupEnv(name); ok {
			command.Env = append(command.Env, name+"="+value)
		}
	}
	command.Env = append(command.Env, "TEMP="+directory, "TMP="+directory, "USERPROFILE="+directory)
	command.Stdin = bytes.NewReader(requestFrame(format, raw))
	stdout := &boundedOutput{limit: MaxStdoutBytes, retain: true, cancel: cancel}
	stderr := &boundedOutput{limit: MaxStderrBytes, cancel: cancel}
	command.Stdout = stdout
	command.Stderr = stderr
	if command.Run() != nil || ctx.Err() != nil || stdout.exceeded || stderr.exceeded {
		return documents.Result{}, ErrExtraction
	}
	return decodeResult(stdout.data.Bytes())
}
