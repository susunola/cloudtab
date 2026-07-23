package pricing

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// countingBackend is a fake pricing backend used to exercise the engine's
// retry, in-flight de-duplication and caching behaviour without any network.
// It records how many times query() was actually invoked and can be scripted
// to fail a given number of times before succeeding, optionally blocking until
// released so concurrent de-duplication can be observed deterministically.
type countingBackend struct {
	calls    int32
	failN    int32 // fail the first failN calls, then succeed
	failWith error
	resp     []byte

	gate chan struct{} // when non-nil, query blocks until closed
}

func (b *countingBackend) query(_ PriceRequest) ([]byte, error) {
	n := atomic.AddInt32(&b.calls, 1)
	if b.gate != nil {
		<-b.gate
	}
	if n <= atomic.LoadInt32(&b.failN) {
		return nil, b.failWith
	}
	return b.resp, nil
}

// engineWithBackend builds an Engine whose AWS path is pre-wired to the given
// fake backend (awsOnce already fired), so PriceRequests with Provider "aws"
// route straight to it. cfg controls timeout/retry/cache knobs.
func engineWithBackend(cfg Config, b backend) *Engine {
	e := &Engine{
		cfg:     cfg,
		clients: map[string]interface{}{},
		flight:  map[string]*inflightCall{},
		aws:     b,
	}
	e.awsOnce.Do(func() {}) // mark done so awsBackend() returns our fake
	return e
}

func awsReq() PriceRequest {
	return PriceRequest{Provider: "aws", Product: "AmazonEC2", Region: "us-east-1"}
}

func TestConfigTimeoutAndRetryDefaults(t *testing.T) {
	var zero Config
	if got := zero.requestTimeout(); got != defaultRequestTimeout {
		t.Errorf("default timeout = %v, want %v", got, defaultRequestTimeout)
	}
	if got := zero.maxRetries(); got != defaultMaxRetries {
		t.Errorf("default retries = %d, want %d", got, defaultMaxRetries)
	}
	if got := (Config{Timeout: 5 * time.Second}).requestTimeout(); got != 5*time.Second {
		t.Errorf("explicit timeout = %v, want 5s", got)
	}
	if got := (Config{MaxRetries: 4}).maxRetries(); got != 4 {
		t.Errorf("explicit retries = %d, want 4", got)
	}
	// A negative MaxRetries disables retries (0 additional attempts).
	if got := (Config{MaxRetries: -1}).maxRetries(); got != 0 {
		t.Errorf("negative retries = %d, want 0", got)
	}
}

