package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	kingpin "gopkg.in/alecthomas/kingpin.v1"
)

type result struct {
	Index int `json:"index"`

	Elapsed time.Duration `json:"elapsed"`
	Stop    time.Time     `json:"stop"`
	Start   time.Time     `json:"start"`

	URL        string `json:"url"`
	Method     string `json:"method"`
	Error      error  `json:"error"`
	Size       int    `json:"size"`
	StatusCode int    `json:"status_code"`
}

func (res result) String() string {
	return fmt.Sprintf(
		"%v  %v\t%v %v\tHTTP %v\t%v\t%v bytes",
		res.Start.Unix(),
		res.Stop.Unix(),
		res.Method,
		res.URL,
		res.StatusCode,
		res.Elapsed,
		res.Size,
	)
}

/*
	go run main.go ramp up 50 https://api.amberengine.com
	go run main.go ramp down 50 https://api.amberengine.com
	go run main.go do 50 https://api.amberengine.com
	go run main.go stress https://api.amberengine.com  # ramp up until big discrepencies
	go run main.go random https://api.amberengine.com

	OPTIONS:
		url
		duration
		request total
		output type (csv, text, json, graphs?)
		verborsity?
		concurrent vs procedural requesting (50 users at once vs 50 after another)
		headers
		request method
		asserts? (required status code, response body, content length??)
		step (amount of time between requests or increasing users?)
*/
var (
	app     = kingpin.New("raven", "A command-line HTTP stress test application.")
	verbose = app.Flag("verbose", "Enable verbose mode").Short('v').Bool()
	headers = app.Flag("headers", "Specify HTTP headers").Short('h').StringMap()
	auth    = app.Flag("authentication", "Provide a username:password").Short('a').String()
	raw     = app.Flag("raw", "Output raw data in specified format").Short('r').Enum(
		"json",
		"prettyjson",
		"csv",
	)

	ramp     = app.Command("ramp", "Ramp up/down requests over a period of time.")
	rampUp   = ramp.Command("up", "Ramp up requests over a period of time.")
	rampDown = ramp.Command("down", "Ramp down requests over a period of time.")

	rampUpMethod = rampUp.Arg("method", "HTTP request method").Required().String()
	rampUpURL    = rampUp.Arg("url", "Target URL address").Required().URL()

	rampDownMethod = rampDown.Arg("method", "HTTP request method").Required().String()
	rampDownURL    = rampDown.Arg("url", "Target URL address").Required().URL()
	rampUpAmt      = rampUp.Arg("max", "max number of requests to make").Required().Int()
	rampDownAmt    = rampDown.Arg("max", "max number of requests to make").Required().Int()

	do       = app.Command("do", "Immediately send requests")
	doAmt    = do.Arg("amount", "Amount of requests to make").Required().Int()
	doMethod = do.Arg("method", "HTTP request method").Required().String()
	doURL    = do.Arg("url", "Target URL address").Required().URL()

	stress           = app.Command("stress", `Ramp up requests until responses slow to within %x percent of a baseline`)
	stressType       = stress.Arg("type", `Stress method type ("duration" or "status")`).Required().Enum("duration", "status")
	stressMethod     = stress.Arg("method", "HTTP request method").Required().String()
	stressURL        = stress.Arg("url", "Target URL address").Required().URL()
	stressThreshold  = stress.Flag("threshold", `%xx percent threshold for valid responses`).Short('t').Default("10.0").Float()
	stressStart      = stress.Flag("start", "Provide a starting amount for concurrent requests").Short('s').Default("1").Int()
	stressIterations = stress.Flag("iterations", "Amount of iterations to perform for each step").Short('i').Default("10").Int()
	stressDelay      = stress.Flag("delay", "Millisecond delay between iterations").Short('d').Default("500").Int()
)

func parseMethod(method string) (string, error) {
	validMethods := map[string]struct{}{
		http.MethodGet:     struct{}{},
		http.MethodHead:    struct{}{},
		http.MethodPost:    struct{}{},
		http.MethodPut:     struct{}{},
		http.MethodPatch:   struct{}{},
		http.MethodDelete:  struct{}{},
		http.MethodConnect: struct{}{},
		http.MethodOptions: struct{}{},
		http.MethodTrace:   struct{}{},
	}

	capsMethod := strings.ToUpper(method)
	if _, ok := validMethods[capsMethod]; ok {
		return capsMethod, nil
	}

	return "", fmt.Errorf("invalid method: %v", method)
}

