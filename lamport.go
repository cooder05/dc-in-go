package main

import (
	"fmt"
	"sync"
	"time"
)



type p struct {
	id      int
	balance int
	snaped  bool
}

type Snapshot struct {
    id        int
    balance  int
    inTransit map[int][]int // messages received after snapshot started
}
var snapshots [3]Snapshot 
var snapshotMu sync.Mutex
type MyChannels [3][3]chan int
var done = make(chan struct{})
var snapshotInProgress bool
var wg sync.WaitGroup // ✅ global WaitGroup

func main() {
	var accounts [3]p = [3]p{{0, 100, false}, {1, 100, false}, {2, 100, false}}

	var channelGrid MyChannels
	isChannelOpen := [3][3]bool{
		{true, true, true},
		{true, true, true},
		{true, true, true},
	}

	// Initialize each channel in the grid
	wg.Add(6)
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			if i != j {
				channelGrid[i][j] = make(chan int, 3)
				go check_channel(&channelGrid, &isChannelOpen, &accounts, i, j)
			}
		}
	}

	wg.Add(5)
	go accounts[0].transfer(&channelGrid, &accounts, 10, 2)
	go accounts[0].transfer(&channelGrid, &accounts, 10, 1)
	go accounts[2].transfer(&channelGrid, &accounts, 10, 1)
	go accounts[0].transfer(&channelGrid, &accounts, 10, 2)

	time.Sleep(1 * time.Second)

	go accounts[0].take_snap(&channelGrid, &accounts,true)
	time.Sleep(2 * time.Second)
	close(done)
	wg.Wait()

	fmt.Println("All goroutines stopped.")
	fmt.Println("=== Final Balances ===")
	for _, acc := range accounts {
		fmt.Printf("Process[%d] final balance = %d\n", acc.id, acc.balance)
	}
}

func (from *p) transfer(c *MyChannels, processes *[3]p, amount int, to int) {
	defer wg.Done()
	select {
	case <-done:
		return
	default:
		if amount > 0 && amount <= from.balance && c[from.id][to] != nil {
			select {
			case c[from.id][to] <- amount: // try sending
				from.balance -= amount
				//fmt.Printf("Transferring %d from %d -> %d\n", amount, from.id, to)
			case <-done: // stop if done before send
				fmt.Println("Stopped transfer during send")
				return
			}
		} else {
			fmt.Println("Insufficient balance")
		}
	}
}

func check_channel(chs *MyChannels, isOpen *[3][3]bool, processes *[3]p, from int, to int) {
	defer wg.Done()
	for {
		select {
		case <-done:
			return
		case message := <-chs[from][to]:
			switch message {
			case 0:
				processes[to].take_snap(chs, processes,false)
				isOpen[from][to] = false
			
				snapshotMu.Lock()
			DrainLoop:
			for {
				select {
				case msg := <-chs[from][to]:
					
					if snapshots[to].inTransit == nil {
    					snapshots[to].inTransit = make(map[int][]int)
					}
					snapshots[to].inTransit[from] = append(snapshots[to].inTransit[from], msg)
				default:
					break DrainLoop  // exits the outer for loop
				}
			}
			snapshotMu.Unlock()

			default:
				if isOpen[from][to] {
					processes[to].balance += message
					//fmt.Printf("transfered [amount:%d] from %d->%d\n", message, from, to)
				}
			}
		default:
			all_closed(isOpen)
		}
	}
}

func (start *p) take_snap(c *MyChannels, processes *[3]p,topLevel bool) {
	if topLevel {
        defer wg.Done()
		snapshotMu.Lock()
		if !snapshotInProgress {
            snapshotInProgress = true // mark snapshot in progress
        }
		snapshotMu.Unlock()
    }
	select {
	case <-done:
		fmt.Println("Stopped")
		return
	default:
		if !start.snaped {
			start.snaped = true
			snapshotMu.Lock()
			
			snapshots[start.id] = Snapshot{
				id: start.id,
				balance:   start.balance,
				inTransit: make(map[int][]int),
			}
			currentSnapshot := snapshots[start.id]
			snapshotMu.Unlock()
			fmt.Printf("=>process[%d] snapped data = %d\n", start.id, start.balance)
			fmt.Printf("In-transit messages: %v\n", currentSnapshot.inTransit)

			for toProcess, channel := range c[start.id] {
				if start.id != toProcess && channel != nil {
					select {
					case channel <- 0:
						// marker sent successfully
					case <-done:
						fmt.Println("Stopped snapshot during marker send")
						return
					}
				}
			}
		}
	}
}

func all_closed(c_states*[3][3]bool){
	for _ , row := range c_states{
		for _ , c := range row{
			if !c{
				return 
			}
		}
	}

	for i := range c_states {
		for j := range c_states[i] {
			c_states[i][j] = true
		}
	}
	 if snapshotInProgress {
        snapshotInProgress = false
		fmt.Println("all channels recorded — resetting")
    }
}