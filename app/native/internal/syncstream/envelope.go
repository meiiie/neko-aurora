package syncstream

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

func NewSnapshot(instanceID, scopeID string, sequence uint64, objects []Object) (Envelope, error) {
	return Seal(Envelope{Version: Version, InstanceID: instanceID, ScopeID: scopeID, Mode: Snapshot, To: sequence, HighWater: sequence, Snapshot: objects})
}

func NewIncrement(instanceID, scopeID string, after, highWater uint64, frames []Frame) (Envelope, error) {
	to := after
	if len(frames) > 0 {
		to = frames[len(frames)-1].Sequence
	}
	return Seal(Envelope{Version: Version, InstanceID: instanceID, ScopeID: scopeID, Mode: Increment, After: after, To: to, HighWater: highWater, Frames: frames})
}

func Seal(e Envelope) (Envelope, error) {
	if err := checkCounts(e); err != nil {
		return Envelope{}, err
	}
	e = cloneEnvelope(e)
	sortObjects(e.Snapshot)
	for i := range e.Frames {
		sortObjects(e.Frames[i].Updates)
	}
	if err := e.validateShape(); err != nil {
		return Envelope{}, err
	}
	e.ScopeDigest, _ = BindingDigest(e.InstanceID, e.ScopeID)
	e.DataDigest = envelopeDigest(e)
	if err := e.Validate(); err != nil {
		return Envelope{}, err
	}
	return e, nil
}

func (e Envelope) Validate() error {
	if err := e.validateShape(); err != nil {
		return err
	}
	binding, _ := BindingDigest(e.InstanceID, e.ScopeID)
	if e.ScopeDigest != binding || !validDigest(e.DataDigest) || e.DataDigest != envelopeDigest(e) {
		return ErrIntegrity
	}
	b, err := json.Marshal(e)
	if err != nil {
		return ErrInvalid
	}
	if len(b) > MaxEnvelopeBytes {
		return ErrLimit
	}
	return nil
}

func (e Envelope) validateShape() error {
	if err := checkCounts(e); err != nil {
		return err
	}
	if e.Version != Version || !validID(e.InstanceID) || !validID(e.ScopeID) || e.After > e.To || e.To > e.HighWater || e.HighWater > MaxSafeSequence || e.Snapshot == nil || e.Frames == nil {
		return ErrInvalid
	}
	switch e.Mode {
	case Snapshot:
		if e.After != 0 || e.To != e.HighWater || len(e.Frames) != 0 {
			return ErrInvalid
		}
		return validateObjects(e.Snapshot)
	case Increment:
		if len(e.Snapshot) != 0 || e.To-e.After != uint64(len(e.Frames)) || len(e.Frames) == 0 && e.To != e.HighWater {
			return ErrGap
		}
		for i, frame := range e.Frames {
			if frame.Sequence != e.After+uint64(i)+1 || frame.Updates == nil {
				return ErrGap
			}
			if err := validateObjects(frame.Updates); err != nil {
				return err
			}
		}
		return nil
	default:
		return ErrInvalid
	}
}

func checkCounts(e Envelope) error {
	if len(e.Snapshot) > MaxObjects || len(e.Frames) > MaxFrames {
		return ErrLimit
	}
	updates := 0
	for _, frame := range e.Frames {
		if len(frame.Updates) > MaxObjects-updates {
			return ErrLimit
		}
		updates += len(frame.Updates)
	}
	return nil
}

func Encode(e Envelope) ([]byte, error) {
	if err := e.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(e)
}

func Decode(data []byte) (Envelope, error) {
	var e Envelope
	err := decodeStrict(data, &e, MaxEnvelopeBytes, []string{"version", "instanceId", "scopeId", "mode", "after", "to", "highWater", "scopeDigest", "dataDigest", "snapshot", "frames"})
	if err != nil {
		return Envelope{}, err
	}
	if err = e.Validate(); err != nil {
		return Envelope{}, err
	}
	return e, nil
}

func DecodeReader(r io.Reader) (Envelope, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxEnvelopeBytes+1))
	if err != nil {
		return Envelope{}, err
	}
	return Decode(data)
}

func BindingDigest(instanceID, scopeID string) (string, error) {
	if !validID(instanceID) || !validID(scopeID) {
		return "", ErrInvalid
	}
	b, _ := json.Marshal(struct {
		Version    int    `json:"version"`
		InstanceID string `json:"instanceId"`
		ScopeID    string `json:"scopeId"`
	}{Version, instanceID, scopeID})
	return digest(b), nil
}

func (c Cursor) Validate() error {
	if c.Version != Version || !validID(c.InstanceID) || !validID(c.ScopeID) || c.Sequence > MaxSafeSequence {
		return ErrInvalid
	}
	return nil
}