func performRequest(index int, client *http.Client, results chan result, wg *sync.WaitGroup, request *http.Request) {
	start := time.Now()
	resp, err := client.Do(request)
	stop := time.Now()
	elapsed := stop.Sub(start)

	res := result{
		Elapsed: elapsed,
		Index:   index,
		Method:  request.Method,
		Start:   start,
		Stop:    stop,
		URL:     request.URL.String(),
	}

	if err != nil {
		res.Error = err
	} else {
		res.Size = int(resp.ContentLength)
		res.StatusCode = resp.StatusCode
	}

	results <- res
	wg.Done()
}

func setupRequest(method, url string, headers map[string]string, basicAuth string) *http.Request {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		app.Fatalf(os.Stderr, fmt.Sprintf("could not setup request: %v", err))
	}

	if len(headers) > 0 {
		for key, val := range headers {
			req.Header.Set(key, val)
		}
	}

	if len(basicAuth) > 0 {
		parts := strings.Split(basicAuth, ":")
		if len(parts) != 2 {
			app.Fatalf(os.Stderr, "basic auth must be in the form username:password")
		}
		req.SetBasicAuth(parts[0], parts[1])
	}

	return req
}

func setupClient() *http.Client {
	var netTransport = &http.Transport{
		Dial: (&net.Dialer{
			Timeout: 5 * time.Second,
		}).Dial,
		TLSHandshakeTimeout: 5 * time.Second,
	}
	client := &http.Client{
		Timeout:   time.Second * 10,
		Transport: netTransport,
	}

	return client
}

func handleDo() {
	client := setupClient()

	method, err := parseMethod(*doMethod)
	if err != nil {
		app.Fatalf(os.Stderr, fmt.Sprintf("could not parse request method: %v", err))
	}

	wg := new(sync.WaitGroup)
	results := make(chan result, *doAmt)
	preparedRequests := make([]*http.Request, *doAmt)

	execStart := time.Now()
	setupStart := time.Now()

	for i := 0; i < *doAmt; i++ {
		preparedRequests[i] = setupRequest(method, (*doURL).String(), *headers, *auth)
	}

	for i, r := range preparedRequests {
		wg.Add(1)
		go performRequest(i, client, results, wg, r)
	}

	setupStop := time.Now()

	wg.Wait()
	close(results)
	execStop := time.Now()

	if len(*raw) > 0 {
		var allResults []result
		for r := range results {
			allResults = append(allResults, r)
		}

		if *raw == "prettyjson" {
			bytes, err := json.MarshalIndent(allResults, "", "    ")
			if err != nil {
				app.Fatalf(os.Stderr, fmt.Sprintf("could not render raw output: %v", err))
			}
			fmt.Println(string(bytes))
		} else if *raw == "json" {
			bytes, err := json.Marshal(allResults)
			if err != nil {
				app.Fatalf(os.Stderr, fmt.Sprintf("could not render raw output: %v", err))
			}
			fmt.Println(string(bytes))
		} else if *raw == "csv" {
			fmt.Println("index,elapsed,stop,start,url,method,error,size,status_code")
			for _, r := range allResults {
				fmt.Printf(
					"%v,%f,%v,%v,%v,%v,%v,%v,%v\n",
					r.Index,
					float64(r.Elapsed)/float64(time.Second),
					r.Stop,
					r.Start,
					r.URL,
					r.Method,
					r.Error,
					r.Size,
					r.StatusCode,
				)
			}
		}

		return
	}

	statusMap := map[int]int{}
	sizeSum := 0
	sum := 0
	min := -1
	max := 0

	for r := range results {
		if i, ok := statusMap[r.StatusCode]; ok {
			statusMap[r.StatusCode] = i + 1
		} else {
			statusMap[r.StatusCode] = 1
		}

		e := int(r.Elapsed)

		sizeSum = sizeSum + r.Size
		sum = sum + e
		if e > max {
			max = e
		}
		if e < min || min == -1 {
			min = e
		}
	}

	fmt.Println("Total Requests:     ", *doAmt)
	fmt.Println("Elapsed Duration:   ", execStop.Sub(execStart))
	if *verbose {
		fmt.Println("Setup duration:     ", setupStop.Sub(setupStart))
	}

	fmt.Println("")

	fmt.Println("Average Request Duration: ", time.Duration(sum / *doAmt))
	fmt.Println("Min Request Duration:     ", time.Duration(min))
	fmt.Println("Max Request Duration:     ", time.Duration(max))

	fmt.Println("")

	fmt.Println("Total Response Size (bytes):   ", sizeSum)
	fmt.Println("Average Response Size (bytes): ", sizeSum / *doAmt)

	fmt.Println("")

	fmt.Println("Status Codes:")
	for code, amt := range statusMap {
		fmt.Printf("\tHTTP %v:\t%v\n", code, amt)
	}
}

