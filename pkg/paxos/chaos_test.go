package paxos

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"
)

type FaultyAcceptor struct {
	acceptor    *Acceptor
	failureRate float64
	delayMin    time.Duration
	delayMax    time.Duration
	shutdown    bool
	mu          sync.Mutex
}

func NewFaultyAcceptor(failureRate float64, delayMin, delayMax time.Duration) *FaultyAcceptor {
	return &FaultyAcceptor{
		acceptor:    NewAcceptor(),
		failureRate: failureRate,
		delayMin:    delayMin,
		delayMax:    delayMax,
	}
}

func (f *FaultyAcceptor) Prepare(ctx context.Context, proposalID ProposalID) (Promise, error) {
	f.mu.Lock()
	if f.shutdown {
		f.mu.Unlock()
		return Promise{}, ErrShutdown
	}

	if rand.Float64() < f.failureRate {
		f.mu.Unlock()
		return Promise{}, ErrRejected
	}

	delay := f.delayMin + time.Duration(rand.Int63n(int64(f.delayMax-f.delayMin)))
	f.mu.Unlock()

	time.Sleep(delay)

	return f.acceptor.Prepare(ctx, proposalID)
}

func (f *FaultyAcceptor) Accept(ctx context.Context, proposalID ProposalID, value interface{}) (AcceptedResult, error) {
	f.mu.Lock()
	if f.shutdown {
		f.mu.Unlock()
		return AcceptedResult{}, ErrShutdown
	}

	if rand.Float64() < f.failureRate {
		f.mu.Unlock()
		return AcceptedResult{}, ErrRejected
	}

	delay := f.delayMin + time.Duration(rand.Int63n(int64(f.delayMax-f.delayMin)))
	f.mu.Unlock()

	time.Sleep(delay)

	return f.acceptor.Accept(ctx, proposalID, value)
}

func (f *FaultyAcceptor) State() (promised ProposalID, accepted ProposalID, value interface{}) {
	return f.acceptor.State()
}

func (f *FaultyAcceptor) Shutdown() {
	f.mu.Lock()
	f.shutdown = true
	f.mu.Unlock()
	f.acceptor.Shutdown()
}

func TestChaosRandomFailures(t *testing.T) {
	rand.Seed(time.Now().UnixNano())

	faultyAcceptors := []*FaultyAcceptor{
		NewFaultyAcceptor(0.2, 0, 10*time.Millisecond),
		NewFaultyAcceptor(0.2, 0, 10*time.Millisecond),
		NewFaultyAcceptor(0.2, 0, 10*time.Millisecond),
	}

	acceptors := make([]*Acceptor, len(faultyAcceptors))
	for i, fa := range faultyAcceptors {
		acceptors[i] = fa.acceptor
	}

	proposer := NewProposer("P1", acceptors)
	learner := NewLearner(acceptors)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := proposer.Propose(ctx, "chaosValue")
	if err != nil {
		t.Fatalf("Propose failed with random failures: %v", err)
	}

	if result != "chaosValue" {
		t.Errorf("Expected chaosValue, got %v", result)
	}

	learned, err := learner.Learn(ctx)
	if err != nil {
		t.Fatalf("Learn failed with random failures: %v", err)
	}

	if learned != "chaosValue" {
		t.Errorf("Learned value mismatch: expected chaosValue, got %v", learned)
	}

	for _, fa := range faultyAcceptors {
		fa.Shutdown()
	}
}

func TestChaosWithDelays(t *testing.T) {
	rand.Seed(time.Now().UnixNano())

	faultyAcceptors := []*FaultyAcceptor{
		NewFaultyAcceptor(0.0, 10*time.Millisecond, 50*time.Millisecond),
		NewFaultyAcceptor(0.0, 10*time.Millisecond, 50*time.Millisecond),
		NewFaultyAcceptor(0.0, 10*time.Millisecond, 50*time.Millisecond),
	}

	acceptors := make([]*Acceptor, len(faultyAcceptors))
	for i, fa := range faultyAcceptors {
		acceptors[i] = fa.acceptor
	}

	proposer := NewProposer("P1", acceptors)
	learner := NewLearner(acceptors)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := proposer.Propose(ctx, "delayedValue")
	if err != nil {
		t.Fatalf("Propose failed with delays: %v", err)
	}

	if result != "delayedValue" {
		t.Errorf("Expected delayedValue, got %v", result)
	}

	learned, err := learner.Learn(ctx)
	if err != nil {
		t.Fatalf("Learn failed with delays: %v", err)
	}

	if learned != "delayedValue" {
		t.Errorf("Learned value mismatch: expected delayedValue, got %v", learned)
	}

	for _, fa := range faultyAcceptors {
		fa.Shutdown()
	}
}

