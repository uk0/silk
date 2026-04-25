package core

/*
import (
	"strconv"
	//	"sync"
	"time"
)

var workersMutex = make(chan int, 1)
var workers = make(map[*_Worker]int)
var nextWorkerSn int
var allWorkersDone = make(chan int, 1)

func init() {
	allWorkersDone <- 1
}

type _Worker struct {
	sn    int
	title string
	time  time.Time
}

func (p *_Worker) String() string {
	return `No.` + strconv.Itoa(p.sn) + ` "` + p.title + `" ` + p.time.String()
}

//
func (p *_Worker) Title() string {
	return p.title
}

// 开始工作的时间
func (p *_Worker) Time() time.Time {
	return p.time
}

// 从开始到现在经过的时间
func (p *_Worker) Duration() time.Duration {
	return time.Now().Sub(p.time)
}

func (p *_Worker) Done() {
	workersMutex <- 1
	_, ok := workers[p]
	if ok {
		Trace("worker done: ", p)
		delete(workers, p)
		if len(workers) == 0 {
			allWorkersDone <- 1
		}
	}
	<-workersMutex
}

func RegisterWorker(title string) *_Worker {
	//	mustNotClosed()

	p := new(_Worker)
	p.time = time.Now()
	p.title = title
	workersMutex <- 1
	p.sn = nextWorkerSn
	nextWorkerSn++
	workers[p] = 1

	// 清除信号
	select {
	case <-allWorkersDone:
	default:
	}
	<-workersMutex
	Trace("register worker: ", p)
	return p
}

func waitAllWorkersDone() {
	Trace("wait workers")
	<-allWorkersDone
	Trace("all workers done")
}

*/
