package raft

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"
)

type Role string

const (
	Follower  Role = "Follower"
	Candidate Role = "Candidate"
	Leader    Role = "Leader"
)

type NodeID string

type VoteRequest struct {
	Term         uint64
	CandidateID  NodeID
	LastLogIndex uint64
	LastLogTerm  uint64
}

type VoteResponse struct {
	Term    uint64
	Granted bool
}

type AppendEntriesRequest struct {
	Term         uint64
	LeaderID     NodeID
	PrevLogIndex uint64
	PrevLogTerm  uint64
	Entries      []LogEntry
	LeaderCommit uint64
}

type AppendEntriesResponse struct {
	Term    uint64
	Success bool
}

type LogEntry struct {
	Index   uint64
	Term    uint64
	Command []byte
}

type Node struct {
	mu                sync.Mutex
	id                NodeID
	peers             []Peer
	currentTerm       uint64
	votedFor          *NodeID
	role              Role
	log               []LogEntry
	commitIndex       uint64
	lastApplied       uint64
	electionTimeout   time.Duration
	heartbeatInterval time.Duration
	electionTimer     *time.Timer
	heartbeatTimers   []*time.Timer
	stopChan          chan struct{}
	wg                sync.WaitGroup
}

type Peer interface {
	RequestVote(ctx context.Context, req *VoteRequest) (*VoteResponse, error)
	AppendEntries(ctx context.Context, req *AppendEntriesRequest) (*AppendEntriesResponse, error)
}

func NewNode(id NodeID, peers []Peer) *Node {
	return &Node{
		id:                id,
		peers:             peers,
		currentTerm:       0,
		votedFor:          nil,
		role:              Follower,
		log:               []LogEntry{{Index: 0, Term: 0}},
		commitIndex:       0,
		lastApplied:       0,
		electionTimeout:   randomElectionTimeout(),
		heartbeatInterval: 100 * time.Millisecond,
		stopChan:          make(chan struct{}),
	}
}

func randomElectionTimeout() time.Duration {
	return time.Duration(150+rand.Intn(150)) * time.Millisecond
}

func (n *Node) Start() {
	n.wg.Add(1)
	go n.run()
}

func (n *Node) Stop() {
	close(n.stopChan)
	n.wg.Wait()
}

func (n *Node) run() {
	defer n.wg.Done()

	for {
		select {
		case <-n.stopChan:
			return
		default:
			n.mu.Lock()
			role := n.role
			n.mu.Unlock()

			switch role {
			case Follower:
				n.runFollower()
			case Candidate:
				n.runCandidate()
			case Leader:
				n.runLeader()
			}
		}
	}
}

func (n *Node) runFollower() {
	n.mu.Lock()
	n.resetElectionTimer()
	timer := n.electionTimer
	n.mu.Unlock()

	select {
	case <-timer.C:
		n.mu.Lock()
		if n.role == Follower {
			n.becomeCandidate()
		}
		n.mu.Unlock()
	case <-n.stopChan:
		return
	}
}

func (n *Node) runCandidate() {
	n.mu.Lock()
	n.currentTerm++
	n.votedFor = &n.id
	n.resetElectionTimer()
	timer := n.electionTimer
	term := n.currentTerm
	n.mu.Unlock()

	votes := 1
	voteChan := make(chan bool, len(n.peers))

	for _, peer := range n.peers {
		go func(p Peer) {
			req := &VoteRequest{
				Term:        term,
				CandidateID: n.id,
			}
			resp, err := p.RequestVote(context.Background(), req)
			if err != nil {
				voteChan <- false
				return
			}

			n.mu.Lock()
			if resp.Term > n.currentTerm {
				n.becomeFollower(resp.Term)
				n.mu.Unlock()
				voteChan <- false
				return
			}
			n.mu.Unlock()

			voteChan <- resp.Granted
		}(peer)
	}

	for i := 0; i < len(n.peers); i++ {
		select {
		case granted := <-voteChan:
			if granted {
				votes++
				if votes > len(n.peers)/2 {
					n.mu.Lock()
					n.becomeLeader()
					n.mu.Unlock()
					return
				}
			}
		case <-timer.C:
			n.mu.Lock()
			if n.role == Candidate {
				n.becomeCandidate()
			}
			n.mu.Unlock()
			return
		case <-n.stopChan:
			return
		}
	}
}

func (n *Node) runLeader() {
	n.startHeartbeats()

	<-n.stopChan
	n.stopHeartbeats()
}

func (n *Node) becomeCandidate() {
	n.role = Candidate
	n.votedFor = &n.id
	n.resetElectionTimer()
}

func (n *Node) becomeFollower(term uint64) {
	n.role = Follower
	n.currentTerm = term
	n.votedFor = nil
	n.resetElectionTimer()
}

func (n *Node) becomeLeader() {
	n.role = Leader
	n.resetElectionTimer()
	n.startHeartbeats()
}

func (n *Node) resetElectionTimer() {
	if n.electionTimer != nil {
		n.electionTimer.Stop()
	}
	n.electionTimeout = randomElectionTimeout()
	n.electionTimer = time.NewTimer(n.electionTimeout)
}

func (n *Node) startHeartbeats() {
	n.stopHeartbeats()
	for _, peer := range n.peers {
		go n.sendHeartbeat(peer)
	}
}

func (n *Node) stopHeartbeats() {
	for _, t := range n.heartbeatTimers {
		if t != nil {
			t.Stop()
		}
	}
	n.heartbeatTimers = nil
}

func (n *Node) sendHeartbeat(peer Peer) {
	ticker := time.NewTicker(n.heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			n.mu.Lock()
			if n.role != Leader {
				n.mu.Unlock()
				return
			}
			req := &AppendEntriesRequest{
				Term:     n.currentTerm,
				LeaderID: n.id,
			}
			n.mu.Unlock()

			resp, err := peer.AppendEntries(context.Background(), req)
			if err != nil {
				continue
			}

			n.mu.Lock()
			if resp.Term > n.currentTerm {
				n.becomeFollower(resp.Term)
				n.mu.Unlock()
				return
			}
			n.mu.Unlock()
		case <-n.stopChan:
			return
		}
	}
}

func (n *Node) RequestVote(ctx context.Context, req *VoteRequest) (*VoteResponse, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	resp := &VoteResponse{Term: n.currentTerm, Granted: false}

	if req.Term < n.currentTerm {
		return resp, nil
	}

	if req.Term > n.currentTerm {
		n.becomeFollower(req.Term)
	}

	if n.votedFor == nil || *n.votedFor == req.CandidateID {
		n.votedFor = &req.CandidateID
		resp.Granted = true
		n.resetElectionTimer()
	}

	return resp, nil
}

func (n *Node) AppendEntries(ctx context.Context, req *AppendEntriesRequest) (*AppendEntriesResponse, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	resp := &AppendEntriesResponse{Term: n.currentTerm, Success: false}

	if req.Term < n.currentTerm {
		return resp, nil
	}

	if req.Term > n.currentTerm {
		n.becomeFollower(req.Term)
	}

	n.resetElectionTimer()
	resp.Success = true

	return resp, nil
}

func (n *Node) GetRole() Role {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.role
}

func (n *Node) GetTerm() uint64 {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.currentTerm
}

func (n *Node) String() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return fmt.Sprintf("Node(%s, %s, Term=%d)", n.id, n.role, n.currentTerm)
}
