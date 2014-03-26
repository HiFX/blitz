package blitzkrieg

import (
	"fmt"
	"github.com/bmizerany/perks/quantile"
	"text/tabwriter"
	"time"

	"bytes"
	"sort"
)

// A blitzResult represents the result of an http Request
type blitzResult struct {
	err           error
	statusCode    int
	duration      time.Duration
	contentLength int64
}

// report represents the results of the load test
type report struct {
	statusCodes     map[int]int
	errors          map[string]int
	latencies       []float64
	percentile50Lat float64
	percentile99Lat float64
	maxLat          float64
	avgLat          float64
	totalTime       float64
	totalTimeSum    float64
	totalSize       int64
	totalRequests   int64
	totalSuccess    int64
	totalHttpErrors int64
	rate            float64
}

func (blitz *Blitz) report() {
	fmt.Println("\nPreparing report...")
	report := &report{statusCodes: make(map[int]int), errors: make(map[string]int)}
	quants := quantile.NewTargeted(0.50, 0.99)
	var duration float64
	for {
		select {
		case results := <-blitz.results:
			for _, result := range results {
				report.totalRequests++
				duration = result.duration.Seconds()
				if result.err != nil {
					report.errors[result.err.Error()]++
					report.totalHttpErrors++
				} else {
					report.statusCodes[result.statusCode]++
					if result.statusCode >= 200 && result.statusCode <= 300 {
						report.totalSuccess++
					}
					report.latencies = append(report.latencies, duration)
					report.totalTimeSum += duration
					if result.contentLength > 0 {
						report.totalSize += result.contentLength
					}
					if duration > report.maxLat {
						report.maxLat = duration
					}
					quants.Insert(duration)
				}
			}
		default:
			report.totalTime = time.Now().Sub(blitz.startTime).Seconds()
			report.percentile50Lat = quants.Query(0.50)
			report.percentile99Lat = quants.Query(0.99)
			if report.totalTimeSum > 0 {
				report.avgLat = report.totalTimeSum / float64(len(report.latencies))
			}
			print(report)
			return
		}
	}
}

func print(report *report) {
	var statusCodes []int
	for code := range report.statusCodes {
		statusCodes = append(statusCodes, code)
	}
	sort.Ints(statusCodes)

	out := &bytes.Buffer{}
	tabw := tabwriter.NewWriter(out, 0, 8, 3, ' ', tabwriter.StripEscape)
	fmt.Fprintf(tabw, "----------------------------------------------------------------------------------\n")
	fmt.Fprintf(tabw, "Requests\t[total]\t%d hits\n", report.totalRequests)
	fmt.Fprintf(tabw, "Requests\t[success]\t%d hits\n", report.totalSuccess)
	fmt.Fprintf(tabw, "Availability\t[ratio]\t%3.3f%%\n", float64(report.totalSuccess)*100/float64(report.totalRequests))
	fmt.Fprintf(tabw, "Status Codes\t[code:count]\t")
	for _, code := range statusCodes {
		fmt.Fprintf(tabw, "%d:%d  ", code, report.statusCodes[code])
	}
	fmt.Fprintf(tabw, "%s:%d  ", "NIL", report.totalHttpErrors)
	fmt.Fprintf(tabw, "\nLatencies\t[mean, 50p, 99p, max]\t%3.4fs, %3.4fs, %3.4fs, %3.4fs\n", report.avgLat, report.percentile50Lat, report.percentile99Lat, report.maxLat)
	fmt.Fprintf(tabw, "Request Rate\t[success]\t%5.3f hits/sec\n", float64(report.totalSuccess)/report.totalTime)
	fmt.Fprintf(tabw, "Data Recieved\t[total]\t%4.5f MB\n", float64(report.totalSize)/1048576)
	fmt.Fprintf(tabw, "Duration\t[total]\t%3.2f secs\n", report.totalTime)
	fmt.Fprintf(tabw, "----------------------------------------------------------------------------------")

	if len(report.errors) > 0 && showErr {
		fmt.Fprintln(tabw, "\n\nErrors: [error]: [count]")
		for key, count := range report.errors {
			fmt.Fprintf(tabw, "%s: [%d]  \n", key, count)
		}
	}
	tabw.Flush()
	fmt.Println(out.String())

	switch outFormat {
	case "graph":
		graph(report)
	case "csv":
		csv(report)
	}
}

func graph(report *report) {
	fmt.Println("graph to be written")
}

func csv(report *report) {
	fmt.Println("csv to be written")
}
