package main

import (
	"fmt"
	"math/rand"
	"sync"
	"time"
)

type MyChannels [][]chan int

// Snapshot state per process per snapshot round
type Snapshot struct {
	id        int
	snaped    bool
	markers   map[int]bool
	inTransit map[int][]int
}

// process
type p struct {
	id      int
	balance int
	mu      sync.Mutex
	snaps   map[int]*Snapshot // multiple snapshots possible
}

var snapshotID int
var snapshotMu sync.Mutex
var wg sync.WaitGroup

// transfer money
func (from *p) transfer(to *p, amount int, c MyChannels) {
	defer wg.Done()
	from.mu.Lock()
	if from.balance >= amount {
		from.balance -= amount
		c[from.id][to.id] <- amount
		fmt.Printf("Transferred [amount:%d] from %d -> %d\n", amount, from.id, to.id)
	}
	from.mu.Unlock()
}

// handle marker message
func (account *p) handleMarker(from int, snapID int, c MyChannels, accounts []*p) {
	account.mu.Lock()
	defer account.mu.Unlock()

	snap, ok := account.snaps[snapID]
	if !ok {
		// first marker → take local snapshot
		snap = &Snapshot{
			id:        snapID,
			snaped:    true,
			markers:   make(map[int]bool),
			inTransit: make(map[int][]int),
		}
		account.snaps[snapID] = snap
		fmt.Printf("process[%d] snapped data (snap %d) = %d\n", account.id, snapID, account.balance)

		// send markers to neighbors
		for to, ch := range c[account.id] {
			if account.id != to && ch != nil {
				ch <- -snapID // negative = marker
			}
		}
	}

	// record marker arrival from "from"
	if from >= 0 {
		snap.markers[from] = true
	}

	// check if all incoming seen (2 neighbors in 3-proc case)
	if len(snap.markers) == len(accounts)-1 {
		fmt.Printf("process[%d] completed snapshot %d. In-transit: %+v\n",
			account.id, snapID, snap.inTransit)
	}
}

// listener goroutine
func (account *p) listen(c MyChannels, accounts []*p) {
	for from := 0; from < len(accounts); from++ {
		if from == account.id {
			continue
		}
		go func(from int) {
			for msg := range c[from][account.id] {
				if msg < 0 {
					// marker
					snapID := -msg
					account.handleMarker(from, snapID, c, accounts)
				} else {
					// normal transfer
					account.mu.Lock()
					account.balance += msg
					account.mu.Unlock()
				}
			}
		}(from)
	}
}

// initiate a new snapshot
func startSnapshot(start *p, c MyChannels, accounts []*p) {
	snapshotMu.Lock()
	snapshotID++
	sid := snapshotID
	snapshotMu.Unlock()

	fmt.Printf("\n=== Starting snapshot %d from process %d ===\n", sid, start.id)
	start.handleMarker(-1, sid, c, accounts)
}

func main() {
	rand.Seed(time.Now().UnixNano())

	// 3 processes
	n := 3
	accounts := make([]*p, n)
	for i := 0; i < n; i++ {
		accounts[i] = &p{id: i, balance: 100, snaps: make(map[int]*Snapshot)}
	}

	// channels
	channels := make(MyChannels, n)
	for i := range channels {
		channels[i] = make([]chan int, n)
		for j := range channels[i] {
			if i != j {
				channels[i][j] = make(chan int, 100)
			}
		}
	}

	// start listeners
	for _, acc := range accounts {
		acc.listen(channels, accounts)
	}

	// some transfers
	wg.Add(3)
	go accounts[0].transfer(accounts[1], 10, channels)
	go accounts[2].transfer(accounts[1], 10, channels)
	go accounts[0].transfer(accounts[2], 10, channels)
	wg.Wait()

	// start snapshot from proc 0
	startSnapshot(accounts[0], channels, accounts)

	time.Sleep(1 * time.Second)

	// more transfers
	wg.Add(2)
	go accounts[0].transfer(accounts[2], 20, channels)
	go accounts[1].transfer(accounts[0], 15, channels)
	wg.Wait()

	// start snapshot from proc 1
	startSnapshot(accounts[1], channels, accounts)

	time.Sleep(1 * time.Second)
}
