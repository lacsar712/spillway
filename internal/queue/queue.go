package queue

import (
	"container/heap"
	"sync"
	"time"

	"github.com/lacsar712/spillway/internal/clock"
	"github.com/lacsar712/spillway/internal/job"
)

type item struct {
	j     job.Job
	index int
}

type dueHeap []*item

func (h dueHeap) Len() int { return len(h) }

func (h dueHeap) Less(i, j int) bool {
	if h[i].j.NotBefore.Equal(h[j].j.NotBefore) {
		return h[i].j.CreatedAt.Before(h[j].j.CreatedAt)
	}
	return h[i].j.NotBefore.Before(h[j].j.NotBefore)
}

func (h dueHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *dueHeap) Push(x any) {
	it := x.(*item)
	it.index = len(*h)
	*h = append(*h, it)
}

func (h *dueHeap) Pop() any {
	old := *h
	n := len(old)
	it := old[n-1]
	old[n-1] = nil
	it.index = -1
	*h = old[:n-1]
	return it
}

type DestQueue struct {
	ID        string
	Ordered   bool
	inFlight  int
	maxFlight int
	ready     dueHeap
}

func newDestQueue(id string, ordered bool, maxFlight int) *DestQueue {
	if maxFlight < 1 {
		maxFlight = 1
	}
	if ordered {
		maxFlight = 1
	}
	dq := &DestQueue{ID: id, Ordered: ordered, maxFlight: maxFlight}
	heap.Init(&dq.ready)
	return dq
}

type Broker struct {
	mu    sync.Mutex
	clk   clock.Clock
	dests map[string]*DestQueue
}

func NewBroker(clk clock.Clock) *Broker {
	return &Broker{clk: clk, dests: make(map[string]*DestQueue)}
}

func (b *Broker) Ensure(id string, ordered bool, maxFlight int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.dests[id]; ok {
		return
	}
	b.dests[id] = newDestQueue(id, ordered, maxFlight)
}

func (b *Broker) Enqueue(j job.Job, ordered bool, maxFlight int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	dq, ok := b.dests[j.PLCID]
	if !ok {
		dq = newDestQueue(j.PLCID, ordered, maxFlight)
		b.dests[j.PLCID] = dq
	}
	heap.Push(&dq.ready, &item{j: j.Clone()})
}

func (b *Broker) Lease() (job.Job, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.clk.Now()
	var best *DestQueue
	var bestDue time.Time
	for _, dq := range b.dests {
		if dq.inFlight >= dq.maxFlight {
			continue
		}
		if dq.ready.Len() == 0 {
			continue
		}
		head := dq.ready[0].j
		if head.NotBefore.After(now) {
			continue
		}
		if best == nil || head.NotBefore.Before(bestDue) {
			best = dq
			bestDue = head.NotBefore
		}
	}
	if best == nil {
		return job.Job{}, false
	}
	it := heap.Pop(&best.ready).(*item)
	best.inFlight++
	return it.j, true
}

func (b *Broker) Release(destID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if dq, ok := b.dests[destID]; ok && dq.inFlight > 0 {
		dq.inFlight--
	}
}

func (b *Broker) Depth() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := 0
	for _, dq := range b.dests {
		n += dq.ready.Len() + dq.inFlight
	}
	return n
}

func (b *Broker) DepthByDest() map[string]int {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make(map[string]int, len(b.dests))
	for id, dq := range b.dests {
		out[id] = dq.ready.Len() + dq.inFlight
	}
	return out
}

func (b *Broker) Snapshot() []job.Job {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []job.Job
	for _, dq := range b.dests {
		for _, it := range dq.ready {
			out = append(out, it.j.Clone())
		}
	}
	return out
}

func (b *Broker) Restore(jobs []job.Job, meta map[string]DestMeta) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.dests = make(map[string]*DestQueue)
	for id, m := range meta {
		b.dests[id] = newDestQueue(id, m.Ordered, m.MaxInFlight)
	}
	for _, j := range jobs {
		dq, ok := b.dests[j.PLCID]
		if !ok {
			dq = newDestQueue(j.PLCID, false, 2)
			b.dests[j.PLCID] = dq
		}
		heap.Push(&dq.ready, &item{j: j.Clone()})
	}
}

type DestMeta struct {
	Ordered     bool
	MaxInFlight int
}
