package cloudbrowser

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const UsedMarkerFile = "cloud-browser-used"
const PurgeMarkerFile = "cloud-browser-purge-pending"

func readMarker(dir, name string) (string, bool, error) {
	if dir == "" {
		return "", false, ErrCleanupPending
	}
	p := filepath.Join(dir, name)
	info, err := os.Lstat(p)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() != 64 {
		return "", false, ErrCleanupPending
	}
	b, err := os.ReadFile(p)
	decoded, e := hex.DecodeString(string(b))
	if err != nil || e != nil || len(decoded) != 32 || hex.EncodeToString(decoded) != string(b) {
		return "", false, ErrCleanupPending
	}
	return string(b), true, nil
}

func NamespaceUsed(dir string) (bool, error) {
	_, exists, err := readMarker(dir, UsedMarkerFile)
	return exists, err
}

// MarkPurgePending retains an erasure request even when worker configuration
// is temporarily absent. It uses only the previously committed local scope.
func MarkPurgePending(dir string) error {
	namespace, used, err := readMarker(dir, UsedMarkerFile)
	if err != nil {
		return err
	}
	if !used {
		return nil
	}
	return writeMarker(dir, PurgeMarkerFile, namespace)
}

func writeMarker(dir, name, namespace string) error {
	if previous, exists, err := readMarker(dir, name); err != nil {
		return err
	} else if exists {
		if previous != namespace {
			return ErrCleanupPending
		}
		return nil
	}
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if os.IsExist(err) {
		previous, exists, e := readMarker(dir, name)
		if e == nil && exists && previous == namespace {
			return nil
		}
		return ErrCleanupPending
	}
	if err != nil {
		return ErrCleanupPending
	}
	_, err = f.WriteString(namespace)
	if e := f.Sync(); err == nil {
		err = e
	}
	if e := f.Close(); err == nil {
		err = e
	}
	if err != nil {
		return ErrCleanupPending
	}
	if runtime.GOOS != "windows" {
		d, e := os.Open(dir)
		if e != nil {
			return ErrCleanupPending
		}
		e = d.Sync()
		d.Close()
		if e != nil {
			return ErrCleanupPending
		}
	}
	return nil
}

// PurgeNamespace is only for the trusted account-deletion boundary. Unused
// workspaces need no remote call. Every later ordinary command is fenced.
func (c *Client) PurgeNamespace(ctx context.Context) error {
	if c == nil {
		return ErrNotConfigured
	}
	c.lifecycle.Lock()
	defer c.lifecycle.Unlock()
	c.deleting = true
	if c.purgeDone {
		return nil
	}
	marked, used, err := readMarker(c.stateDir, UsedMarkerFile)
	if err != nil {
		return err
	}
	if !used {
		c.purgeDone = true
		return nil
	}
	if marked != c.namespace {
		return ErrCleanupPending
	}
	if err = writeMarker(c.stateDir, PurgeMarkerFile, c.namespace); err != nil {
		return err
	}
	sum := sha256.Sum256([]byte(c.namespace))
	nonce := "purge-" + hex.EncodeToString(sum[:24])
	request := struct {
		Namespace string `json:"namespace"`
		Nonce     string `json:"nonce"`
	}{c.namespace, nonce}
	body, _ := json.Marshal(request)
	timestamp := strconv.FormatInt(c.now().Unix(), 10)
	digest := sha256.Sum256(body)
	m := hmac.New(sha256.New, c.key)
	m.Write([]byte(timestamp + "\n" + hex.EncodeToString(digest[:])))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSuffix(c.endpoint, "/command")+"/purge", bytes.NewReader(body))
	if err != nil {
		return ErrCleanupPending
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Aurora-Timestamp", timestamp)
	req.Header.Set("X-Aurora-Signature", hex.EncodeToString(m.Sum(nil)))
	res, err := c.http.Do(req)
	if err != nil {
		return ErrCleanupPending
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return ErrCleanupPending
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 1025))
	if err != nil || len(b) > 1024 {
		return ErrCleanupPending
	}
	var receipt struct {
		Namespace string `json:"namespace"`
		Nonce     string `json:"nonce"`
		Status    string `json:"status"`
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(&receipt) != nil || d.Decode(new(any)) != io.EOF || receipt.Namespace != c.namespace || receipt.Nonce != nonce || receipt.Status != "purged" {
		return ErrCleanupPending
	}
	c.purgeDone = true
	return nil
}
