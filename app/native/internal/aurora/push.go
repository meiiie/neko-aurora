package aurora

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

var b64 = base64.RawURLEncoding

// Pusher sends RFC 8291 encrypted Web Push messages signed with VAPID
// (RFC 8292). Keys are generated once and kept private in the state dir.
type Pusher struct {
	key  *ecdsa.PrivateKey
	Pub  string
	http *http.Client
}

func LoadPusher(dir string) (*Pusher, error) {
	path := filepath.Join(dir, "duet-vapid.pem")
	var k *ecdsa.PrivateKey
	if b, err := os.ReadFile(path); err == nil {
		blk, _ := pem.Decode(b)
		if blk == nil {
			return nil, errors.New("vapid_corrupt")
		}
		if k, err = x509.ParseECPrivateKey(blk.Bytes); err != nil {
			return nil, err
		}
	} else {
		if k, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader); err != nil {
			return nil, err
		}
		der, _ := x509.MarshalECPrivateKey(k)
		if err = os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}), 0o600); err != nil {
			return nil, err
		}
	}
	pub, _ := k.PublicKey.ECDH()
	return &Pusher{key: k, Pub: b64.EncodeToString(pub.Bytes()), http: &http.Client{Timeout: 15 * time.Second}}, nil
}

func (p *Pusher) vapid(endpoint string) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	h := b64.EncodeToString([]byte(`{"typ":"JWT","alg":"ES256"}`))
	c, _ := json.Marshal(map[string]any{"aud": u.Scheme + "://" + u.Host, "exp": time.Now().Add(12 * time.Hour).Unix(), "sub": "mailto:admin@holilihu.online"})
	in := h + "." + b64.EncodeToString(c)
	sum := sha256.Sum256([]byte(in))
	r, s, err := ecdsa.Sign(rand.Reader, p.key, sum[:])
	if err != nil {
		return "", err
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return fmt.Sprintf("vapid t=%s.%s, k=%s", in, b64.EncodeToString(sig), p.Pub), nil
}

func Encrypt(sub PushSub, plaintext []byte) ([]byte, error) {
	uaPubB, err := b64.DecodeString(sub.Keys.P256dh)
	if err != nil {
		return nil, err
	}
	auth, err := b64.DecodeString(sub.Keys.Auth)
	if err != nil {
		return nil, err
	}
	uaPub, err := ecdh.P256().NewPublicKey(uaPubB)
	if err != nil {
		return nil, err
	}
	as, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	shared, err := as.ECDH(uaPub)
	if err != nil {
		return nil, err
	}
	asPub := as.PublicKey().Bytes()
	prkKey, _ := hkdf.Extract(sha256.New, shared, auth)
	ikm, _ := hkdf.Expand(sha256.New, prkKey, "WebPush: info\x00"+string(uaPubB)+string(asPub), 32)
	salt := make([]byte, 16)
	_, _ = rand.Read(salt)
	prk, _ := hkdf.Extract(sha256.New, ikm, salt)
	cek, _ := hkdf.Expand(sha256.New, prk, "Content-Encoding: aes128gcm\x00", 16)
	nonce, _ := hkdf.Expand(sha256.New, prk, "Content-Encoding: nonce\x00", 12)
	blk, _ := aes.NewCipher(cek)
	gcm, _ := cipher.NewGCM(blk)
	ct := gcm.Seal(nil, nonce, append(append([]byte{}, plaintext...), 2), nil)
	var buf bytes.Buffer
	buf.Write(salt)
	_ = binary.Write(&buf, binary.BigEndian, uint32(4096))
	buf.WriteByte(byte(len(asPub)))
	buf.Write(asPub)
	buf.Write(ct)
	return buf.Bytes(), nil
}

// Send returns gone=true when the subscription is expired (404/410).
func (p *Pusher) Send(sub PushSub, payload []byte) (gone bool, err error) {
	body, err := Encrypt(sub, payload)
	if err != nil {
		return false, err
	}
	auth, err := p.vapid(sub.Endpoint)
	if err != nil {
		return false, err
	}
	req, _ := http.NewRequest("POST", sub.Endpoint, bytes.NewReader(body))
	req.Header.Set("Content-Encoding", "aes128gcm")
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("TTL", "86400")
	req.Header.Set("Urgency", "normal")
	req.Header.Set("Authorization", auth)
	res, err := p.http.Do(req)
	if err != nil {
		return false, err
	}
	res.Body.Close()
	if res.StatusCode == 404 || res.StatusCode == 410 {
		return true, nil
	}
	if res.StatusCode >= 300 {
		return false, fmt.Errorf("push_http_%d", res.StatusCode)
	}
	return false, nil
}
