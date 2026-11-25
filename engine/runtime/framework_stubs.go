package runtime

import (
	"fmt"
	"sync"
)

// ActivityState captures the Android Activity lifecycle state
type ActivityState string

const (
	StateUninitialized ActivityState = "UNINITIALIZED"
	StateCreated       ActivityState = "CREATED"
	StateStarted       ActivityState = "STARTED"
	StateResumed       ActivityState = "RESUMED"
	StatePaused        ActivityState = "PAUSED"
	StateStopped       ActivityState = "STOPPED"
	StateDestroyed     ActivityState = "DESTROYED"
)

// Bundle mocks android.os.Bundle
type Bundle struct {
	mu   sync.RWMutex
	data map[string]any
}

// NewBundle creates an empty initialized Bundle
func NewBundle() *Bundle {
	return &Bundle{
		data: make(map[string]any),
	}
}

// PutString sets a string extra
func (b *Bundle) PutString(key, val string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data[key] = val
}

// GetString gets a string extra or default empty string
func (b *Bundle) GetString(key string) string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if v, ok := b.data[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
		return fmt.Sprintf("%v", v)
	}
	return ""
}

// PutInt sets an int extra
func (b *Bundle) PutInt(key string, val int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data[key] = val
}

// GetInt gets an int extra or default value
func (b *Bundle) GetInt(key string, def int) int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if v, ok := b.data[key]; ok {
		switch num := v.(type) {
		case int:
			return num
		case int32:
			return int(num)
		case int64:
			return int(num)
		case float64:
			return int(num)
		}
	}
	return def
}

// PutBoolean sets a boolean extra
func (b *Bundle) PutBoolean(key string, val bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data[key] = val
}

// GetBoolean gets a boolean extra or default value
func (b *Bundle) GetBoolean(key string, def bool) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if v, ok := b.data[key]; ok {
		if val, ok := v.(bool); ok {
			return val
		}
	}
	return def
}

// PutAll adds all key-value pairs from a map
func (b *Bundle) PutAll(m map[string]any) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for k, v := range m {
		b.data[k] = v
	}
}

// ContainsKey checks if key exists in bundle
func (b *Bundle) ContainsKey(key string) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	_, ok := b.data[key]
	return ok
}

// KeySet returns all keys in the bundle
func (b *Bundle) KeySet() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	keys := make([]string, 0, len(b.data))
	for k := range b.data {
		keys = append(keys, k)
	}
	return keys
}

// ToMap returns a copy of the bundle as a map
func (b *Bundle) ToMap() map[string]any {
	b.mu.RLock()
	defer b.mu.RUnlock()
	res := make(map[string]any, len(b.data))
	for k, v := range b.data {
		res[k] = v
	}
	return res
}

// Intent mocks android.content.Intent
type Intent struct {
	mu         sync.RWMutex
	action     string
	categories map[string]struct{}
	extras     *Bundle
	component  string
	data       string
	flags      int
}

// NewIntent creates a new Intent with optional action
func NewIntent(action ...string) *Intent {
	act := ""
	if len(action) > 0 {
		act = action[0]
	}
	return &Intent{
		action:     act,
		categories: make(map[string]struct{}),
		extras:     NewBundle(),
	}
}

// SetAction sets the Intent action string
func (in *Intent) SetAction(action string) *Intent {
	in.mu.Lock()
	defer in.mu.Unlock()
	in.action = action
	return in
}

// GetAction retrieves the Intent action
func (in *Intent) GetAction() string {
	in.mu.RLock()
	defer in.mu.RUnlock()
	return in.action
}

// AddCategory adds an intent category
func (in *Intent) AddCategory(category string) *Intent {
	in.mu.Lock()
	defer in.mu.Unlock()
	in.categories[category] = struct{}{}
	return in
}

// HasCategory checks if a category exists
func (in *Intent) HasCategory(category string) bool {
	in.mu.RLock()
	defer in.mu.RUnlock()
	_, ok := in.categories[category]
	return ok
}

// GetCategories returns all categories
func (in *Intent) GetCategories() []string {
	in.mu.RLock()
	defer in.mu.RUnlock()
	cats := make([]string, 0, len(in.categories))
	for c := range in.categories {
		cats = append(cats, c)
	}
	return cats
}

// PutExtra attaches extra data to Intent
func (in *Intent) PutExtra(key string, val any) *Intent {
	in.mu.Lock()
	defer in.mu.Unlock()
	in.extras.PutAll(map[string]any{key: val})
	return in
}

// GetExtras returns the Bundle of extras
func (in *Intent) GetExtras() *Bundle {
	in.mu.RLock()
	defer in.mu.RUnlock()
	return in.extras
}

// GetStringExtra retrieves a string extra
func (in *Intent) GetStringExtra(key string) string {
	return in.extras.GetString(key)
}

// GetIntExtra retrieves an int extra
func (in *Intent) GetIntExtra(key string, def int) int {
	return in.extras.GetInt(key, def)
}

// GetBooleanExtra retrieves a boolean extra
func (in *Intent) GetBooleanExtra(key string, def bool) bool {
	return in.extras.GetBoolean(key, def)
}

