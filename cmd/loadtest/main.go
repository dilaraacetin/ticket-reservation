// Command loadtest drives the API hard enough to show where it bends.
//
// It is here rather than in a k6 or vegeta script because the interesting
// scenario is a sequence — hold a seat, then give it up — carried out by many
// callers at once, each with its own token. A tool that replays one request at a
// time cannot express that.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand/v2"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"ticket-reservation/internal/auth"
)

type options struct {
	url      string
	secret   string
	event    string
	seats    int
	users    int
	waiters  int
	duration time.Duration
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "loadtest:", err)
		os.Exit(1)
	}
}

func run() error {
	var opts options

	flag.StringVar(&opts.url, "url", "http://localhost:8099", "where the server is")
	flag.StringVar(&opts.secret, "secret", "", "AUTH_SECRET of that server, for minting tokens")
	flag.StringVar(&opts.event, "event", "event-1", "which event to hammer")
	flag.IntVar(&opts.seats, "seats", 10, "how many seats the event has")
	flag.IntVar(&opts.users, "users", 20, "callers holding and releasing seats")
	flag.IntVar(&opts.waiters, "waiters", 5, "callers sitting in the waiting list")
	flag.DurationVar(&opts.duration, "for", 15*time.Second, "how long to keep going")
	flag.Parse()

	if opts.secret == "" {
		return fmt.Errorf("-secret is required: it is the AUTH_SECRET the server was started with")
	}

	tokens, err := auth.NewTokens(opts.secret)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ctx, cancel := context.WithTimeout(ctx, opts.duration)
	defer cancel()

	// Tokens are minted rather than earned through /auth/login, so that the
	// measurement is about seats rather than about Argon2.
	token := func(userID string) (string, error) {
		return tokens.Issue(userID, time.Now().Add(time.Hour))
	}

	results := &results{}

	if err := joinWaiters(ctx, opts, token, results); err != nil {
		return err
	}

	var wg sync.WaitGroup

	started := time.Now()

	for i := range opts.users {
		bearer, err := token(fmt.Sprintf("load-user-%d", i))
		if err != nil {
			return err
		}

		wg.Add(1)

		go func() {
			defer wg.Done()

			churn(ctx, opts, bearer, results)
		}()
	}

	wg.Wait()
	results.report(time.Since(started))

	return nil
}

// joinWaiters puts a few callers in the queue before the churn starts, so that
// every freed seat has somebody to be offered to.
func joinWaiters(ctx context.Context, opts options, token func(string) (string, error), r *results) error {
	for i := range opts.waiters {
		bearer, err := token(fmt.Sprintf("load-waiter-%d", i))
		if err != nil {
			return err
		}

		status, _ := request(ctx, http.MethodPost,
			opts.url+"/events/"+opts.event+"/waiting-list", bearer, r)

		if status != http.StatusOK {
			return fmt.Errorf("a waiter could not join the queue: got %d", status)
		}
	}

	return nil
}

// churn holds a seat and immediately gives it back, over and over. Releasing is
// what puts a freed seat in front of the waiting list, so this is the loop that
// decides whether the handoff keeps up.
func churn(ctx context.Context, opts options, bearer string, r *results) {
	for ctx.Err() == nil {
		seat := seatID(rand.IntN(opts.seats))

		status, body := request(ctx, http.MethodPost,
			fmt.Sprintf("%s/events/%s/seats/%s/hold", opts.url, opts.event, seat), bearer, r)

		if status != http.StatusCreated {
			continue
		}

		var held struct {
			HoldID string `json:"holdId"`
		}
		if json.Unmarshal(body, &held) != nil || held.HoldID == "" {
			continue
		}

		request(ctx, http.MethodDelete, opts.url+"/holds/"+held.HoldID, bearer, r)
	}
}

// seatID walks the demo seat map, which is rows of five.
func seatID(n int) string {
	return fmt.Sprintf("%c%d", 'A'+n/5, n%5+1)
}

func request(ctx context.Context, method, url, bearer string, r *results) (int, []byte) {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return 0, nil
	}

	req.Header.Set("Authorization", "Bearer "+bearer)

	started := time.Now()

	response, err := http.DefaultClient.Do(req)
	if err != nil {
		r.record(0, time.Since(started))

		return 0, nil
	}
	defer func() { _ = response.Body.Close() }()

	body := make([]byte, 0, 512)
	buf := make([]byte, 512)

	for {
		n, err := response.Body.Read(buf)
		body = append(body, buf[:n]...)

		if err != nil {
			break
		}
	}

	r.record(response.StatusCode, time.Since(started))

	return response.StatusCode, body
}

type results struct {
	mu        sync.Mutex
	latencies []time.Duration
	byStatus  map[int]int
	total     atomic.Int64
}

func (r *results) record(status int, took time.Duration) {
	r.total.Add(1)

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.byStatus == nil {
		r.byStatus = make(map[int]int)
	}

	r.byStatus[status]++
	r.latencies = append(r.latencies, took)
}

func (r *results) report(elapsed time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	sort.Slice(r.latencies, func(i, j int) bool { return r.latencies[i] < r.latencies[j] })

	fmt.Printf("\n%d requests in %s, %.0f/s\n",
		len(r.latencies), elapsed.Round(time.Millisecond),
		float64(len(r.latencies))/elapsed.Seconds())

	statuses := make([]int, 0, len(r.byStatus))
	for status := range r.byStatus {
		statuses = append(statuses, status)
	}

	sort.Ints(statuses)

	fmt.Println("\nby status")
	for _, status := range statuses {
		fmt.Printf("  %3d  %6d  %5.1f%%\n", status, r.byStatus[status],
			float64(r.byStatus[status])/float64(len(r.latencies))*100)
	}

	fmt.Println("\nlatency")
	for _, p := range []struct {
		name string
		at   float64
	}{{"p50", 0.50}, {"p90", 0.90}, {"p99", 0.99}, {"max", 1}} {
		fmt.Printf("  %s  %s\n", p.name, r.percentile(p.at).Round(time.Microsecond))
	}
}

func (r *results) percentile(at float64) time.Duration {
	if len(r.latencies) == 0 {
		return 0
	}

	index := int(float64(len(r.latencies)-1) * at)

	return r.latencies[index]
}
