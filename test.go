package main
import (
	"fmt"
	"sync"
	"time"
)
var done = make(chan struct{})
var snapshotInProgress bool
var wg sync.WaitGroup  // ✅ global WaitGroup


type p struct{
	id int
	balance int
	time int
	snaped bool
}
type MyChannels [3][3]chan int

func main(){
	var accounts [3]p = [3]p{{0,100,0,false},{1,100,0,false},{2,100,0,false}}
	
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
			if(i !=j){
				channelGrid[i][j] = make(chan int,3)
				go check_channel(&channelGrid,&isChannelOpen,&accounts,i,j)
			}
		}
	}
	
	wg.Add(5)
	go accounts[0].transfer(&channelGrid,&accounts,10,2)
	go accounts[0].transfer(&channelGrid,&accounts,10,1)
	go accounts[2].transfer(&channelGrid,&accounts,10,1)
	go accounts[0].transfer(&channelGrid,&accounts,10,2)
	go accounts[0].take_snap(&channelGrid,&accounts,true)

	time.Sleep(2 * time.Second)
	close(done)
	wg.Wait()
	fmt.Println("All goroutines stopped.")
}


func (from *p)transfer(c *MyChannels,processes *[3]p,amount int,to int){
	defer wg.Done()
	select {
	case <-done:
		fmt.Println("Stopped")
		return
	default:
		if(amount > 0 &&amount <= from.balance && c[from.id][to] != nil){
		select {
		case c[from.id][to] <- amount: // try sending
			from.balance -= amount
			fmt.Printf("Transferred %d from %d -> %d\n", amount, from.id, to)
		case <-done: // stop if done before send
			fmt.Println("Stopped transfer during send")
			return
		}
		} else {
		fmt.Println("Insufficient balance")
		}
	}
}

func check_channel(chs *MyChannels,isOpen *[3][3]bool,processes *[3]p,from int,to int){
	defer wg.Done()
	for {
		select {
		case <-done:
			fmt.Println("Stopped")
			return
		case message := <-chs[from][to]:
			switch message {
			case 0:
				processes[to].take_snap(chs, processes,false)
				isOpen[from][to] = false

				// drain channel contents
			DrainLoop:
				for {
					select {
					case msginclosed := <-chs[from][to]:
						fmt.Printf("%d,", msginclosed)
					default:
						fmt.Print("]\n")
						break DrainLoop
					}
				}

			default:
				if isOpen[from][to] {
					processes[to].balance += message
					fmt.Printf("transfered [amount:%d] from %d->%d\n", message, from, to)
				}
			}
		default:
			if all_closed(isOpen) {
				fmt.Println("all channels recorded — resetting")
			}
		}
	}
}

func (start *p)take_snap(c *MyChannels,processes *[3]p,topLevel bool){
	if topLevel {
        defer wg.Done()
    }
	select {
	case <-done:
		fmt.Println("Stopped")
		return
	default:
		if (!start.snaped){
			start.snaped = true
			fmt.Printf("process[%d] snapped data = %d\n",start.id,start.balance)
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

func all_closed(c_states*[3][3]bool) bool{
	for _ , row := range c_states{
		for _ , c := range row{
			if !c{
				return false
			}
		}
	}

	for i := range c_states {
		for j := range c_states[i] {
			c_states[i][j] = true
		}
	}
	 if !snapshotInProgress {
        snapshotInProgress = true
        return true
    }
	return false
}