func TestTimeoutSeconds(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want int
	}{
		{0, int(defaultRequestTimeout / time.Second)},
		{-1, int(defaultRequestTimeout / time.Second)},
		{30 * time.Second, 30},
		{500 * time.Millisecond, 1}, // rounds up, never 0
		{1500 * time.Millisecond, 2},
		{time.Nanosecond, 1},
	}
	for _, c := range cases {
		if got := timeoutSeconds(c.in); got != c.want {
			t.Errorf("timeoutSeconds(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestIsRetryable(t *testing.T) {
	retry := []error{
		errors.New("tencent api RequestLimitExceeded: too frequent"),
		errors.New("Foo.LimitExceeded"),
		errors.New("ThrottlingException: rate exceeded"),
		errors.New("TooManyRequestsException"),
		errors.New("tencent api InternalError: try later"),
		errors.New("ServiceUnavailable"),
		errors.New("context deadline exceeded"),
		errors.New("dial tcp: i/o timeout"),
		errors.New("read: connection reset by peer"),
		errors.New("unexpected EOF"),
	}
	for _, e := range retry {
		if !isRetryable(e) {
			t.Errorf("expected retryable: %v", e)
		}
	}
	noRetry := []error{
		nil,
		errors.New("unsupported product \"foo\""),
		errors.New("bind params: bad field"),
		errors.New("aws price list: no matching products"),
		errors.New("InvalidParameterValue"),
	}
	for _, e := range noRetry {
		if isRetryable(e) {
			t.Errorf("expected NOT retryable: %v", e)
		}
	}
}

func TestDispatchWithRetrySucceedsAfterTransientErrors(t *testing.T) {
	b := &countingBackend{
		failN:    2, // fail twice, succeed on the 3rd
		failWith: errors.New("ThrottlingException: slow down"),
		resp:     []byte(`["ok"]`),
	}
	// 2 retries -> 3 attempts total; fast backoff base is fine for the test.
	e := engineWithBackend(Config{MaxRetries: 2}, b)

	resp, err := e.dispatchWithRetry(awsReq())
	if err != nil {
		t.Fatalf("expected success after retries, got %v", err)
	}
	if string(resp) != `["ok"]` {
		t.Fatalf("resp = %s", resp)
	}
	if got := atomic.LoadInt32(&b.calls); got != 3 {
		t.Fatalf("backend called %d times, want 3", got)
	}
}

func TestDispatchWithRetryStopsAtBudget(t *testing.T) {
	b := &countingBackend{
		failN:    99, // always fail
		failWith: errors.New("Throttling: nope"),
	}
	e := engineWithBackend(Config{MaxRetries: 2}, b)

	_, err := e.dispatchWithRetry(awsReq())
	if err == nil {
		t.Fatal("expected failure after exhausting retries")
	}
	if got := atomic.LoadInt32(&b.calls); got != 3 { // 1 + 2 retries
		t.Fatalf("backend called %d times, want 3", got)
	}
}

func TestDispatchWithRetryDoesNotRetryPermanentError(t *testing.T) {
	b := &countingBackend{
		failN:    99,
		failWith: errors.New("InvalidParameterValue: bad instanceType"),
	}
	e := engineWithBackend(Config{MaxRetries: 5}, b)

	_, err := e.dispatchWithRetry(awsReq())
	if err == nil {
		t.Fatal("expected failure")
	}
	if got := atomic.LoadInt32(&b.calls); got != 1 {
		t.Fatalf("permanent error retried: called %d times, want 1", got)
	}
}

func TestInflightDedupCollapsesConcurrentIdenticalRequests(t *testing.T) {
	b := &countingBackend{
		resp: []byte(`["shared"]`),
		gate: make(chan struct{}),
	}
	// No cache: dedup must stand on its own. Disable retries for determinism.
	e := engineWithBackend(Config{MaxRetries: -1}, b)

	const n = 16
	var wg sync.WaitGroup
	got := make([][]byte, n)
	errs := make([]error, n)
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			got[idx], errs[idx] = e.Query(awsReq())
		}(i)
	}
	close(start)

	// Give all goroutines time to converge on the same in-flight key, then
	// release the single backend call.
	time.Sleep(50 * time.Millisecond)
	close(b.gate)
	wg.Wait()

	if calls := atomic.LoadInt32(&b.calls); calls != 1 {
		t.Fatalf("dedup failed: backend called %d times, want 1", calls)
	}
	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Fatalf("waiter %d error: %v", i, errs[i])
		}
		if string(got[i]) != `["shared"]` {
			t.Fatalf("waiter %d got %s", i, got[i])
		}
	}
}

func TestInflightDedupClearsAfterCompletion(t *testing.T) {
	b := &countingBackend{resp: []byte(`["x"]`)}
	e := engineWithBackend(Config{MaxRetries: -1}, b)

	// Two sequential queries: dedup only collapses CONCURRENT calls, so with no
	// cache each sequential call should hit the backend and the in-flight map
	// must be empty between them (no leak).
	if _, err := e.Query(awsReq()); err != nil {
		t.Fatal(err)
	}
	e.flightMu.Lock()
	leaked := len(e.flight)
	e.flightMu.Unlock()
	if leaked != 0 {
		t.Fatalf("in-flight map leaked %d entries", leaked)
	}
	if _, err := e.Query(awsReq()); err != nil {
		t.Fatal(err)
	}
	if calls := atomic.LoadInt32(&b.calls); calls != 2 {
		t.Fatalf("sequential uncached calls hit backend %d times, want 2", calls)
	}
}

func TestQueryFailureNotCached(t *testing.T) {
	dir := t.TempDir()
	c, err := openCache(dir + "/cache.db")
	if err != nil {
		t.Fatalf("openCache: %v", err)
	}
	defer c.Close()

	b := &countingBackend{
		failN:    1, // fail once, then succeed
		failWith: errors.New("Throttling: transient"),
		resp:     []byte(`["good"]`),
	}
	e := engineWithBackend(Config{MaxRetries: -1}, b) // no auto-retry
	e.cache = c

	// First call fails (retries disabled) and must NOT be cached.
	if _, err := e.Query(awsReq()); err == nil {
		t.Fatal("expected first call to fail")
	}
	// Second call should reach the backend again (proving the failure was not
	// cached) and now succeed.
	resp, err := e.Query(awsReq())
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if string(resp) != `["good"]` {
		t.Fatalf("resp = %s", resp)
	}
	if calls := atomic.LoadInt32(&b.calls); calls != 2 {
		t.Fatalf("backend called %d times, want 2 (failure must not be cached)", calls)
	}
	// Third call must now be served from cache (no new backend hit).
	if _, err := e.Query(awsReq()); err != nil {
		t.Fatalf("third call: %v", err)
	}
	if calls := atomic.LoadInt32(&b.calls); calls != 2 {
		t.Fatalf("cache miss on third call: backend called %d times, want 2", calls)
	}
}
