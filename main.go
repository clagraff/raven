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

	"github.com/pkg/errors"
	kingpin "gopkg.in/alecthomas/kingpin.v1"
)

func getVersion() string {
	return "1.0.0-alpha"
}

func marshalTest(format string, tests []*endpointTest) (string, error) {
	var output string
	switch format {
	case "json":
		bites, err := json.Marshal(tests)
		if err != nil {
			return output, errors.Wrap(
				err,
				"could not JSON marshal test results",
			)
		}
		output = string(bites)
	case "prettyjson":
		bites, err := json.MarshalIndent(tests, "", "    ")
		if err != nil {
			return output, errors.Wrap(
				err,
				"could not JSON marshal test results",
			)
		}
		output = string(bites)
	case "csv":
		bites, err := json.Marshal(tests)
		if err != nil {
			return output, errors.Wrap(
				err,
				"could not JSON marshal test result",
			)
		}

		testMap := make([]map[string]interface{}, 0)
		err = json.Unmarshal(bites, &testMap)

		var headers []string
		var values []string

		if len(testMap) == 0 {
			return output, nil
		}
		for attr := range testMap[0] {
			headers = append(headers, attr)
		}

		for _, m := range testMap {
			var currValues []string
			for _, attr := range headers {
				currValues = append(
					currValues,
					fmt.Sprintf("%v", m[attr]),
				)
			}
			values = append(values, strings.Join(currValues, ","))
		}

		headerRow := strings.Join(headers, ",")
		valueRow := strings.Join(values, "\n")

		output = fmt.Sprintf("%s\n%s", headerRow, valueRow)
	default:
		return output, errors.Errorf("invalid format: %s", format)
	}

	return output, nil
}

type endpointTest struct {
	client   *http.Client
	request  *http.Request
	response *http.Response

	elapsed time.Duration
	err     error
	index   int
	step    int
}

func newEndpointTest(client *http.Client, request *http.Request) *endpointTest {
	return &endpointTest{
		client:  client,
		request: request,
	}
}

func (et *endpointTest) execute(step int, index int) {
	start := time.Now()

	resp, err := et.client.Do(et.request)
	stop := time.Now()
	elapsed := stop.Sub(start)

	et.response = resp
	et.elapsed = elapsed
	et.err = err
	et.index = index
	et.step = step
}

func (et endpointTest) MarshalJSON() ([]byte, error) {
	testMap := map[string]interface{}{
		"method": et.request.Method,
		"url":    et.request.URL.String(),

		"status": et.response.StatusCode,

		"elapsed":             et.elapsed.String(),
		"nanoseconds_elapsed": et.elapsed,
		"error":               et.err,
		"index":               et.index,
		"step":                et.step,
	}

	return json.Marshal(testMap)
}

func newHTTPClient() *http.Client {
	netTransport := &http.Transport{
		Dial:                (&net.Dialer{Timeout: 5 * time.Second}).Dial,
		TLSHandshakeTimeout: 5 * time.Second,
	}

	client := &http.Client{
		Timeout:   time.Second * 10,
		Transport: netTransport,
	}

	return client
}

