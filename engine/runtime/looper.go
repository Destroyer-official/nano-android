package runtime

import (
	"container/heap"
	"errors"
	"sync"
	"sync/atomic"
)

// Message mocks android.os.Message
type Message struct {
	What     int
	Arg1     int
	Arg2     int
	Obj      any
	When     int64
	Target   *Handler
	Callback func()
	sequence uint64
	index    int // used by container/heap
}

// messagePriorityQueue implements heap.Interface for Messages ordered by When then sequence
type messagePriorityQueue []*Message

func (pq messagePriorityQueue) Len() int { return len(pq) }

func (pq messagePriorityQueue) Less(i, j int) bool {
	if pq[i].When == pq[j].When {
		return pq[i].sequence < pq[j].sequence
	}
	return pq[i].When < pq[j].When
}

func (pq messagePriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

func (pq *messagePriorityQueue) Push(x any) {
	n := len(*pq)
	item := x.(*Message)
	item.index = n
	*pq = append(*pq, item)
}

func (pq *messagePriorityQueue) Pop() any {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	*pq = old[0 : n-1]
	return item
}

// FrameCallback represents a choreographer vsync frame callback
type FrameCallback func(frameTimeNanos int64)

// IdleHandler is called when the message queue has run out of tasks and is idling
type IdleHandler func() bool

// MessageQueue mocks android.os.MessageQueue
type MessageQueue struct {
	mu           sync.Mutex
	pq           messagePriorityQueue
	seqGen       atomic.Uint64
	isQuited     bool
	idleHandlers []IdleHandler
}

// NewMessageQueue creates an empty initialized MessageQueue
func NewMessageQueue() *MessageQueue {
	mq := &MessageQueue{
		pq:           make(messagePriorityQueue, 0, 64),
		idleHandlers: make([]IdleHandler, 0),
	}
	heap.Init(&mq.pq)
	return mq
}

// AddIdleHandler adds an idle listener
func (mq *MessageQueue) AddIdleHandler(handler IdleHandler) {
	mq.mu.Lock()
	defer mq.mu.Unlock()
	mq.idleHandlers = append(mq.idleHandlers, handler)
}

// RemoveCallbacksAndMessages removes callbacks and messages matching token (or all if token == nil)
func (mq *MessageQueue) RemoveCallbacksAndMessages(h *Handler, token any) {
	mq.mu.Lock()
	defer mq.mu.Unlock()
	newPQ := make(messagePriorityQueue, 0, len(mq.pq))
	for _, m := range mq.pq {
		if m.Target == h {
			if token == nil || m.Obj == token {
				continue
			}
		}
		newPQ = append(newPQ, m)
	}
	mq.pq = newPQ
	heap.Init(&mq.pq)
}

// EnqueueMessage inserts a message into the priority queue ordered by execution time
func (mq *MessageQueue) EnqueueMessage(msg *Message, when int64) bool {
	mq.mu.Lock()
	defer mq.mu.Unlock()
	if mq.isQuited {
		return false
	}
	msg.When = when
	msg.sequence = mq.seqGen.Add(1)
	heap.Push(&mq.pq, msg)
	return true
}

// RemoveMessages removes messages matching what and/or object
func (mq *MessageQueue) RemoveMessages(h *Handler, what int, obj any) {
	mq.mu.Lock()
	defer mq.mu.Unlock()
	newPQ := make(messagePriorityQueue, 0, len(mq.pq))
	for _, m := range mq.pq {
		if m.Target == h && m.What == what && (obj == nil || m.Obj == obj) {
			continue
		}
		newPQ = append(newPQ, m)
	}
	mq.pq = newPQ
	heap.Init(&mq.pq)
}

// RemoveCallbacks removes messages matching callback
func (mq *MessageQueue) RemoveCallbacks(h *Handler, r func()) {
	mq.mu.Lock()
	defer mq.mu.Unlock()
	// Note: In Go, func pointers cannot be directly compared for equality with ==,
	// but we can remove any callback message for this handler or match by address/tag if provided
	newPQ := make(messagePriorityQueue, 0, len(mq.pq))
	for _, m := range mq.pq {
		if m.Target == h && m.Callback != nil {
			continue
		}
		newPQ = append(newPQ, m)
	}
	mq.pq = newPQ
	heap.Init(&mq.pq)
}

// Count returns the number of pending messages in the queue
func (mq *MessageQueue) Count() int {
	mq.mu.Lock()
	defer mq.mu.Unlock()
	return len(mq.pq)
}

// VirtualLooper implements NanoLooper: a deterministic, zero-sleep Android looper
type VirtualLooper struct {
	mu             sync.Mutex
	virtualTimeMs  int64
	queue          *MessageQueue
	frameCallbacks []FrameCallback
	frameNanosStep int64 // default 16_666_666 ns (~60 fps)
}

// NewVirtualLooper creates a new VirtualLooper starting at the specified virtual millisecond time
func NewVirtualLooper(initialUptimeMs ...int64) *VirtualLooper {
	initTime := int64(0)
	if len(initialUptimeMs) > 0 {
		initTime = initialUptimeMs[0]
	}
	return &VirtualLooper{
		virtualTimeMs:  initTime,
		queue:          NewMessageQueue(),
		frameCallbacks: make([]FrameCallback, 0),
		frameNanosStep: 16_666_666,
	}
}

// UptimeMillis intercepts android.os.SystemClock.uptimeMillis()
func (vl *VirtualLooper) UptimeMillis() int64 {
	vl.mu.Lock()
	defer vl.mu.Unlock()
	return vl.virtualTimeMs
}

// SetUptimeMillis sets the current virtual uptime
func (vl *VirtualLooper) SetUptimeMillis(uptimeMs int64) {
	vl.mu.Lock()
	defer vl.mu.Unlock()
	vl.virtualTimeMs = uptimeMs
}

// GetQueue returns the associated MessageQueue
func (vl *VirtualLooper) GetQueue() *MessageQueue {
	return vl.queue
}

// Post schedules a Runnable to run immediately at current virtual time
func (vl *VirtualLooper) Post(runnable func()) bool {
	return vl.PostDelayed(runnable, 0)
}

// PostDelayed schedules a Runnable to run after delayMs virtual milliseconds
func (vl *VirtualLooper) PostDelayed(runnable func(), delayMs int64) bool {
	vl.mu.Lock()
	when := vl.virtualTimeMs + delayMs
	vl.mu.Unlock()

	msg := &Message{
		Callback: runnable,
	}
	return vl.queue.EnqueueMessage(msg, when)
}

// PostFrameCallback registers a vsync animation frame callback (Choreographer mock)
func (vl *VirtualLooper) PostFrameCallback(cb FrameCallback) {
	vl.mu.Lock()
	defer vl.mu.Unlock()
	vl.frameCallbacks = append(vl.frameCallbacks, cb)
}

// Step advances virtual time by deltaMs and synchronously executes all messages scheduled <= new time
func (vl *VirtualLooper) Step(deltaMs int64) error {
	if deltaMs < 0 {
		return errors.New("deltaMs cannot be negative")
	}

	vl.mu.Lock()
	targetTime := vl.virtualTimeMs + deltaMs
	vl.mu.Unlock()

	for {
		var nextMsg *Message

		// Inspect next message under lock
		vl.queue.mu.Lock()
		if len(vl.queue.pq) > 0 {
			peek := vl.queue.pq[0]
			if peek.When <= targetTime {
				nextMsg = heap.Pop(&vl.queue.pq).(*Message)
			}
		}
		vl.queue.mu.Unlock()

		if nextMsg == nil {
			// No more messages due in this window
			break
		}

		// Advance virtual time to the message execution timestamp
		vl.mu.Lock()
		if nextMsg.When > vl.virtualTimeMs {
			vl.virtualTimeMs = nextMsg.When
		}
		vl.mu.Unlock()

		// Execute message outside locks to prevent deadlocks
		if nextMsg.Target != nil {
			nextMsg.Target.DispatchMessage(nextMsg)
		} else if nextMsg.Callback != nil {
			nextMsg.Callback()
		}
	}

	// Advance virtual time to target time if no remaining messages pushed it further
	vl.mu.Lock()
	if vl.virtualTimeMs < targetTime {
		vl.virtualTimeMs = targetTime
	}

	// Dispatch any registered frame callbacks if virtual time advanced
	frames := vl.frameCallbacks
	vl.frameCallbacks = make([]FrameCallback, 0)
	currentNanos := vl.virtualTimeMs * 1_000_000
	vl.mu.Unlock()

	for _, cb := range frames {
		cb(currentNanos)
	}

	// Process idle handlers if queue is currently idle
	vl.queue.mu.Lock()
	idlePending := len(vl.queue.pq) == 0 || vl.queue.pq[0].When > vl.virtualTimeMs
	var idlers []IdleHandler
	if idlePending && len(vl.queue.idleHandlers) > 0 {
		idlers = make([]IdleHandler, len(vl.queue.idleHandlers))
		copy(idlers, vl.queue.idleHandlers)
	}
	vl.queue.mu.Unlock()

	if len(idlers) > 0 {
		var surviving []IdleHandler
		for _, ih := range idlers {
			if ih() {
				surviving = append(surviving, ih)
			}
		}
		vl.queue.mu.Lock()
		vl.queue.idleHandlers = surviving
		vl.queue.mu.Unlock()
	}

	return nil
}

// StepLooper advances virtual looper by deltaMs and returns the remaining queued messages count
func (vl *VirtualLooper) StepLooper(deltaMs int64) (remainingMessages int, err error) {
	if err := vl.Step(deltaMs); err != nil {
		return vl.queue.Count(), err
	}
	return vl.queue.Count(), nil
}

// Handler mocks android.os.Handler
type Handler struct {
	looper        *VirtualLooper
	handleMsgFunc func(msg *Message)
	mu            sync.RWMutex
}

// NewHandler creates a Handler attached to the specified VirtualLooper
func NewHandler(looper *VirtualLooper, onMessage ...func(msg *Message)) *Handler {
	var fn func(msg *Message)
	if len(onMessage) > 0 {
		fn = onMessage[0]
	}
	return &Handler{
		looper:        looper,
		handleMsgFunc: fn,
	}
}

// GetLooper returns the looper this handler is bound to
func (h *Handler) GetLooper() *VirtualLooper {
	return h.looper
}

// Post posts a runnable to be executed at current virtual uptime
func (h *Handler) Post(r func()) bool {
	return h.PostDelayed(r, 0)
}

// PostDelayed posts a runnable to be executed after delayMs
func (h *Handler) PostDelayed(r func(), delayMs int64) bool {
	uptime := h.looper.UptimeMillis() + delayMs
	return h.PostAtTime(r, uptime)
}

// PostAtTime posts a runnable at a specific virtual uptime
func (h *Handler) PostAtTime(r func(), uptimeMs int64) bool {
	msg := &Message{
		Target:   h,
		Callback: r,
	}
	return h.looper.GetQueue().EnqueueMessage(msg, uptimeMs)
}

// SendMessage sends an immediate message
func (h *Handler) SendMessage(msg *Message) bool {
	return h.SendMessageDelayed(msg, 0)
}

// SendMessageDelayed sends a message with a virtual delay
func (h *Handler) SendMessageDelayed(msg *Message, delayMs int64) bool {
	uptime := h.looper.UptimeMillis() + delayMs
	return h.SendMessageAtTime(msg, uptime)
}

// SendMessageAtTime sends a message at a specific virtual uptime
func (h *Handler) SendMessageAtTime(msg *Message, uptimeMs int64) bool {
	msg.Target = h
	return h.looper.GetQueue().EnqueueMessage(msg, uptimeMs)
}

// RemoveCallbacks removes callback runnables
func (h *Handler) RemoveCallbacks(r func()) {
	h.looper.GetQueue().RemoveCallbacks(h, r)
}

// RemoveCallbacksAndMessages removes callbacks and messages matching token (or all if token == nil)
func (h *Handler) RemoveCallbacksAndMessages(token any) {
	h.looper.GetQueue().RemoveCallbacksAndMessages(h, token)
}

// RemoveMessages removes messages with matching what
func (h *Handler) RemoveMessages(what int) {
	h.looper.GetQueue().RemoveMessages(h, what, nil)
}

// DispatchMessage routes message to callback or handleMessage
func (h *Handler) DispatchMessage(msg *Message) {
	if msg.Callback != nil {
		msg.Callback()
		return
	}
	h.HandleMessage(msg)
}

// HandleMessage executes the message handler
func (h *Handler) HandleMessage(msg *Message) {
	h.mu.RLock()
	fn := h.handleMsgFunc
	h.mu.RUnlock()
	if fn != nil {
		fn(msg)
	}
}

// SetMessageCallback updates the message handler function
func (h *Handler) SetMessageCallback(fn func(msg *Message)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.handleMsgFunc = fn
}

// SystemClock mocks android.os.SystemClock
type SystemClock struct {
	looper *VirtualLooper
}

// NewSystemClock creates a SystemClock bound to a virtual looper
func NewSystemClock(looper *VirtualLooper) *SystemClock {
	return &SystemClock{looper: looper}
}

// UptimeMillis returns the intercepted virtual uptime
func (sc *SystemClock) UptimeMillis() int64 {
	if sc.looper != nil {
		return sc.looper.UptimeMillis()
	}
	return 0
}

// Sleep simulates virtual sleep without blocking wall-clock time
func (sc *SystemClock) Sleep(ms int64) error {
	if sc.looper != nil {
		return sc.looper.Step(ms)
	}
	return nil
}

// Global default virtual looper for convenience
var (
	defaultLooperLock sync.RWMutex
	defaultLooper     *VirtualLooper
)

// GetDefaultLooper returns the singleton virtual looper
func GetDefaultLooper() *VirtualLooper {
	defaultLooperLock.RLock()
	if defaultLooper != nil {
		defer defaultLooperLock.RUnlock()
		return defaultLooper
	}
	defaultLooperLock.RUnlock()

	defaultLooperLock.Lock()
	defer defaultLooperLock.Unlock()
	if defaultLooper == nil {
		defaultLooper = NewVirtualLooper(0)
	}
	return defaultLooper
}

// ResetDefaultLooper resets the global default looper to a fresh state
func ResetDefaultLooper(initialUptimeMs ...int64) *VirtualLooper {
	defaultLooperLock.Lock()
	defer defaultLooperLock.Unlock()
	defaultLooper = NewVirtualLooper(initialUptimeMs...)
	return defaultLooper
}

// VirtualHandlerThread mocks android.os.HandlerThread
type VirtualHandlerThread struct {
	name   string
	looper *VirtualLooper
}

// NewVirtualHandlerThread creates a background thread with its own VirtualLooper
func NewVirtualHandlerThread(name string, initialUptimeMs ...int64) *VirtualHandlerThread {
	return &VirtualHandlerThread{
		name:   name,
		looper: NewVirtualLooper(initialUptimeMs...),
	}
}

// GetLooper returns the looper associated with this thread
func (ht *VirtualHandlerThread) GetLooper() *VirtualLooper {
	return ht.looper
}

// Step advances this thread's looper
func (ht *VirtualHandlerThread) Step(deltaMs int64) error {
	return ht.looper.Step(deltaMs)
}

// Quit terminates the thread's looper queue
func (ht *VirtualHandlerThread) Quit() {
	ht.looper.GetQueue().mu.Lock()
	defer ht.looper.GetQueue().mu.Unlock()
	ht.looper.GetQueue().isQuited = true
	ht.looper.GetQueue().pq = nil
}
