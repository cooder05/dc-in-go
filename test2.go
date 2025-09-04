package main

import (
	"fmt"
	"math/rand"
	"sync"
	"time"
)

type p struct {
	id      int
	balance int
	isRed   bool
	sentLog map[int][]int
	recvLog map[int][]int
}

type Snapshot struct {
	id        int
	balance   int
	inTransit map[int][]int
	recvLog   map[int][]int // only received messages
}

var snapshots [3]Snapshot
var snapshotMu sync.Mutex

type Message struct {
	Amount int
	IsRed  bool
}

type arr struct {
	messages []Message
	mu       sync.Mutex
}

func (a *arr) send(msg Message) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.messages = append(a.messages, msg)
}

func (a *arr) recv() (Message, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.messages) == 0 {
		return Message{}, false
	}
	idx := rand.Intn(len(a.messages)) // non-FIFO
	msg := a.messages[idx]
	a.messages[idx] = a.messages[len(a.messages)-1]
	a.messages = a.messages[:len(a.messages)-1]
	return msg, true
}

type MyChannels [3][3]*arr
var done = make(chan struct{})
var wg sync.WaitGroup

func main() {
	rand.Seed(time.Now().UnixNano())

	var accounts [3]p = [3]p{
		{0, 100, false, make(map[int][]int), make(map[int][]int)},
		{1, 100, false, make(map[int][]int), make(map[int][]int)},
		{2, 100, false, make(map[int][]int), make(map[int][]int)},
	}

	var channelGrid MyChannels

	// Initialize channels
	wg.Add(6)
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			if i != j {
				channelGrid[i][j] = &arr{messages: []Message{}}
				go check_channel(&channelGrid, &accounts, i, j)
			}
		}
	}

	// Transfers
	wg.Add(5)
	go accounts[0].transfer(&channelGrid, 10, 2)
	go accounts[0].transfer(&channelGrid, 10, 1)
	go accounts[2].transfer(&channelGrid, 10, 1)
	go accounts[0].transfer(&channelGrid, 10, 2)
	go accounts[1].transfer(&channelGrid, 5, 0)

	time.Sleep(1 * time.Second)

	// Take snapshot from P0
	wg.Add(1)
	go accounts[0].take_snap(&channelGrid, true)
	time.Sleep(2 * time.Second)
	wg.Add(1)
	go accounts[1].transfer(&channelGrid, 5, 0)

	time.Sleep(3 * time.Second)
	close(done)
	wg.Wait()

	fmt.Println("All goroutines stopped.")
	fmt.Println("=== Final Balances ===")
	for _, acc := range accounts {
		fmt.Printf("Process[%d] final balance = %d\n", acc.id, acc.balance)
	}
	fmt.Println("\n=== Snapshots ===")
	for _, snap := range snapshots {
		fmt.Printf("Process[%d] snapshot receive log: %v\n", snap.id, snap.recvLog)
		fmt.Printf("  In-transit: %v\n\n", snap.inTransit)
	}
}

func (from *p) transfer(c *MyChannels, amount int, to int) {
	defer wg.Done()
	select {
	case <-done:
		return
	default:
		if amount > 0 && amount <= from.balance && c[from.id][to] != nil {
			from.balance -= amount
			msg := Message{Amount: amount, IsRed: from.isRed}
			c[from.id][to].send(msg)
			if !from.isRed {
				from.sentLog[to] = append(from.sentLog[to], amount)
			}
		} else {
			fmt.Println("Insufficient balance")
		}
	}
}

func check_channel(chs *MyChannels, processes *[3]p, from int, to int) {
	defer wg.Done()
	for {
		select {
		case <-done:
			return
		default:
			message, ok := chs[from][to].recv()
			if !ok {
				continue
			}

			recvProc := &processes[to]
			if message.IsRed { // marker
				if !recvProc.isRed {
					recvProc.take_snap(chs, false)
				}
			} else { // normal message
				if !recvProc.isRed {
					recvProc.recvLog[from] = append(recvProc.recvLog[from], message.Amount)
				} else {
					snapshotMu.Lock()
					if snapshots[to].inTransit == nil {
						snapshots[to].inTransit = make(map[int][]int)
					}
					snapshots[to].inTransit[from] = append(snapshots[to].inTransit[from], message.Amount)
					snapshotMu.Unlock()
				}
				recvProc.balance += message.Amount
			}
		}
	}
}

func (start *p) take_snap(c *MyChannels, topLevel bool) {
	if topLevel {
		defer wg.Done()
	}
	if !start.isRed {
		start.isRed = true
		snapshotMu.Lock()
		snapshots[start.id] = Snapshot{
			id:        start.id,
			balance:   start.balance,
			inTransit: make(map[int][]int),
			recvLog:   copyLog(start.recvLog),
		}
		snapshotMu.Unlock()
		fmt.Printf("=> Process[%d] snapped data = %d\n", start.id, start.balance)

		for toProcess, channel := range c[start.id] {
			if start.id != toProcess && channel != nil {
				channel.send(Message{Amount: 0, IsRed: true})
			}
		}
	}
}

func copyLog(log map[int][]int) map[int][]int {
	c := make(map[int][]int)
	for k, v := range log {
		c[k] = append([]int(nil), v...)
	}
	return c
}