package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	chart "github.com/wcharczuk/go-chart"
	"github.com/wcharczuk/go-chart/drawing"
	kingpin "gopkg.in/alecthomas/kingpin.v1"
)

func getVersion() string {
	return "1.0.1-alpha"
}

func marshalTest(format string, tests []*endpointTest) (string, error) {
	var output string
	switch format {
	case "graph":
		makeGraph(tests)
		return output, nil
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
		if err != nil {
			return output, errors.Wrap(err, "could not unmarshal JSON")
		}

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
		return output, errors.Errorf(
			"invalid format: %s must be: csv,json,prettyjson,graph",
			format,
		)
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

	if err != nil {
		printVerbose("request error:", err)
	}
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
		Dial: (&net.Dialer{
			Timeout: time.Duration(*cutoff) * time.Second,
		}).Dial,
		TLSHandshakeTimeout: time.Duration(*cutoff) * time.Second,
	}

	client := &http.Client{
		Timeout:   time.Second * time.Duration(*cutoff),
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
		"graph",
	)
	cutoff = app.Flag(
		"cutoff",
		"Max seconds before hanging requests are terminated.",
	).Short('c').Default("10").Int()

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

func performDo(
	amount int,
	client *http.Client,
	reqFactory func() *http.Request,
) []*endpointTest {
	group := new(sync.WaitGroup)
	preparedTests := make([]*endpointTest, amount)

	printVerbose("\tgenerating", amount, "requests")
	for i := 0; i < amount; i++ {
		req := reqFactory()
		preparedTests[i] = newEndpointTest(client, req)
	}

	for i, t := range preparedTests {
		printVerbose("\t\tperforming request", i)
		group.Add(1)
		go func(index int, test *endpointTest) {
			test.execute(0, index)
			group.Done()
		}(i, t)
	}

	printVerbose("\twaiting requests to complete")
	group.Wait()
	return preparedTests
}

func getBaselineTests(
	amount int,
	client *http.Client,
	reqFactory func() *http.Request,
) []*endpointTest {
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
				printVerbose(
					"\t\t\texecuting request",
					i+iteration+((step-1)*stepIterations),
				)
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

func makeGraph(tests []*endpointTest) {
	continuous := chart.ContinuousSeries{
		Name: "Response Durations",
		Style: chart.Style{
			Show:        true,
			StrokeColor: chart.ColorBlue,
			FillColor:   drawing.ColorBlue.WithAlpha(64),
		},
		XValues: []float64{},
		YValues: []float64{},
	}

	erroredSeries := chart.ContinuousSeries{
		Name: "Errored Response Durations",
		Style: chart.Style{
			Show:        true,
			StrokeWidth: chart.Disabled,
			DotWidth:    5,
		},
		XValues: []float64{},
		YValues: []float64{},
	}

	for _, test := range tests {
		x := float64(
			test.index + (test.step * (*stressIterations)),
		)

		if test.err == nil {
			continuous.XValues = append(continuous.XValues, x)
			continuous.YValues = append(continuous.YValues, float64(test.elapsed))
		} else {
			erroredSeries.XValues = append(erroredSeries.XValues, x)
			erroredSeries.YValues = append(erroredSeries.YValues, float64(test.elapsed))
		}
	}

	graph := chart.Chart{
		XAxis: chart.XAxis{
			Name:      "Index",
			NameStyle: chart.StyleShow(),
			Style:     chart.StyleShow(),
			GridMajorStyle: chart.Style{
				Show:        true,
				StrokeColor: chart.ColorAlternateGray,
				StrokeWidth: 1.0,
			},
			Range: &chart.ContinuousRange{
				Min: 0.0,
				Max: float64(len(tests)),
			},
		},
		YAxis: chart.YAxis{
			Name:      "Duration",
			NameStyle: chart.StyleShow(),
			Style:     chart.StyleShow(),
			TickStyle: chart.Style{
				TextRotationDegrees: 45.0,
			},
			ValueFormatter: func(i interface{}) string {
				return fmt.Sprintf("%v", time.Duration(i.(float64)))
			},
		},
		Series: []chart.Series{continuous, erroredSeries},
		Width:  1920,
		Height: 1090,
		Background: chart.Style{
			Padding: chart.Box{
				Top: 50,
			},
		},
	}

	buff := new(bytes.Buffer)
	err := graph.Render(chart.PNG, buff)
	if err != nil {
		panic(err)
	}

	bites := buff.Bytes()

	writer := bufio.NewWriter(os.Stdout)
	if _, err = writer.Write(bites); err != nil {
		panic(err)
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

			fmt.Println(out)
		} else {
			elapsedSum := time.Duration(0)
			maxElapsed := time.Duration(0)
			minElapsed := time.Duration(0)
			errored := 0

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

			fmt.Println(out)
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
		fmt.Println(getVersion())
	default:
		app.Usage(os.Stdout)
	}
}