func handleStress() {
	if *stressStart <= 0 {
		app.Fatalf(os.Stderr, "'-s' must be a minimum of '1' request concurrently.")
	}

	client := setupClient()

	method, err := parseMethod(*stressMethod)
	if err != nil {
		app.Fatalf(os.Stderr, fmt.Sprintf("could not parse request method: %v", err))
	}

	baselineSum := 0

	for i := 0; i < *stressIterations; i++ {
		r := setupRequest(method, (*stressURL).String(), *headers, *auth)

		start := time.Now()
		_, err := client.Do(r)
		if err != nil {
			app.Fatalf(os.Stderr, fmt.Sprintf("request failed: %v", err))
		}

		stop := time.Now()
		elapsed := stop.Sub(start)

		baselineSum = baselineSum + int(elapsed)
	}

	avgBaseline := float64(baselineSum) / float64(*stressIterations)
	maxResponseTime := time.Duration((1.00 + (*stressThreshold / 100.0)) * avgBaseline)

	fmt.Println("Step delay:                  ", time.Duration(*stressDelay))
	fmt.Println("Baseline response time:      ", time.Duration(avgBaseline))
	fmt.Printf("Percent threshold:            %f percent\n", *stressThreshold)
	if *stressType == "duration" {
		fmt.Printf("Max acceptable response time: %v\n\n", maxResponseTime)
	} else {
		fmt.Println("")
	}

	for reqCount := *stressStart; ; reqCount++ {
		fmt.Printf("Performing %d concurrent requests...\n", reqCount)

		reqSum := 0
		reqNum := 0
		reqMin := -1
		reqMax := 0

		reqNon200 := 0
		maxNon200 := int(*stressThreshold / 100.0 * float64(reqCount) * float64(*stressIterations))

		if *stressType == "status" && *verbose {
			fmt.Println("    Max acceptable non-200 amount:", maxNon200)
		}
		for loop := 0; loop < *stressIterations; loop++ {
			if *verbose {
				fmt.Println("\t...performing iteration", loop, "of", *stressIterations)
			}

			wg := new(sync.WaitGroup)
			results := make(chan result, reqCount)
			preparedRequests := make([]*http.Request, reqCount)

			for i := 0; i < reqCount; i++ {
				preparedRequests[i] = setupRequest(method, (*stressURL).String(), *headers, *auth)
			}

			for i, r := range preparedRequests {
				wg.Add(1)
				go performRequest(i, client, results, wg, r)
			}

			wg.Wait()
			close(results)

			for r := range results {
				e := int(r.Elapsed)
				reqSum = reqSum + e
				reqNum = reqNum + 1

				if e > reqMax {
					reqMax = e
				}
				if e < reqMin || reqMin == -1 {
					reqMin = e
				}

				if r.StatusCode != 200 {
					reqNon200 = reqNon200 + 1
				}
			}
			time.Sleep(time.Duration(*stressDelay * int(time.Millisecond)))
		}

		avg := time.Duration(float64(reqSum) / float64(reqNum))

		if *verbose {
			fmt.Println("")
			fmt.Println("\tAverage: ", time.Duration(avg))
			fmt.Println("\tMin:     ", time.Duration(reqMin))
			fmt.Println("\tMax:     ", time.Duration(reqMax))
			fmt.Println("\t200s:    ", reqNum-reqNon200)
			fmt.Println("\tNon200s: ", reqNon200)

			fmt.Println("")
		}

		if (*stressType == "duration") && avg > maxResponseTime {
			fmt.Printf(
				"%v exceeds %v\n",
				avg,
				maxResponseTime,
			)
			break
		} else if (*stressType == "status") && int(reqNon200) > int(maxNon200) {
			fmt.Printf(
				"%d non-200s exceeds %d\n",
				reqNon200,
				maxNon200,
			)
			break
		}
	}
}

func main() {
	cmd := kingpin.MustParse(app.Parse(os.Args[1:]))

	if *verbose && len(*raw) > 0 {
		app.Fatalf(os.Stderr, "cannot user 'verbose' and 'raw' mode at same time")
	}

	switch cmd {
	case do.FullCommand():
		handleDo()
	case stress.FullCommand():
		handleStress()
	}
}