// SharedPreferences mocks android.content.SharedPreferences
type SharedPreferences struct {
	mu     sync.RWMutex
	name   string
	values map[string]any
}

// NewSharedPreferences creates a new SharedPreferences store
func NewSharedPreferences(name string) *SharedPreferences {
	return &SharedPreferences{
		name:   name,
		values: make(map[string]any),
	}
}

// GetString returns string preference or def
func (sp *SharedPreferences) GetString(key, def string) string {
	sp.mu.RLock()
	defer sp.mu.RUnlock()
	if v, ok := sp.values[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return def
}

// GetInt returns int preference or def
func (sp *SharedPreferences) GetInt(key string, def int) int {
	sp.mu.RLock()
	defer sp.mu.RUnlock()
	if v, ok := sp.values[key]; ok {
		if num, ok := v.(int); ok {
			return num
		}
	}
	return def
}

// GetBoolean returns boolean preference or def
func (sp *SharedPreferences) GetBoolean(key string, def bool) bool {
	sp.mu.RLock()
	defer sp.mu.RUnlock()
	if v, ok := sp.values[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return def
}

// Contains checks if a key exists
func (sp *SharedPreferences) Contains(key string) bool {
	sp.mu.RLock()
	defer sp.mu.RUnlock()
	_, ok := sp.values[key]
	return ok
}

// Edit returns an Editor to mutate preferences
func (sp *SharedPreferences) Edit() *SharedPreferencesEditor {
	return &SharedPreferencesEditor{
		prefs:   sp,
		changes: make(map[string]any),
		removes: make(map[string]struct{}),
	}
}

// SharedPreferencesEditor mocks android.content.SharedPreferences.Editor
type SharedPreferencesEditor struct {
	mu      sync.Mutex
	prefs   *SharedPreferences
	changes map[string]any
	removes map[string]struct{}
	clear   bool
}

// PutString stages a string modification
func (e *SharedPreferencesEditor) PutString(key, val string) *SharedPreferencesEditor {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.changes[key] = val
	delete(e.removes, key)
	return e
}

// PutInt stages an int modification
func (e *SharedPreferencesEditor) PutInt(key string, val int) *SharedPreferencesEditor {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.changes[key] = val
	delete(e.removes, key)
	return e
}

// PutBoolean stages a boolean modification
func (e *SharedPreferencesEditor) PutBoolean(key string, val bool) *SharedPreferencesEditor {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.changes[key] = val
	delete(e.removes, key)
	return e
}

// Remove stages removing a key
func (e *SharedPreferencesEditor) Remove(key string) *SharedPreferencesEditor {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.removes[key] = struct{}{}
	delete(e.changes, key)
	return e
}

// Clear stages clearing all keys
func (e *SharedPreferencesEditor) Clear() *SharedPreferencesEditor {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.clear = true
	return e
}

// Apply asynchronously applies modifications
func (e *SharedPreferencesEditor) Apply() {
	e.Commit()
}

// Commit synchronously applies modifications
func (e *SharedPreferencesEditor) Commit() bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.prefs.mu.Lock()
	defer e.prefs.mu.Unlock()

	if e.clear {
		e.prefs.values = make(map[string]any)
	}
	for k := range e.removes {
		delete(e.prefs.values, k)
	}
	for k, v := range e.changes {
		e.prefs.values[k] = v
	}

	e.changes = make(map[string]any)
	e.removes = make(map[string]struct{})
	e.clear = false
	return true
}

// Resources mocks android.content.res.Resources
type Resources struct {
	mu      sync.RWMutex
	strings map[string]string
	colors  map[string]uint32
}

// NewResources creates an initialized Resources instance
func NewResources() *Resources {
	return &Resources{
		strings: make(map[string]string),
		colors:  make(map[string]uint32),
	}
}

// SetString stores a string resource
func (r *Resources) SetString(name, val string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.strings[name] = val
}

// GetString retrieves a string resource
func (r *Resources) GetString(name string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.strings[name]
}

// Context mocks android.content.Context
type Context struct {
	mu          sync.RWMutex
	packageName string
	sharedPrefs map[string]*SharedPreferences
	resources   *Resources
	mainLooper  *VirtualLooper
}

// NewContext creates a new initialized Context
func NewContext(packageName string, looper *VirtualLooper) *Context {
	return &Context{
		packageName: packageName,
		sharedPrefs: make(map[string]*SharedPreferences),
		resources:   NewResources(),
		mainLooper:  looper,
	}
}

// GetPackageName returns the Android package name
func (ctx *Context) GetPackageName() string {
	ctx.mu.RLock()
	defer ctx.mu.RUnlock()
	return ctx.packageName
}

// SetPackageName updates the package name
func (ctx *Context) SetPackageName(pkg string) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	ctx.packageName = pkg
}

// GetSharedPreferences retrieves or creates a named SharedPreferences store
func (ctx *Context) GetSharedPreferences(name string, mode int) *SharedPreferences {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	if sp, ok := ctx.sharedPrefs[name]; ok {
		return sp
	}
	sp := NewSharedPreferences(name)
	ctx.sharedPrefs[name] = sp
	return sp
}

