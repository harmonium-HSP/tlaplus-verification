package paxos

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrRejected      = errors.New("proposal rejected")
	ErrTimeout       = errors.New("operation timeout")
	ErrNoQuorum      = errors.New("no quorum reached")
	ErrShutdown      = errors.New("node shutdown")
	ErrValueLearned  = errors.New("value already learned")
)

type ProposalID struct {
	Round    int64
	Proposer string
}

func (p ProposalID) GreaterThan(other ProposalID) bool {
	if p.Round > other.Round {
		return true
	}
	if p.Round == other.Round && p.Proposer > other.Proposer {
		return true
	}
	return false
}

type Promise struct {
	ProposalID ProposalID
	AcceptedID ProposalID
	Value      interface{}
	Accepted   bool
}

type AcceptedResult struct {
	ProposalID ProposalID
	Accepted   bool
}

type LearnerState struct {
	ChosenValue interface{}
	Chosen      bool
	Accepted    map[ProposalID]int
}

type Acceptor struct {
	mu           sync.RWMutex
	promisedID   ProposalID
	acceptedID   ProposalID
	acceptedVal  interface{}
	shutdown     bool
}

func NewAcceptor() *Acceptor {
	return &Acceptor{
		promisedID:  ProposalID{},
		acceptedID:  ProposalID{},
		acceptedVal: nil,
	}
}

func (a *Acceptor) Prepare(ctx context.Context, proposalID ProposalID) (Promise, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.shutdown {
		return Promise{}, ErrShutdown
	}

	select {
	case <-ctx.Done():
		return Promise{}, ctx.Err()
	default:
	}

	if proposalID.GreaterThan(a.promisedID) {
		a.promisedID = proposalID
		return Promise{
			ProposalID: proposalID,
			AcceptedID: a.acceptedID,
			Value:      a.acceptedVal,
			Accepted:   true,
		}, nil
	}

	return Promise{
		ProposalID: proposalID,
		Accepted:   false,
	}, ErrRejected
}

func (a *Acceptor) Accept(ctx context.Context, proposalID ProposalID, value interface{}) (AcceptedResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.shutdown {
		return AcceptedResult{}, ErrShutdown
	}

	select {
	case <-ctx.Done():
		return AcceptedResult{}, ctx.Err()
	default:
	}

	if proposalID.GreaterThan(a.promisedID) || proposalID == a.promisedID {
		a.promisedID = proposalID
		a.acceptedID = proposalID
		a.acceptedVal = value
		return AcceptedResult{
			ProposalID: proposalID,
			Accepted:   true,
		}, nil
	}

	return AcceptedResult{
		ProposalID: proposalID,
		Accepted:   false,
	}, ErrRejected
}

func (a *Acceptor) State() (promised ProposalID, accepted ProposalID, value interface{}) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.promisedID, a.acceptedID, a.acceptedVal
}

func (a *Acceptor) Shutdown() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.shutdown = true
}

type Proposer struct {
	proposerID   string
	acceptors    []*Acceptor
	quorum       int
	round        int64
	shutdown     bool
	mu           sync.Mutex
}

func NewProposer(proposerID string, acceptors []*Acceptor) *Proposer {
	return &Proposer{
		proposerID: proposerID,
		acceptors:  acceptors,
		quorum:     len(acceptors)/2 + 1,
		round:      0,
	}
}

func (p *Proposer) propose(ctx context.Context, value interface{}) (interface{}, error) {
	p.mu.Lock()
	if p.shutdown {
		p.mu.Unlock()
		return nil, ErrShutdown
	}
	p.round++
	proposalID := ProposalID{Round: p.round, Proposer: p.proposerID}
	p.mu.Unlock()

	phase1Ctx, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()

	promises := make(chan Promise, len(p.acceptors))
	var wg sync.WaitGroup

	for _, acceptor := range p.acceptors {
		wg.Add(1)
		go func(a *Acceptor) {
			defer wg.Done()
			promise, err := a.Prepare(phase1Ctx, proposalID)
			if err == nil || errors.Is(err, ErrRejected) {
				promises <- promise
			}
		}(acceptor)
	}

	go func() {
		wg.Wait()
		close(promises)
	}()

	var promiseCount int
	var maxAcceptedID ProposalID
	var chosenValue interface{} = value

	for promise := range promises {
		if promise.Accepted {
			promiseCount++
			if promise.AcceptedID.GreaterThan(maxAcceptedID) {
				maxAcceptedID = promise.AcceptedID
				if promise.Value != nil {
					chosenValue = promise.Value
				}
			}
		}
	}

	if promiseCount < p.quorum {
		return nil, ErrNoQuorum
	}

	phase2Ctx, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()

	accepted := make(chan AcceptedResult, len(p.acceptors))

	for _, acceptor := range p.acceptors {
		wg.Add(1)
		go func(a *Acceptor) {
			defer wg.Done()
			result, err := a.Accept(phase2Ctx, proposalID, chosenValue)
			if err == nil || errors.Is(err, ErrRejected) {
				accepted <- result
			}
		}(acceptor)
	}

	go func() {
		wg.Wait()
		close(accepted)
	}()

	var acceptCount int
	for result := range accepted {
		if result.Accepted {
			acceptCount++
		}
	}

	if acceptCount < p.quorum {
		return nil, ErrNoQuorum
	}

	return chosenValue, nil
}

