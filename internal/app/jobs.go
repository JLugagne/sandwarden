package app

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

// JobEvent is one message on a job's topic.
type JobEvent struct {
	Kind  string `json:"kind"`
	Chunk string `json:"chunk,omitempty"`
	Error string `json:"error,omitempty"`
}

// Job statuses.
const (
	jobRunning = "running"
	jobDone    = "done"
	jobError   = "error"
)

// maxJobOutput caps the replayed output kept per job.
const maxJobOutput = 256 << 10

// jobTTL is how long a finished job stays queryable.
const jobTTL = 10 * time.Minute

// JobView is the queryable state of a job.
type JobView struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Output string `json:"output"`
	Error  string `json:"error,omitempty"`
}

// JobBroker runs background commands whose output streams over the hub on
// TopicJob(id), and buffers recent output so reconnecting clients can catch up
// through the job REST endpoint.
type JobBroker struct {
	hub  *Hub
	mu   sync.Mutex
	jobs map[string]*job
}

type job struct {
	id      string
	mu      sync.Mutex
	output  []byte
	status  string
	errMsg  string
	updated time.Time
	cancel  context.CancelFunc
}

// NewJobBroker builds a broker publishing on hub.
func NewJobBroker(hub *Hub) *JobBroker {
	return &JobBroker{hub: hub, jobs: make(map[string]*job)}
}

// Start runs fn in the background and returns the job id. An empty id is
// generated. Output written to the provided writer is streamed on the job's
// topic and retained for later queries.
func (b *JobBroker) Start(parent context.Context, id string, fn func(context.Context, io.Writer) error) string {
	if strings.TrimSpace(id) == "" {
		id = fmt.Sprintf("job-%d", time.Now().UnixNano())
	}
	ctx, cancel := context.WithCancel(parent)
	j := &job{id: id, status: jobRunning, updated: time.Now(), cancel: cancel}
	b.mu.Lock()
	b.jobs[id] = j
	b.mu.Unlock()
	b.sweep()
	go func() {
		err := fn(ctx, &jobWriter{broker: b, job: j})
		ev := JobEvent{Kind: "done"}
		if err != nil {
			ev.Error = err.Error()
		}
		j.finish(err)
		b.hub.Publish(EventFor(TopicJob(id), ev))
	}()
	return id
}

// Get returns a job's buffered state.
func (b *JobBroker) Get(id string) (JobView, bool) {
	b.mu.Lock()
	j, ok := b.jobs[id]
	b.mu.Unlock()
	if !ok {
		return JobView{}, false
	}
	return j.view(), true
}

// Cancel stops a running job.
func (b *JobBroker) Cancel(id string) {
	b.mu.Lock()
	j, ok := b.jobs[id]
	b.mu.Unlock()
	if ok {
		j.cancel()
	}
}

func (b *JobBroker) sweep() {
	cutoff := time.Now().Add(-jobTTL)
	b.mu.Lock()
	defer b.mu.Unlock()
	for id, j := range b.jobs {
		if j.finishedBefore(cutoff) {
			delete(b.jobs, id)
		}
	}
}

type jobWriter struct {
	broker *JobBroker
	job    *job
}

func (w *jobWriter) Write(p []byte) (int, error) {
	w.job.append(p)
	w.broker.hub.Publish(EventFor(TopicJob(w.job.id), JobEvent{Kind: "output", Chunk: string(p)}))
	return len(p), nil
}

func (j *job) append(p []byte) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.output = append(j.output, p...)
	if len(j.output) > maxJobOutput {
		j.output = j.output[len(j.output)-maxJobOutput:]
	}
	j.updated = time.Now()
}

func (j *job) finish(err error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if err != nil {
		j.status = jobError
		j.errMsg = err.Error()
	} else {
		j.status = jobDone
	}
	j.updated = time.Now()
}

func (j *job) finishedBefore(cutoff time.Time) bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.status != jobRunning && j.updated.Before(cutoff)
}

func (j *job) view() JobView {
	j.mu.Lock()
	defer j.mu.Unlock()
	return JobView{ID: j.id, Status: j.status, Output: string(j.output), Error: j.errMsg}
}
