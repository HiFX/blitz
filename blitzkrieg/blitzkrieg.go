package blitzkrieg

import (
	"crypto/tls"
	"fmt"
	"github.com/rakyll/pb"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// A Blitz contains all the vars to perform the load test
type Blitz struct {
	requests       []*blitzRequest      // generated from URL/URLs file
	count          int                  //Number of requests
	clients        int                  //The number of concurrent clients to run
	duration       int                  // Duration to run the test
	keepAlive      bool                 //Whether to set KeepAlive ON or NOT
	gzip           bool                 //Whether to enable gzip or not
	connectTimeout int                  //Connect timeout in ms
	readTimeout    int                  //Read timeout in ms
	writeTimeout   int                  //Write timeout in ms
	rate           int                  // Rate limit.
	header         http.Header          // Http Headers
	startTime      time.Time            // Start time
	bar            *pb.ProgressBar      // Progress bar
	jobs           chan *blitzRequest   //Jobs channel
	results        chan *[]*blitzResult //Results Channel holds array of blitzResult (size blitz.clients)
}

type BlitzConn struct {
	net.Conn
	readTimeout  time.Duration
	writeTimeout time.Duration
}

func (blitzConn *BlitzConn) Read(b []byte) (n int, err error) {
	len, err := blitzConn.Conn.Read(b)
	if err == nil {
		blitzConn.Conn.SetReadDeadline(time.Now().Add(blitzConn.readTimeout))
	}
	return len, err
}

func (blitzConn *BlitzConn) Write(b []byte) (n int, err error) {
	len, err := blitzConn.Conn.Write(b)
	if err == nil {
		blitzConn.Conn.SetWriteDeadline(time.Now().Add(blitzConn.writeTimeout))
	}
	return len, err
}

// Run sets up the variables and runs the load test
func (blitz *Blitz) Run() {
	//Results channel
	blitz.results = make(chan *[]*blitzResult, blitz.clients)
	if blitz.duration != 0 { // test to be run for blitz.duration seconds
		setTimeout(blitz.duration)
		blitz.bar = newPBar(blitz.duration)
		go blitz.showDurationPBar()
	} else { // test to be run for blitz.count requests
		blitz.bar = newPBar(blitz.count)
	}
	blitz.handleInterrupts() // Handle Ctrl+C and other interrupts
	blitz.run()
}

// Runs creates blitz.clients number of goroutines and sends
// requests to them via the jobs channel
func (blitz *Blitz) run() {
	blitz.jobs = make(chan *blitzRequest, blitz.clients*5)

	// Throttle the rate at which requests are sent in the job channel if Rate limiting is applicable
	var throttler <-chan time.Time
	if blitz.rate > 0 {
		throttler = time.Tick(time.Duration(1e6/(blitz.rate)) * time.Microsecond)
	}
	blitz.startTime = time.Now()
	var waitr sync.WaitGroup
	waitr.Add(blitz.clients)
	fmt.Printf("Preparing %d concurrent users:\n", blitz.clients)
	for i := 0; i < blitz.clients; i++ {
		go func() {
			blitz.raider()
			waitr.Done()
		}()
	}
	// Sends the requests to the jobs channel
	requestCount := len(blitz.requests)
	for i, j := 0, 0; i < blitz.count; i, j = i+1, j+1 {
		if blitz.rate > 0 {
			<-throttler
		}
		if j == requestCount {
			j = 0
		}
		blitz.jobs <- blitz.requests[j]
	}
	close(blitz.jobs)
	waitr.Wait()
	blitz.bar.Finish()
	blitz.report()
}

func (blitz *Blitz) raider() {
	result := make([]*blitzResult, 0)
	blitz.results <- &result
	tr := &http.Transport{
		TLSClientConfig:    &tls.Config{InsecureSkipVerify: true},
		DisableKeepAlives:  !blitz.keepAlive,
		DisableCompression: !blitz.gzip,
	}
	tr.Dial = func(network string, address string) (net.Conn, error) {
		conn, err := net.DialTimeout(network, address, time.Duration(blitz.connectTimeout)*time.Millisecond)
		if err != nil {
			return nil, err
		}
		conn.SetReadDeadline(time.Now().Add(time.Duration(blitz.readTimeout) * time.Millisecond))
		conn.SetWriteDeadline(time.Now().Add(time.Duration(blitz.writeTimeout) * time.Millisecond))

		bConn := &BlitzConn{Conn: conn, readTimeout: time.Duration(blitz.readTimeout) * time.Millisecond, writeTimeout: time.Duration(blitz.writeTimeout) * time.Millisecond}
		return bConn, nil

	}
	//client := &http.Client{Transport: tr}

	for req := range blitz.jobs {
		s := time.Now()
		//resp, err := client.Do(req.getHttpRequest())
		resp, err := tr.RoundTrip(req.getHttpRequest())
		code := 0
		var size int64 = 0
		if resp != nil {
			code = resp.StatusCode
			if body, err := ioutil.ReadAll(resp.Body); err == nil {
				if code >= 200 && code <= 302 {
					size = int64(len(body))
				}
			}
			io.Copy(ioutil.Discard, resp.Body)
			resp.Body.Close()
		}
		result = append(result, &blitzResult{
			statusCode:    code,
			duration:      time.Now().Sub(s),
			err:           err,
			contentLength: size,
			timestamp:     time.Now(),
		})
		if blitz.duration == 0 {
			blitz.bar.Increment()
		}
	}

}

func (blitz *Blitz) handleInterrupts() {
	signalChannel := make(chan os.Signal, 2) // Handle Interruptions
	signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)
	go func() {
		_ = <-signalChannel
		blitz.jobs = nil
		blitz.bar.Finish()
		blitz.report()
		os.Exit(0)
	}()
}

func (blitz *Blitz) showDurationPBar() {
	ticker := time.Tick(1 * time.Second)
	for i := 0; i < blitz.count; i++ {
		blitz.bar.Increment()
		<-ticker
	}
}

func newPBar(size int) (bar *pb.ProgressBar) {
	bar = pb.New(size)
	bar.ShowBar = false
	bar.ShowPercent = true
	bar.Start()
	return
}

func setTimeout(duration int) {
	timeout := make(chan bool, 1)

	go func() {
		time.Sleep(time.Duration(duration) * time.Second)
		timeout <- true
	}()
	go func() {
		<-timeout
		//signal would be sent to signalChannel and report() would be called
		syscall.Kill(syscall.Getpid(), syscall.SIGTERM)
	}()
}