func (c Cursor) Check(instanceID, scopeID string, highWater uint64) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if !validID(instanceID) || !validID(scopeID) || highWater > MaxSafeSequence {
		return ErrInvalid
	}
	if c.InstanceID != instanceID {
		return ErrInstance
	}
	if c.ScopeID != scopeID {
		return ErrScope
	}
	if c.Sequence > highWater {
		return ErrAhead
	}
	return nil
}

func (c Cursor) Encode() (string, error) {
	if err := c.Validate(); err != nil {
		return "", err
	}
	b, _ := json.Marshal(c)
	token := base64.RawURLEncoding.EncodeToString(b)
	if len(token) > MaxCursorBytes {
		return "", ErrLimit
	}
	return token, nil
}

func ParseCursor(token string) (Cursor, error) {
	if len(token) == 0 || len(token) > MaxCursorBytes {
		return Cursor{}, ErrLimit
	}
	b, err := base64.RawURLEncoding.Strict().DecodeString(token)
	if err != nil {
		return Cursor{}, ErrInvalid
	}
	var c Cursor
	if err = decodeStrict(b, &c, MaxCursorBytes, []string{"version", "instanceId", "scopeId", "sequence"}); err != nil {
		return Cursor{}, err
	}
	canonical, err := c.Encode()
	if err != nil {
		return Cursor{}, err
	}
	if canonical != token {
		return Cursor{}, ErrInvalid
	}
	return c, nil
}

func validateObjects(objects []Object) error {
	prior := ""
	for i, object := range objects {
		key := objectKey(object)
		if i > 0 && key <= prior {
			return ErrInvalid
		}
		prior = key
		if err := validateObject(object); err != nil {
			return err
		}
	}
	return nil
}

func validateObject(o Object) error {
	if !validID(o.ID) || o.Revision == 0 || o.Revision > MaxSafeSequence {
		return ErrInvalid
	}
	if o.Kind != "bot" && o.Kind != "message" && o.Kind != "file" {
		return ErrInvalid
	}
	if o.Deleted {
		if o.Bot != nil || o.Message != nil || o.File != nil {
			return ErrInvalid
		}
		return nil
	}
	switch o.Kind {
	case "bot":
		if o.Bot == nil || o.Message != nil || o.File != nil {
			return ErrInvalid
		}
		b := o.Bot
		if !plainText(b.Name, 320, false) || b.Name == "" || strings.TrimSpace(b.Name) != b.Name || utf8.RuneCountInString(b.Name) > 80 || !plainText(b.Alias, 128, false) || !plainText(b.Description, 4096, true) || !plainText(b.Color, 32, false) {
			return ErrInvalid
		}
	case "message":
		if o.Message == nil || o.Bot != nil || o.File != nil {
			return ErrInvalid
		}
		m := o.Message
		if !validID(m.BotID) || !optionalID(m.ChannelID) || (m.Role != "user" && m.Role != "assistant") || !plainText(m.Text, 12<<10, true) || !validID(m.Status) {
			return ErrInvalid
		}
	case "file":
		if o.File == nil || o.Bot != nil || o.Message != nil {
			return ErrInvalid
		}
		f := o.File
		if !plainText(f.Name, 256, false) || f.Name == "" || strings.ContainsAny(f.Name, "/\\") || !plainText(f.MIME, 128, false) || f.MIME == "" || f.Size > MaxSafeSequence || !validDigest(f.SHA256) || !optionalID(f.BotID) || !optionalID(f.ChannelID) || !optionalID(f.ProjectID) || !validID(f.Status) || f.Version == 0 || f.Version > MaxSafeSequence || f.AccessRevision > MaxSafeSequence {
			return ErrInvalid
		}
	}
	b, err := json.Marshal(o)
	if err != nil {
		return ErrInvalid
	}
	if len(b) > MaxObjectBytes {
		return ErrLimit
	}
	return nil
}

func validID(s string) bool {
	if len(s) == 0 || len(s) > 128 {
		return false
	}
	for i, c := range s {
		letterOrDigit := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
		if !letterOrDigit && (i == 0 || c != '-' && c != '_' && c != '.' && c != ':') {
			return false
		}
	}
	return true
}

func optionalID(s string) bool { return s == "" || validID(s) }

func plainText(s string, max int, multiline bool) bool {
	if len(s) > max || !utf8.ValidString(s) {
		return false
	}
	for _, c := range s {
		if unicode.IsControl(c) && !(multiline && (c == '\n' || c == '\r' || c == '\t')) {
			return false
		}
	}
	return true
}

