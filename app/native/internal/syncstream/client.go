package syncstream

import (
	"encoding/json"
	"sync"
)

type Outcome string

const (
	Applied Outcome = "applied"
	Ignored Outcome = "ignored"
)

type Client struct {
	mu            sync.Mutex
	instanceID    string
	scopeID       string
	sequence      uint64
	highWater     uint64
	initialized   bool
	needsSnapshot bool
	objects       map[string]Object
	floors        map[string]revisionFloor
	frames        map[uint64]string
	frameOrder    []uint64
	anchors       map[uint64]string
	anchorOrder   []uint64
}

type revisionFloor struct {
	revision uint64
	digest   string
	present  bool
}

func NewClient(instanceID, scopeID string) (*Client, error) {
	if !validID(instanceID) || !validID(scopeID) {
		return nil, ErrInvalid
	}
	c := &Client{instanceID: instanceID, scopeID: scopeID}
	c.clear()
	return c, nil
}

func (c *Client) Apply(e Envelope) (Outcome, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	fail := func(err error) (Outcome, error) {
		c.needsSnapshot = true
		return "", err
	}
	if err := e.Validate(); err != nil {
		return fail(err)
	}
	if e.InstanceID != c.instanceID {
		return fail(ErrInstance)
	}
	if e.ScopeID != c.scopeID {
		return fail(ErrScope)
	}
	e = cloneEnvelope(e)
	if e.Mode == Snapshot {
		outcome, err := c.applySnapshot(e)
		if err != nil {
			return fail(err)
		}
		return outcome, nil
	}
	if !c.initialized {
		return fail(ErrResync)
	}
	if e.After > c.sequence {
		return fail(ErrGap)
	}
	for _, frame := range e.Frames {
		if frame.Sequence > c.sequence {
			break
		}
		known, ok := c.frames[frame.Sequence]
		if !ok {
			return fail(ErrResync)
		}
		if known != frameDigest(frame) {
			return fail(ErrConflict)
		}
	}
	if e.To <= c.sequence {
		if len(e.Frames) == 0 {
			if _, known := c.anchors[e.To]; !known {
				return fail(ErrResync)
			}
		}
		c.observeHighWater(e.HighWater)
		return Ignored, nil
	}
	if c.needsSnapshot {
		return fail(ErrResync)
	}
	state := cloneState(c.objects)
	floors := cloneFloors(c.floors)
	for _, frame := range e.Frames {
		if frame.Sequence <= c.sequence {
			continue
		}
		if err := updateState(state, floors, frame.Updates); err != nil {
			return fail(err)
		}
	}
	if err := checkStateBudget(state, floors); err != nil {
		return fail(err)
	}
	for _, frame := range e.Frames {
		if frame.Sequence > c.sequence {
			c.rememberFrame(frame.Sequence, frameDigest(frame))
		}
	}
	c.objects = state
	c.floors = floors
	c.sequence = e.To
	c.observeHighWater(e.HighWater)
	c.rememberAnchor(e.To, stateDigest(state))
	return Applied, nil
}

func (c *Client) applySnapshot(e Envelope) (Outcome, error) {
	state := make(map[string]Object, len(e.Snapshot))
	for _, o := range e.Snapshot {
		state[objectKey(o)] = o
	}
	fingerprint := stateDigest(state)
	if c.initialized && e.To <= c.sequence {
		known, ok := c.anchors[e.To]
		if !ok {
			return "", ErrResync
		}
		if known != fingerprint {
			return "", ErrConflict
		}
		c.observeHighWater(e.HighWater)
		if e.To == c.sequence {
			c.needsSnapshot = false
		}
		return Ignored, nil
	}
	floors := cloneFloors(c.floors)
	if err := updateState(make(map[string]Object), floors, e.Snapshot); err != nil {
		return "", err
	}
	for key, floor := range floors {
		if _, present := state[key]; !present {
			floor.present = false
			floors[key] = floor
		}
	}
	if err := checkStateBudget(state, floors); err != nil {
		return "", err
	}
	c.objects = state
	c.floors = floors
	c.sequence = e.To
	c.observeHighWater(e.HighWater)
	c.initialized = true
	c.needsSnapshot = false
	c.rememberAnchor(e.To, fingerprint)
	return Applied, nil
}