// GetResources retrieves the Resources mock
func (ctx *Context) GetResources() *Resources {
	ctx.mu.RLock()
	defer ctx.mu.RUnlock()
	return ctx.resources
}

// GetMainLooper returns the virtual main looper
func (ctx *Context) GetMainLooper() *VirtualLooper {
	ctx.mu.RLock()
	defer ctx.mu.RUnlock()
	return ctx.mainLooper
}

// Activity mocks android.app.Activity
type Activity struct {
	mu               sync.RWMutex
	context          *Context
	intent           *Intent
	state            ActivityState
	lifecycleHistory []ActivityState
	isFinishing      bool
	callbacks        map[ActivityState]func()
	contentView      *View
}

// NewActivity creates an initialized Activity
func NewActivity() *Activity {
	return &Activity{
		state:            StateUninitialized,
		lifecycleHistory: make([]ActivityState, 0, 8),
		callbacks:        make(map[ActivityState]func()),
	}
}

// Attach binds the Context and Intent to the Activity
func (a *Activity) Attach(ctx *Context, intent *Intent) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.context = ctx
	a.intent = intent
}

// OnCreate triggers onCreate lifecycle callback
func (a *Activity) OnCreate(savedInstanceState *Bundle) {
	a.mu.Lock()
	a.state = StateCreated
	a.lifecycleHistory = append(a.lifecycleHistory, StateCreated)
	cb := a.callbacks[StateCreated]
	a.mu.Unlock()
	if cb != nil {
		cb()
	}
}

// OnStart triggers onStart lifecycle callback
func (a *Activity) OnStart() {
	a.mu.Lock()
	a.state = StateStarted
	a.lifecycleHistory = append(a.lifecycleHistory, StateStarted)
	cb := a.callbacks[StateStarted]
	a.mu.Unlock()
	if cb != nil {
		cb()
	}
}

// OnResume triggers onResume lifecycle callback
func (a *Activity) OnResume() {
	a.mu.Lock()
	a.state = StateResumed
	a.lifecycleHistory = append(a.lifecycleHistory, StateResumed)
	cb := a.callbacks[StateResumed]
	a.mu.Unlock()
	if cb != nil {
		cb()
	}
}

// OnPause triggers onPause lifecycle callback
func (a *Activity) OnPause() {
	a.mu.Lock()
	a.state = StatePaused
	a.lifecycleHistory = append(a.lifecycleHistory, StatePaused)
	cb := a.callbacks[StatePaused]
	a.mu.Unlock()
	if cb != nil {
		cb()
	}
}

// OnStop triggers onStop lifecycle callback
func (a *Activity) OnStop() {
	a.mu.Lock()
	a.state = StateStopped
	a.lifecycleHistory = append(a.lifecycleHistory, StateStopped)
	cb := a.callbacks[StateStopped]
	a.mu.Unlock()
	if cb != nil {
		cb()
	}
}

// OnDestroy triggers onDestroy lifecycle callback
func (a *Activity) OnDestroy() {
	a.mu.Lock()
	a.state = StateDestroyed
	a.lifecycleHistory = append(a.lifecycleHistory, StateDestroyed)
	cb := a.callbacks[StateDestroyed]
	a.mu.Unlock()
	if cb != nil {
		cb()
	}
}

// RegisterLifecycleCallback registers a custom listener for a lifecycle state
func (a *Activity) RegisterLifecycleCallback(state ActivityState, callback func()) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.callbacks[state] = callback
}

// RunOnUiThread posts a runnable to the main looper's message queue
func (a *Activity) RunOnUiThread(action func()) {
	a.mu.RLock()
	looper := a.context.GetMainLooper()
	a.mu.RUnlock()
	if looper != nil {
		looper.Post(action)
	} else if action != nil {
		action()
	}
}

// Finish marks the activity as finishing
func (a *Activity) Finish() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.isFinishing = true
}

// IsFinishing returns true if finish() was requested
func (a *Activity) IsFinishing() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.isFinishing
}

// GetState returns current lifecycle state
func (a *Activity) GetState() ActivityState {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.state
}

// GetLifecycleHistory returns a copy of states traversed
func (a *Activity) GetLifecycleHistory() []ActivityState {
	a.mu.RLock()
	defer a.mu.RUnlock()
	res := make([]ActivityState, len(a.lifecycleHistory))
	copy(res, a.lifecycleHistory)
	return res
}

// GetIntent returns the current intent
func (a *Activity) GetIntent() *Intent {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.intent
}

// SetIntent updates the current intent
func (a *Activity) SetIntent(intent *Intent) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.intent = intent
}

// GetPackageName returns package name from context
func (a *Activity) GetPackageName() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.context != nil {
		return a.context.GetPackageName()
	}
	return ""
}

// GetApplicationContext returns context
func (a *Activity) GetApplicationContext() *Context {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.context
}

// GetSharedPreferences accesses SharedPreferences via context
func (a *Activity) GetSharedPreferences(name string, mode int) *SharedPreferences {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.context != nil {
		return a.context.GetSharedPreferences(name, mode)
	}
	return nil
}