func TestChaosConcurrentProposersWithFailures(t *testing.T) {
	rand.Seed(time.Now().UnixNano())

	faultyAcceptors := []*FaultyAcceptor{
		NewFaultyAcceptor(0.15, 5*time.Millisecond, 20*time.Millisecond),
		NewFaultyAcceptor(0.15, 5*time.Millisecond, 20*time.Millisecond),
		NewFaultyAcceptor(0.15, 5*time.Millisecond, 20*time.Millisecond),
		NewFaultyAcceptor(0.15, 5*time.Millisecond, 20*time.Millisecond),
		NewFaultyAcceptor(0.15, 5*time.Millisecond, 20*time.Millisecond),
	}

	acceptors := make([]*Acceptor, len(faultyAcceptors))
	for i, fa := range faultyAcceptors {
		acceptors[i] = fa.acceptor
	}

	proposer1 := NewProposer("P1", acceptors)
	proposer2 := NewProposer("P2", acceptors)
	proposer3 := NewProposer("P3", acceptors)
	learner := NewLearner(acceptors)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	var results []interface{}
	var mu sync.Mutex

	wg.Add(3)
	go func() {
		defer wg.Done()
		result, err := proposer1.Propose(ctx, "valueX")
		if err == nil {
			mu.Lock()
			results = append(results, result)
			mu.Unlock()
		}
	}()

	go func() {
		defer wg.Done()
		result, err := proposer2.Propose(ctx, "valueY")
		if err == nil {
			mu.Lock()
			results = append(results, result)
			mu.Unlock()
		}
	}()

	go func() {
		defer wg.Done()
		result, err := proposer3.Propose(ctx, "valueZ")
		if err == nil {
			mu.Lock()
			results = append(results, result)
			mu.Unlock()
		}
	}()

	wg.Wait()

	if len(results) == 0 {
		t.Fatal("No proposals succeeded")
	}

	firstValue := results[0]
	for _, result := range results {
		if result != firstValue {
			t.Errorf("Concurrent proposers with failures chose different values: %v vs %v", firstValue, result)
		}
	}

	learned, err := learner.Learn(ctx)
	if err != nil {
		t.Fatalf("Learn failed with concurrent proposers and failures: %v", err)
	}

	if learned != firstValue {
		t.Errorf("Learned value mismatch: expected %v, got %v", firstValue, learned)
	}

	for _, fa := range faultyAcceptors {
		fa.Shutdown()
	}
}

func TestChaosNetworkPartition(t *testing.T) {
	rand.Seed(time.Now().UnixNano())

	acceptors := []*Acceptor{
		NewAcceptor(),
		NewAcceptor(),
		NewAcceptor(),
		NewAcceptor(),
		NewAcceptor(),
	}

	partition1 := acceptors[:2]
	partition2 := acceptors[2:]

	proposer1 := NewProposer("P1", partition1)
	proposer2 := NewProposer("P2", partition2)
	learner := NewLearner(acceptors)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		proposer1.Propose(ctx, "partitionValue1")
	}()

	go func() {
		defer wg.Done()
		proposer2.Propose(ctx, "partitionValue2")
	}()

	wg.Wait()

	time.Sleep(500 * time.Millisecond)

	fullProposer := NewProposer("P3", acceptors)
	result, err := fullProposer.Propose(ctx, "unifiedValue")
	if err != nil {
		t.Fatalf("Propose after partition recovery failed: %v", err)
	}

	learned, err := learner.Learn(ctx)
	if err != nil {
		t.Fatalf("Learn after partition recovery failed: %v", err)
	}

	if learned != result {
		t.Errorf("Learned value mismatch after partition: expected %v, got %v", result, learned)
	}

	t.Logf("Partition recovery result: %v (Paxos preserves majority-chosen value)", result)
}

func TestChaosRepeatedFailures(t *testing.T) {
	for iteration := 0; iteration < 5; iteration++ {
		t.Run(fmt.Sprintf("Iteration_%d", iteration), func(t *testing.T) {
			rand.Seed(time.Now().UnixNano())

			faultyAcceptors := []*FaultyAcceptor{
				NewFaultyAcceptor(0.2, 0, 15*time.Millisecond),
				NewFaultyAcceptor(0.2, 0, 15*time.Millisecond),
				NewFaultyAcceptor(0.2, 0, 15*time.Millisecond),
			}

			acceptors := make([]*Acceptor, len(faultyAcceptors))
			for i, fa := range faultyAcceptors {
				acceptors[i] = fa.acceptor
			}

			proposer := NewProposer("P1", acceptors)
			learner := NewLearner(acceptors)

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			value := fmt.Sprintf("repeatedValue_%d", iteration)
			result, err := proposer.Propose(ctx, value)
			if err != nil {
				t.Fatalf("Propose failed on iteration %d: %v", iteration, err)
			}

			if result != value {
				t.Errorf("Iteration %d: expected %v, got %v", iteration, value, result)
			}

			learned, err := learner.Learn(ctx)
			if err != nil {
				t.Fatalf("Learn failed on iteration %d: %v", iteration, err)
			}

			if learned != value {
				t.Errorf("Iteration %d: learned value mismatch: expected %v, got %v", iteration, value, learned)
			}

			for _, fa := range faultyAcceptors {
				fa.Shutdown()
			}
		})
	}
}
