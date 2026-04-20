package raft

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

type MockPeer struct {
	mu        sync.Mutex
	node      *Node
	partition bool
	delay     time.Duration
	failRate  float64
}

func NewMockPeer(node *Node) *MockPeer {
	return &MockPeer{
		node:      node,
		partition: false,
		delay:     0,
		failRate:  0,
	}
}

func (p *MockPeer) SetPartitioned(partitioned bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.partition = partitioned
}

func (p *MockPeer) SetDelay(delay time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.delay = delay
}

func (p *MockPeer) SetFailRate(rate float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.failRate = rate
}

func (p *MockPeer) RequestVote(ctx context.Context, req *VoteRequest) (*VoteResponse, error) {
	p.mu.Lock()
	partition := p.partition
	delay := p.delay
	p.mu.Unlock()

	if partition {
		return nil, errors.New("network partition")
	}
	if delay > 0 {
		time.Sleep(delay)
	}

	return p.node.RequestVote(ctx, req)
}

func (p *MockPeer) AppendEntries(ctx context.Context, req *AppendEntriesRequest) (*AppendEntriesResponse, error) {
	p.mu.Lock()
	partition := p.partition
	delay := p.delay
	p.mu.Unlock()

	if partition {
		return nil, errors.New("network partition")
	}
	if delay > 0 {
		time.Sleep(delay)
	}

	return p.node.AppendEntries(ctx, req)
}

func TestRaftElectionSafety(t *testing.T) {
	nodes := make([]*Node, 3)
	peers := make([][]Peer, 3)

	for i := 0; i < 3; i++ {
		nodes[i] = NewNode(NodeID(fmt.Sprintf("N%d", i+1)), nil)
	}

	for i := 0; i < 3; i++ {
		peers[i] = make([]Peer, 2)
		idx := 0
		for j := 0; j < 3; j++ {
			if j != i {
				peers[i][idx] = NewMockPeer(nodes[j])
				idx++
			}
		}
		nodes[i].peers = peers[i]
	}

	for _, node := range nodes {
		node.Start()
		defer node.Stop()
	}

	time.Sleep(500 * time.Millisecond)

	leaders := 0
	for _, node := range nodes {
		if node.GetRole() == Leader {
			leaders++
		}
	}

	if leaders != 1 {
		t.Errorf("Expected exactly 1 leader, got %d", leaders)
	}

	for _, node := range nodes {
		t.Logf("%s", node.String())
	}
}

func TestRaftNetworkPartition(t *testing.T) {
	nodes := make([]*Node, 3)
	peers := make([][]Peer, 3)
	mockPeers := make([][]*MockPeer, 3)

	for i := 0; i < 3; i++ {
		nodes[i] = NewNode(NodeID(fmt.Sprintf("N%d", i+1)), nil)
	}

	for i := 0; i < 3; i++ {
		peers[i] = make([]Peer, 2)
		mockPeers[i] = make([]*MockPeer, 2)
		idx := 0
		for j := 0; j < 3; j++ {
			if j != i {
				mockPeers[i][idx] = NewMockPeer(nodes[j])
				peers[i][idx] = mockPeers[i][idx]
				idx++
			}
		}
		nodes[i].peers = peers[i]
	}

	for _, node := range nodes {
		node.Start()
		defer node.Stop()
	}

	time.Sleep(500 * time.Millisecond)

	for i := 0; i < len(mockPeers[0]); i++ {
		mockPeers[0][i].SetPartitioned(true)
		mockPeers[1][i].SetPartitioned(true)
	}

	time.Sleep(500 * time.Millisecond)

	leadersBefore := 0
	for _, node := range nodes {
		if node.GetRole() == Leader {
			leadersBefore++
		}
	}

	if leadersBefore == 0 {
		t.Log("No leader during partition (expected)")
	}

	for i := 0; i < len(mockPeers[0]); i++ {
		mockPeers[0][i].SetPartitioned(false)
		mockPeers[1][i].SetPartitioned(false)
	}

	time.Sleep(500 * time.Millisecond)

	leadersAfter := 0
	for _, node := range nodes {
		if node.GetRole() == Leader {
			leadersAfter++
		}
	}

	if leadersAfter != 1 {
		t.Errorf("Expected exactly 1 leader after partition recovery, got %d", leadersAfter)
	}
}

func TestRaftLeaderElectionWithDelay(t *testing.T) {
	nodes := make([]*Node, 3)
	peers := make([][]Peer, 3)

	for i := 0; i < 3; i++ {
		nodes[i] = NewNode(NodeID(fmt.Sprintf("N%d", i+1)), nil)
	}

	for i := 0; i < 3; i++ {
		peers[i] = make([]Peer, 2)
		idx := 0
		for j := 0; j < 3; j++ {
			if j != i {
				peer := NewMockPeer(nodes[j])
				if i == 0 {
					peer.SetDelay(200 * time.Millisecond)
				}
				peers[i][idx] = peer
				idx++
			}
		}
		nodes[i].peers = peers[i]
	}

	for _, node := range nodes {
		node.Start()
		defer node.Stop()
	}

	time.Sleep(1 * time.Second)

	leaders := 0
	var leaderID NodeID
	for _, node := range nodes {
		if node.GetRole() == Leader {
			leaders++
			leaderID = node.id
		}
	}

	if leaders != 1 {
		t.Errorf("Expected exactly 1 leader, got %d", leaders)
	}

	t.Logf("Leader elected: %s", leaderID)
}

func TestRaftTermProgress(t *testing.T) {
	nodes := make([]*Node, 3)
	peers := make([][]Peer, 3)

	for i := 0; i < 3; i++ {
		nodes[i] = NewNode(NodeID(fmt.Sprintf("N%d", i+1)), nil)
	}

	for i := 0; i < 3; i++ {
		peers[i] = make([]Peer, 2)
		idx := 0
		for j := 0; j < 3; j++ {
			if j != i {
				peers[i][idx] = NewMockPeer(nodes[j])
				idx++
			}
		}
		nodes[i].peers = peers[i]
	}

	for _, node := range nodes {
		node.Start()
		defer node.Stop()
	}

	time.Sleep(500 * time.Millisecond)

	term := nodes[0].GetTerm()
	for _, node := range nodes {
		if node.GetTerm() != term {
			t.Errorf("Terms not synchronized: expected %d, got %d", term, node.GetTerm())
		}
	}
}