func (p *Proposer) Propose(ctx context.Context, value interface{}) (interface{}, error) {
	for {
		result, err := p.propose(ctx, value)
		if err == nil {
			return result, nil
		}
		if errors.Is(err, ErrShutdown) || errors.Is(err, ctx.Err()) {
			return nil, err
		}
	}
}

func (p *Proposer) Shutdown() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.shutdown = true
}

type Learner struct {
	mu           sync.RWMutex
	acceptors    []*Acceptor
	quorum       int
	chosen       bool
	chosenValue  interface{}
	shutdown     bool
	listeners    []chan interface{}
}

func NewLearner(acceptors []*Acceptor) *Learner {
	return &Learner{
		acceptors:   acceptors,
		quorum:      len(acceptors)/2 + 1,
		chosen:      false,
		chosenValue: nil,
		listeners:   make([]chan interface{}, 0),
	}
}

func (l *Learner) Learn(ctx context.Context) (interface{}, error) {
	l.mu.RLock()
	if l.chosen {
		val := l.chosenValue
		l.mu.RUnlock()
		return val, nil
	}
	l.mu.RUnlock()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			l.mu.RLock()
			if l.chosen {
				val := l.chosenValue
				l.mu.RUnlock()
				return val, nil
			}
			l.mu.RUnlock()

			results := make(map[interface{}]int)
			l.mu.Lock()
			for _, acceptor := range l.acceptors {
				_, acceptedID, acceptedVal := acceptor.State()
				if acceptedID.Round > 0 && acceptedVal != nil {
					results[acceptedVal]++
				}
			}

			for val, count := range results {
				if count >= l.quorum {
					l.chosen = true
					l.chosenValue = val
					
					for _, listener := range l.listeners {
						select {
						case listener <- val:
						default:
						}
					}
					
					l.mu.Unlock()
					return val, nil
				}
			}
			l.mu.Unlock()
		}
	}
}

func (l *Learner) Subscribe() <-chan interface{} {
	l.mu.Lock()
	defer l.mu.Unlock()
	
	ch := make(chan interface{}, 1)
	if l.chosen {
		ch <- l.chosenValue
	}
	l.listeners = append(l.listeners, ch)
	return ch
}

func (l *Learner) Shutdown() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.shutdown = true
	for _, listener := range l.listeners {
		close(listener)
	}
}

type PaxosNode struct {
	proposer *Proposer
	acceptor *Acceptor
	learner  *Learner
	nodeID   string
}

func NewPaxosNode(nodeID string, acceptors []*Acceptor) *PaxosNode {
	selfAcceptor := NewAcceptor()
	
	allAcceptors := append([]*Acceptor{}, acceptors...)
	allAcceptors = append(allAcceptors, selfAcceptor)
	
	return &PaxosNode{
		proposer: NewProposer(nodeID, allAcceptors),
		acceptor: selfAcceptor,
		learner:  NewLearner(allAcceptors),
		nodeID:   nodeID,
	}
}

func (n *PaxosNode) Propose(ctx context.Context, value interface{}) (interface{}, error) {
	return n.proposer.Propose(ctx, value)
}

func (n *PaxosNode) Learn(ctx context.Context) (interface{}, error) {
	return n.learner.Learn(ctx)
}

func (n *PaxosNode) Subscribe() <-chan interface{} {
	return n.learner.Subscribe()
}

func (n *PaxosNode) Shutdown() {
	n.proposer.Shutdown()
	n.acceptor.Shutdown()
	n.learner.Shutdown()
}

func (n *PaxosNode) String() string {
	return fmt.Sprintf("PaxosNode(%s)", n.nodeID)
}