func (c *Client) Cursor() Cursor {
	c.mu.Lock()
	defer c.mu.Unlock()
	return Cursor{Version: Version, InstanceID: c.instanceID, ScopeID: c.scopeID, Sequence: c.sequence}
}

func (c *Client) Objects() []Object {
	c.mu.Lock()
	defer c.mu.Unlock()
	return stateObjects(c.objects)
}

func (c *Client) HighWater() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.highWater
}

func (c *Client) NeedsSnapshot() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.needsSnapshot
}

func (c *Client) ResetInstance(instanceID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !validID(instanceID) || instanceID == c.instanceID {
		return ErrInvalid
	}
	c.instanceID = instanceID
	c.clear()
	return nil
}

func (c *Client) clear() {
	c.sequence = 0
	c.highWater = 0
	c.initialized = false
	c.needsSnapshot = true
	c.objects = map[string]Object{}
	c.floors = map[string]revisionFloor{}
	c.frames = map[uint64]string{}
	c.frameOrder = nil
	c.anchors = map[uint64]string{}
	c.anchorOrder = nil
}

func (c *Client) observeHighWater(highWater uint64) {
	if highWater > c.highWater {
		c.highWater = highWater
	}
}

func (c *Client) rememberFrame(sequence uint64, fingerprint string) {
	c.frames[sequence] = fingerprint
	c.frameOrder = append(c.frameOrder, sequence)
	if len(c.frameOrder) > ReplayWindow {
		delete(c.frames, c.frameOrder[0])
		c.frameOrder = c.frameOrder[1:]
	}
}

func (c *Client) rememberAnchor(sequence uint64, fingerprint string) {
	if _, exists := c.anchors[sequence]; !exists {
		c.anchorOrder = append(c.anchorOrder, sequence)
	}
	c.anchors[sequence] = fingerprint
	if len(c.anchorOrder) > ReplayWindow {
		delete(c.anchors, c.anchorOrder[0])
		c.anchorOrder = c.anchorOrder[1:]
	}
}

func cloneState(state map[string]Object) map[string]Object {
	out := make(map[string]Object, len(state))
	for _, o := range stateObjects(state) {
		out[objectKey(o)] = o
	}
	return out
}

func cloneFloors(floors map[string]revisionFloor) map[string]revisionFloor {
	out := make(map[string]revisionFloor, len(floors))
	for key, floor := range floors {
		out[key] = floor
	}
	return out
}

func updateState(state map[string]Object, floors map[string]revisionFloor, updates []Object) error {
	for _, o := range updates {
		key := objectKey(o)
		fingerprint := objectDigest(o)
		if floor, exists := floors[key]; exists {
			if o.Revision < floor.revision || o.Revision == floor.revision && (fingerprint != floor.digest || !floor.present && !o.Deleted) {
				return ErrConflict
			}
		}
		state[key] = o
		floors[key] = revisionFloor{revision: o.Revision, digest: fingerprint, present: !o.Deleted}
		if len(state) > MaxObjects || len(floors) > MaxObjects {
			return ErrLimit
		}
	}
	return nil
}

func stateObjects(state map[string]Object) []Object {
	objects := make([]Object, 0, len(state))
	for _, o := range state {
		objects = append(objects, o)
	}
	objects = cloneObjects(objects)
	sortObjects(objects)
	return objects
}

func stateDigest(state map[string]Object) string {
	b, _ := json.Marshal(stateObjects(state))
	return digest(b)
}

func checkStateBudget(state map[string]Object, floors map[string]revisionFloor) error {
	if len(state) > MaxObjects || len(floors) > MaxObjects {
		return ErrLimit
	}
	b, _ := json.Marshal(stateObjects(state))
	if len(b) > MaxStateBytes {
		return ErrLimit
	}
	return nil
}
