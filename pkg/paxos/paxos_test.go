package paxos

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestBasicPaxos(t *testing.T) {
	acceptors := []*Acceptor{
		NewAcceptor(),
		NewAcceptor(),
		NewAcceptor(),
	}

	proposer := NewProposer("P1", acceptors)
	learner := NewLearner(acceptors)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := proposer.Propose(ctx, "value1")
	if err != nil {
		t.Fatalf("Propose failed: %v", err)
	}

	if result != "value1" {
		t.Errorf("Expected value1, got %v", result)
	}

	learned, err := learner.Learn(ctx)
	if err != nil {
		t.Fatalf("Learn failed: %v", err)
	}

	if learned != "value1" {
		t.Errorf("Learned value mismatch: expected value1, got %v", learned)
	}
}

func TestConcurrentProposers(t *testing.T) {
	acceptors := []*Acceptor{
		NewAcceptor(),
		NewAcceptor(),
		NewAcceptor(),
	}

	proposer1 := NewProposer("P1", acceptors)
	proposer2 := NewProposer("P2", acceptors)
	learner := NewLearner(acceptors)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	var results []interface{}
	var mu sync.Mutex

	wg.Add(2)
	go func() {
		defer wg.Done()
		result, err := proposer1.Propose(ctx, "valueA")
		if err == nil {
			mu.Lock()
			results = append(results, result)
			mu.Unlock()
		}
	}()

	go func() {
		defer wg.Done()
		result, err := proposer2.Propose(ctx, "valueB")
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
			t.Errorf("Concurrent proposers chose different values: %v vs %v", firstValue, result)
		}
	}

	learned, err := learner.Learn(ctx)
	if err != nil {
		t.Fatalf("Learn failed: %v", err)
	}

	if learned != firstValue {
		t.Errorf("Learned value mismatch: expected %v, got %v", firstValue, learned)
	}
}

func TestLearner(t *testing.T) {
	acceptors := []*Acceptor{
		NewAcceptor(),
		NewAcceptor(),
		NewAcceptor(),
	}

	proposer := NewProposer("P1", acceptors)
	learner := NewLearner(acceptors)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go func() {
		time.Sleep(100 * time.Millisecond)
		proposer.Propose(ctx, "learnedValue")
	}()

	learned, err := learner.Learn(ctx)
	if err != nil {
		t.Fatalf("Learn failed: %v", err)
	}

	if learned != "learnedValue" {
		t.Errorf("Expected learnedValue, got %v", learned)
	}
}

func TestPaxosNode(t *testing.T) {
	node1 := NewPaxosNode("node1", []*Acceptor{})

	node2Acceptors := []*Acceptor{node1.acceptor}
	node2 := NewPaxosNode("node2", node2Acceptors)

	node3Acceptors := []*Acceptor{node1.acceptor, node2.acceptor}
	node3 := NewPaxosNode("node3", node3Acceptors)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := node1.Propose(ctx, "nodeValue")
	if err != nil {
		t.Fatalf("Node1 propose failed: %v", err)
	}

	if result != "nodeValue" {
		t.Errorf("Expected nodeValue, got %v", result)
	}

	learned2, err := node2.Learn(ctx)
	if err != nil {
		t.Fatalf("Node2 learn failed: %v", err)
	}
	if learned2 != "nodeValue" {
		t.Errorf("Node2 learned wrong value: %v", learned2)
	}

	learned3, err := node3.Learn(ctx)
	if err != nil {
		t.Fatalf("Node3 learn failed: %v", err)
	}
	if learned3 != "nodeValue" {
		t.Errorf("Node3 learned wrong value: %v", learned3)
	}
}

func TestMajorityTolerance(t *testing.T) {
	acceptors := []*Acceptor{
		NewAcceptor(),
		NewAcceptor(),
		NewAcceptor(),
		NewAcceptor(),
		NewAcceptor(),
	}

	proposer := NewProposer("P1", acceptors)
	learner := NewLearner(acceptors)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := proposer.Propose(ctx, "majorityValue")
	if err != nil {
		t.Fatalf("Propose failed: %v", err)
	}

	if result != "majorityValue" {
		t.Errorf("Expected majorityValue, got %v", result)
	}

	learned, err := learner.Learn(ctx)
	if err != nil {
		t.Fatalf("Learn failed: %v", err)
	}

	if learned != "majorityValue" {
		t.Errorf("Learned value mismatch: expected majorityValue, got %v", learned)
	}
}

func TestAcceptorPersistence(t *testing.T) {
	acceptor := NewAcceptor()

	ctx := context.Background()

	proposal1 := ProposalID{Round: 1, Proposer: "P1"}
	promise1, err := acceptor.Prepare(ctx, proposal1)
	if err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}
	if !promise1.Accepted {
		t.Error("First prepare should be accepted")
	}

	acceptor.Accept(ctx, proposal1, "persistedValue")

	promise2, err := acceptor.Prepare(ctx, proposal1)
	if err != nil {
		t.Fatalf("Second prepare failed: %v", err)
	}
	if !promise2.Accepted {
		t.Error("Prepare with same ID should be accepted")
	}
	if promise2.Value != "persistedValue" {
		t.Errorf("Expected persistedValue, got %v", promise2.Value)
	}

	proposal2 := ProposalID{Round: 2, Proposer: "P2"}
	promise3, err := acceptor.Prepare(ctx, proposal2)
	if err != nil && err != ErrRejected {
		t.Fatalf("Prepare with higher ID failed: %v", err)
	}
	if promise3.Accepted {
		if promise3.Value != "persistedValue" {
			t.Errorf("Higher round should see persisted value: got %v", promise3.Value)
		}
	}
}

func TestProposalIDComparison(t *testing.T) {
	p1 := ProposalID{Round: 1, Proposer: "A"}
	p2 := ProposalID{Round: 2, Proposer: "A"}
	p3 := ProposalID{Round: 1, Proposer: "B"}
	p4 := ProposalID{Round: 1, Proposer: "A"}

	if !p2.GreaterThan(p1) {
		t.Error("Higher round should be greater")
	}
	if !p3.GreaterThan(p1) {
		t.Error("Same round, higher proposer ID should be greater")
	}
	if p1.GreaterThan(p4) {
		t.Error("Equal IDs should not be greater")
	}
}

func TestLearnerSubscription(t *testing.T) {
	acceptors := []*Acceptor{
		NewAcceptor(),
		NewAcceptor(),
		NewAcceptor(),
	}

	proposer := NewProposer("P1", acceptors)
	learner := NewLearner(acceptors)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	subscription := learner.Subscribe()

	go func() {
		time.Sleep(100 * time.Millisecond)
		proposer.Propose(ctx, "subscribedValue")
	}()

	select {
	case val := <-subscription:
		if val != "subscribedValue" {
			t.Errorf("Expected subscribedValue, got %v", val)
		}
	case <-time.After(5 * time.Second):
		t.Error("Subscription timeout")
	}
}