func newHTTPRequest(
	method,
	url,
	basicAuth string,
	headers map[string]string,
) *http.Request {
	req, err := http.NewRequest(strings.ToUpper(method), url, nil)
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

var (
	app     = kingpin.New("raven", "A command-line HTTP stress test application.")
	verbose = app.Flag("verbose", "Enable verbose mode").Short('v').Bool()
	headers = app.Flag("headers", "Specify HTTP headers").Short('h').StringMap()
	auth    = app.Flag(
		"authentication",
		"Provide a username:password",
	).Short('a').String()
	raw = app.Flag(
		"raw",
		"Output raw data in specified format",
	).Short('r').Enum(
		"json",
		"prettyjson",
		"csv",
	)

	version = app.Command("version", "Print running version of raven.")

	do       = app.Command("do", "Immediately send requests")
	doAmt    = do.Arg("amount", "Amount of requests to make").Required().Int()
	doMethod = do.Arg("method", "HTTP request method").Required().String()
	doURL    = do.Arg("url", "Target URL address").Required().URL()

	stress = app.Command(
		"stress",
		`Ramp up requests until responses slow`+
			`to within %x percent of a baseline`,
	)
	stressType = stress.Arg(
		"type",
		`Stress method type ("duration" or "status")`,
	).Required().Enum("duration", "status")
	stressMethod = stress.Arg(
		"method",
		"HTTP request method",
	).Required().String()
	stressURL       = stress.Arg("url", "Target URL address").Required().URL()
	stressThreshold = stress.Flag(
		"threshold",
		`%xx percent threshold for valid responses`,
	).Short('t').Default("10.0").Float()
	stressStart = stress.Flag(
		"start",
		"Provide a starting amount for concurrent requests",
	).Short('s').Default("1").Int()
	stressIterations = stress.Flag(
		"iterations",
		"Amount of iterations to perform for each step",
	).Short('i').Default("10").Int()
	stressDelay = stress.Flag(
		"delay",
		"Millisecond delay between iterations",
	).Short('d').Default("500").Int()
)

func parseMethod(method string) (string, error) {
	validMethods := map[string]struct{}{
		http.MethodGet:     {},
		http.MethodHead:    {},
		http.MethodPost:    {},
		http.MethodPut:     {},
		http.MethodPatch:   {},
		http.MethodDelete:  {},
		http.MethodConnect: {},
		http.MethodOptions: {},
		http.MethodTrace:   {},
	}

	capsMethod := strings.ToUpper(method)
	if _, ok := validMethods[capsMethod]; ok {
		return capsMethod, nil
	}

	return "", fmt.Errorf("invalid method: %v", method)
}

func performRequest(
	index int,
	client *http.Client,
	results chan result,
	wg *sync.WaitGroup,
	request *http.Request,
) {
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

func setupRequest(
	method,
	url string,
	headers map[string]string,
	basicAuth string,
) *http.Request {
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

func performDo(amount int, client *http.Client, reqFactory func() *http.Request) []*endpointTest {
	group := new(sync.WaitGroup)
	preparedTests := make([]*endpointTest, amount)

	for i := 0; i < amount; i++ {
		req := reqFactory()
		preparedTests[i] = newEndpointTest(client, req)
	}

	for i, t := range preparedTests {
		group.Add(1)
		go func(index int, test *endpointTest) {
			test.execute(0, index)
			group.Done()
		}(i, t)
	}

	group.Wait()
	return preparedTests
}

func getBaselineTests(amount int, client *http.Client, reqFactory func() *http.Request) []*endpointTest {
	printVerbose("getting", amount, "baseline tests")
	preparedTests := make([]*endpointTest, amount)

	for i := 0; i < amount; i++ {
		req := reqFactory()
		preparedTests[i] = newEndpointTest(client, req)
	}

	for index, test := range preparedTests {
		printVerbose("\texecuting baseline test", index)
		test.execute(0, index)
	}

	return preparedTests
}

func performStress(
	stopType string,
	startingStep int,
	threshold float64,
	stepIterations int,
	stepDelay time.Duration,
	client *http.Client,
	reqFactory func() *http.Request,
) []*endpointTest {
	// First, calculate baseline average
	baselineTests := getBaselineTests(stepIterations, client, reqFactory)
	baselineSum := time.Duration(0)
	for _, t := range baselineTests {
		baselineSum = baselineSum + t.elapsed
	}

	baselineAverage := time.Duration(
		float64(baselineSum) / float64(stepIterations),
	)

	// max acceptable elapse duration (if using "duration" type)
	maxAcceptableElapse := time.Duration(
		(1.00 + (threshold / 100.00)) * float64(baselineAverage),
	)

	maxAcceptableNon200s := int(
		(1.00 + (threshold / 100.00)) * float64(stepIterations),
	)

	printVerbose("average baseline duration", baselineAverage)
	printVerbose("max acceptable duration", maxAcceptableElapse)
	printVerbose("max acceptable non-200s", maxAcceptableNon200s)

	completedTests := make([]*endpointTest, 0)

	for step := startingStep; ; step++ {

		completedStepTests := make([]*endpointTest, 0)
		maxFailedElapseTests := int(
			((100.00 - threshold) / 100.00) * float64(stepIterations*step),
		)
		printVerbose("current max failed duration tests", maxFailedElapseTests)
		printVerbose("\tstarting step", step)

		for iteration := 0; iteration < stepIterations; iteration++ {
			printVerbose("\t\tstarting iteration", iteration)

			group := new(sync.WaitGroup)
			preparedTests := make([]*endpointTest, step)

			for i := 0; i < step; i++ {
				req := reqFactory()
				preparedTests[i] = newEndpointTest(client, req)
			}

			for i, t := range preparedTests {
				group.Add(1)
				printVerbose("\t\t\texecuting request", i+iteration+((step-1)*stepIterations))
				go func(index int, test *endpointTest) {
					test.execute(step, iteration+(step*stepIterations))
					group.Done()
				}(i, t)
			}

			group.Wait()
			completedStepTests = append(completedStepTests, preparedTests...)
			completedTests = append(completedTests, preparedTests...)
			time.Sleep(stepDelay)
		}

		failedElapsedTests := 0
		non200s := 0
		totalElapse := time.Duration(0)

		for _, test := range completedStepTests {
			totalElapse = totalElapse + test.elapsed
			if test.response != nil && test.response.StatusCode != 200 {
				non200s = non200s + 1
				printVerbose("\t\t\tNon-200s:", non200s)
			}

			if test.elapsed > maxAcceptableElapse {
				failedElapsedTests = failedElapsedTests + 1
				printVerbose("\t\t\tFailed elapsed tests:", failedElapsedTests)
			}
		}

		if stopType == "duration" && failedElapsedTests > maxFailedElapseTests {
			printVerbose("max duration exceeded by", failedElapsedTests)
			break
		} else if stopType == "status" && non200s > maxAcceptableNon200s {
			printVerbose("max non-200s exceeded by", non200s)
			break
		}
	}

	return completedTests
}

func printVerbose(items ...interface{}) {
	if *verbose {
		fmt.Println(items...)
	}
}

func main() {
	cmd := kingpin.MustParse(app.Parse(os.Args[1:]))

	if *verbose && len(*raw) > 0 {
		app.Fatalf(os.Stderr, "cannot user 'verbose' and 'raw' mode at same time")
	}

	c := newHTTPClient()

	switch cmd {
	case do.FullCommand():
		factory := func() *http.Request {
			return newHTTPRequest(*doMethod, (*doURL).String(), *auth, *headers)
		}
		tests := performDo(*doAmt, c, factory)

		if len(*raw) > 0 {
			out, err := marshalTest(*raw, tests)
			if err != nil {
				panic(err)
			}

			fmt.Println(string(out))
		} else {
			elapsedSum := time.Duration(0)
			maxElapsed := time.Duration(0)
			minElapsed := time.Duration(0)
			errored := time.Duration(0)

			statuses := make(map[int]int)
			for _, test := range tests {
				elapsedSum = elapsedSum + test.elapsed
				if test.elapsed > maxElapsed {
					maxElapsed = test.elapsed
				}
				if test.elapsed < minElapsed || minElapsed == 0 {
					minElapsed = test.elapsed
				}

				if test.err != nil {
					errored = errored + 1
				} else {
					status := test.response.StatusCode
					if count, ok := statuses[status]; ok {
						statuses[status] = count + 1
					} else {
						statuses[status] = 1
					}
				}
			}

			avgElapsed := time.Duration(float64(elapsedSum) / float64(*doAmt))
			fmt.Println("Total requests:", *doAmt)
			fmt.Println("Errored requests:", errored)
			fmt.Println("Max elapsed:", maxElapsed)
			fmt.Println("Min elapsed:", minElapsed)
			fmt.Println("Avg elapsed:", avgElapsed)

			if len(statuses) > 0 {
				fmt.Println("\nStatus Code counts:")
				for code, amount := range statuses {
					fmt.Println("\tHTTP", code, "-", amount)
				}
			}
		}
	case stress.FullCommand():
		factory := func() *http.Request {
			return newHTTPRequest(*stressMethod, (*stressURL).String(), *auth, *headers)
		}

		tests := performStress(
			*stressType,
			*stressStart,
			*stressThreshold,
			*stressIterations,
			time.Duration(*stressDelay)*time.Millisecond,
			c,
			factory,
		)

		if len(*raw) > 0 {
			out, err := marshalTest(*raw, tests)
			if err != nil {
				panic(err)
			}

			fmt.Println(string(out))
		} else {
			elapsedSum := time.Duration(0)
			maxElapsed := time.Duration(0)
			minElapsed := time.Duration(0)
			errored := 0

			statuses := make(map[int]int)
			testsByStep := make(map[int][]*endpointTest)
			maxStep := 0

			for _, test := range tests {
				if _, ok := testsByStep[test.step]; ok {
					testsByStep[test.step] = append(testsByStep[test.step], test)
				} else {
					testsByStep[test.step] = []*endpointTest{test}
				}

				if test.step > maxStep {
					maxStep = test.step
				}

				elapsedSum = elapsedSum + test.elapsed
				if test.elapsed > maxElapsed {
					maxElapsed = test.elapsed
				}
				if test.elapsed < minElapsed || minElapsed == 0 {
					minElapsed = test.elapsed
				}

				if test.err != nil {
					errored = errored + 1
				} else {
					status := test.response.StatusCode
					if count, ok := statuses[status]; ok {
						statuses[status] = count + 1
					} else {
						statuses[status] = 1
					}
				}
			}

			avgElapsed := time.Duration(float64(elapsedSum) / float64(len(tests)))
			fmt.Println("Total requests:", len(tests))
			fmt.Println("Errored requests:", errored)
			fmt.Println("Max elapsed:", maxElapsed)
			fmt.Println("Min elapsed:", minElapsed)
			fmt.Println("Avg elapsed:", avgElapsed)

			fmt.Println("Max step reached:", maxStep)

			if len(statuses) > 0 {
				fmt.Println("\nStatus Code counts:")
				for code, amount := range statuses {
					fmt.Println("\tHTTP", code, "-", amount)
				}
			}

			fmt.Println("\nStep Breakdown")
			for step := *stressStart; step < *stressStart+len(testsByStep); step++ {
				stepTests := testsByStep[step]

				sum := time.Duration(0)
				max := time.Duration(0)
				min := time.Duration(0)

				for _, t := range stepTests {
					sum = sum + t.elapsed
					if t.elapsed > max {
						max = t.elapsed
					}
					if t.elapsed < min || min == 0 {
						min = t.elapsed
					}
				}

				avg := time.Duration(float64(sum) / float64(len(tests)))
				fmt.Println("\n\tInformation for Step", step)
				fmt.Println("\t\tMax elapsed:", max)
				fmt.Println("\t\tMin elapsed:", min)
				fmt.Println("\t\tAvg elapsed:", avg)

			}
		}

	case version.FullCommand():
		fmt.Println("raven", getVersion())
	default:
		app.Usage(os.Stdout)
	}
}
