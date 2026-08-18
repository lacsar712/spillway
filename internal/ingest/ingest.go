package ingest

import (
	"sync"
)

type Key struct {
	ID     string `json:"id"`
	Secret string `json:"secret"`
}

type Keys struct {
	mu   sync.Mutex
	byID map[string]Key
}

func New(defaultID, secret string) *Keys {
	k := &Keys{byID: map[string]Key{}}
	if defaultID == "" {
		defaultID = "ops"
	}
	k.byID[defaultID] = Key{ID: defaultID, Secret: secret}
	return k
}

func (k *Keys) Secrets(id string) []string {
	k.mu.Lock()
	defer k.mu.Unlock()
	if id == "" {
		id = "ingest"
	}
	item, ok := k.byID[id]
	if !ok {
		return nil
	}
	return []string{item.Secret}
}

func (k *Keys) Put(id, secret string) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.byID[id] = Key{ID: id, Secret: secret}
}

func (k *Keys) Snapshot() []Key {
	k.mu.Lock()
	defer k.mu.Unlock()
	out := make([]Key, 0, len(k.byID))
	for _, v := range k.byID {
		out = append(out, v)
	}
	return out
}

func (k *Keys) Restore(items []Key) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.byID = make(map[string]Key, len(items))
	for _, item := range items {
		k.byID[item.ID] = item
	}
}