func validDigest(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func digest(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func envelopeDigest(e Envelope) string {
	e.DataDigest = ""
	b, _ := json.Marshal(e)
	return digest(b)
}

func objectDigest(o Object) string {
	b, _ := json.Marshal(o)
	return digest(b)
}

func frameDigest(f Frame) string {
	b, _ := json.Marshal(f)
	return digest(b)
}

func objectKey(o Object) string { return o.Kind + ":" + o.ID }

func sortObjects(objects []Object) {
	sort.Slice(objects, func(i, j int) bool { return objectKey(objects[i]) < objectKey(objects[j]) })
}

func cloneObjects(objects []Object) []Object {
	out := make([]Object, len(objects))
	for i, o := range objects {
		out[i] = o
		if o.Bot != nil {
			b := *o.Bot
			out[i].Bot = &b
		}
		if o.Message != nil {
			m := *o.Message
			out[i].Message = &m
		}
		if o.File != nil {
			f := *o.File
			out[i].File = &f
		}
	}
	return out
}

func cloneEnvelope(e Envelope) Envelope {
	e.Snapshot = cloneObjects(e.Snapshot)
	e.Frames = append([]Frame{}, e.Frames...)
	for i := range e.Frames {
		e.Frames[i].Updates = cloneObjects(e.Frames[i].Updates)
	}
	return e
}

func decodeStrict(data []byte, dst any, limit int, required []string) error {
	if len(data) > limit {
		return ErrLimit
	}
	if !utf8.Valid(data) {
		return ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	keys := map[string]bool{}
	if err := walkJSON(d, 0, keys); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return ErrInvalid
	}
	for _, key := range required {
		if !keys[key] {
			return ErrInvalid
		}
	}
	d = json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	var raw any
	if err := d.Decode(&raw); err != nil {
		return ErrInvalid
	}
	if err := checkSchema(raw, reflect.TypeOf(dst).Elem()); err != nil {
		return err
	}
	d = json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		return ErrInvalid
	}
	return nil
}

func checkSchema(value any, schema reflect.Type) error {
	if schema.Kind() == reflect.Pointer {
		if value == nil {
			return nil
		}
		return checkSchema(value, schema.Elem())
	}
	switch schema.Kind() {
	case reflect.Struct:
		object, ok := value.(map[string]any)
		if !ok {
			return ErrInvalid
		}
		allowed := map[string]reflect.StructField{}
		for i := 0; i < schema.NumField(); i++ {
			field := schema.Field(i)
			tag := strings.Split(field.Tag.Get("json"), ",")
			allowed[tag[0]] = field
			if len(tag) == 1 {
				if _, found := object[tag[0]]; !found {
					return ErrInvalid
				}
			}
		}
		for key, child := range object {
			field, found := allowed[key]
			if !found {
				return ErrInvalid
			}
			if err := checkSchema(child, field.Type); err != nil {
				return err
			}
		}
	case reflect.Slice:
		if value == nil {
			return nil
		}
		array, ok := value.([]any)
		if !ok {
			return ErrInvalid
		}
		for _, child := range array {
			if err := checkSchema(child, schema.Elem()); err != nil {
				return err
			}
		}
	case reflect.String:
		if _, ok := value.(string); !ok {
			return ErrInvalid
		}
	case reflect.Bool:
		if _, ok := value.(bool); !ok {
			return ErrInvalid
		}
	case reflect.Int, reflect.Uint64:
		if _, ok := value.(json.Number); !ok {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}

func walkJSON(d *json.Decoder, depth int, rootKeys map[string]bool) error {
	if depth > 8 {
		return ErrLimit
	}
	token, err := d.Token()
	if err != nil {
		return ErrInvalid
	}
	delim, compound := token.(json.Delim)
	if !compound {
		if depth == 0 {
			return ErrInvalid
		}
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			keyToken, err := d.Token()
			if err != nil {
				return ErrInvalid
			}
			key, ok := keyToken.(string)
			if !ok || seen[key] {
				return ErrInvalid
			}
			seen[key] = true
			if depth == 0 {
				rootKeys[key] = true
			}
			if err := walkJSON(d, depth+1, rootKeys); err != nil {
				return err
			}
		}
		end, err := d.Token()
		if err != nil || end != json.Delim('}') {
			return ErrInvalid
		}
	case '[':
		if depth == 0 {
			return ErrInvalid
		}
		for d.More() {
			if err := walkJSON(d, depth+1, rootKeys); err != nil {
				return err
			}
		}
		end, err := d.Token()
		if err != nil || end != json.Delim(']') {
			return ErrInvalid
		}
	default:
		return fmt.Errorf("%w: JSON delimiter", ErrInvalid)
	}
	return nil
}
