![](.github/logo.png)

[![GoDoc](https://godoc.org/github.com/clagraff/raven?status.svg)](https://godoc.org/github.com/clagraff/raven)
[![Go Report Card](http://goreportcard.com/badge/clagraff/raven)](http://goreportcard.com/report/clagraff/raven)
[![CircleCI](https://circleci.com/gh/clagraff/raven/tree/master.svg?style=svg)](https://circleci.com/gh/clagraff/raven/tree/master)

# Raven
Stress test the hell outta your API with this simple CLI tool.

# Get it!

This is a Go program. If you already have Golang installed, installing `raven`
is as simple as:

```bash
$ go install github.com/clagraff/raven
$ raven version
raven 1.0.0
```

## tl;dr

```bash
# Perform bulk requests at once...
$ raven do 50 get http://localhost

# Stress until response times slow...
$ raven stress duration get http://localhost/

# Stress until HTTP status codes are non-200s
$ raven stress status get http://localhost/
```


## Tutorial
### Perform a batch stress test
> Perform multiple requests against an endpoint all at once

```bash
$ raven do 50 get http://localhost:8000
Total requests: 50
Errored requests: 0
Max elapsed: 1.008389032s
Min elapsed: 3.059769ms
Avg elapsed: 145.083852ms

Status Code counts:
	HTTP 200 - 50
```


The command syntax is:
`raven [<app flags>] do <requests amount> <method> <full url>`

The `do` command takes no additional flags.

### Perform continious stress testing
> Ramp-up concurrent requests until responses slow or errors occur

```bash
$ raven stress duration get http://localhost:8000
Total requests: 30
Errored requests: 0
Max elapsed: 4.14156ms
Min elapsed: 1.307548ms
Avg elapsed: 3.078331ms
Max step reached: 2

Status Code counts:
	HTTP 200 - 30

Step Breakdown

	Information for Step 1
		Max elapsed: 3.009577ms
		Min elapsed: 1.307548ms
		Avg elapsed: 903.564µs

	Information for Step 2
		Max elapsed: 4.14156ms
		Min elapsed: 2.498984ms
		Avg elapsed: 2.174766ms
```

The command syntax is:

`raven [<app flags>] stress [<command flags>] <type> <method> <full url>`

There are two commands: `duration` and `status`.

* `duration` increases concurrent requests until response times slow
* `status` increases concurrent requests until too many non-`200` status codes occur

#### Steps, iterations, and flags
```bash
$ raven  stress -t 20 -s 2 -d 300 duration get http://localhost:8000
Total requests: 140
Errored requests: 0
Max elapsed: 3.912831ms
Min elapsed: 1.458454ms
Avg elapsed: 2.706491ms
Max step reached: 5

Status Code counts:
	HTTP 200 - 140

Step Breakdown

	Information for Step 2
		Max elapsed: 2.881158ms
		Min elapsed: 1.458454ms
		Avg elapsed: 345.847µs

	Information for Step 3
		Max elapsed: 3.014396ms
		Min elapsed: 1.90508ms
		Avg elapsed: 549.612µs

	Information for Step 4
		Max elapsed: 3.30094ms
		Min elapsed: 2.083939ms
		Avg elapsed: 734.38µs

	Information for Step 5
		Max elapsed: 3.912831ms
		Min elapsed: 2.255182ms
		Avg elapsed: 1.07665ms
```

Each `step` when using the `stress` command represents how many concurrent requests will be sent.

The `--iterations=10` flag determines how many iterations of requests will be performed for each step.

The `--start=1` flag determines the starting step.

The `--delay=500` flag determines how many milliseconds to delay between each iteration.

The `--threshold=10.0` flag is used to determine the percent of requests per step which can fail before the
test stops.

### (global) Basic Auth for stress tests
> Provide a username & password to enable Basic Auth for requests

```bash
$ raven -a "johndoe:hunter2" do 50 get http://localhost:8000
Total requests: 50
Errored requests: 0
Max elapsed: 1.010340952s
Min elapsed: 2.937627ms
Avg elapsed: 325.627831ms

Status Code counts:
	HTTP 200 - 50
```

Provide a `username:password` to the application flag `-a / --authentication`
and all requests will use those credentials via Basic Auth

**This flag is passed _before_ the command name**

The username and password will be `base64`-encoded and passed in the
`Authorization` header for you automatically.

### (global) Supply custom headers
> Provide customer headers for all requests

```bash
$ raven -h Accept=text/plain -h Cache-Control=no-cache do 50 get http://localhost:8000
Total requests: 50
Errored requests: 37ns
Max elapsed: 2.009915103s
Min elapsed: 3.246405ms
Avg elapsed: 205.514321ms

Status Code counts:
	HTTP 200 - 50
```

You can provide multiple `key=value` pairs to be supplied as additional
headers to requests using the `-h / --headers` flag.

**These flags are passed _before_ the command name**

### (global) Display raw data
> Display raw request/response data in the form of CSV or JSON

```bash
$ raven --raw csv do 5 get http://localhost:8000
nanoseconds_elapsed,status,step,url,elapsed,error,index,method
6.656024e+06,200,0,http://localhost:8000,6.656024ms,<nil>,0,GET
5.450617e+06,200,0,http://localhost:8000,5.450617ms,<nil>,1,GET
4.461675e+06,200,0,http://localhost:8000,4.461675ms,<nil>,2,GET
5.935396e+06,200,0,http://localhost:8000,5.935396ms,<nil>,3,GET
4.060925e+06,200,0,http://localhost:8000,4.060925ms,<nil>,4,GET
```

```bash
$ raven --raw json do 5 get http://localhost:8000
[{"elapsed":"6.345739ms","error":null,"index":0,"method":"GET","nanoseconds_elapsed":6345739,"status":200,"step":0,"url":"http://localhost:8000"},{"elapsed":"4.559865ms","error":null,"index":1,"method":"GET","nanoseconds_elapsed":4559865,"status":200,"step":0,"url":"http://localhost:8000"},{"elapsed":"5.676389ms","error":null,"index":2,"method":"GET","nanoseconds_elapsed":5676389,"status":200,"step":0,"url":"http://localhost:8000"},{"elapsed":"4.979091ms","error":null,"index":3,"method":"GET","nanoseconds_elapsed":4979091,"status":200,"step":0,"url":"http://localhost:8000"},{"elapsed":"3.899406ms","error":null,"index":4,"method":"GET","nanoseconds_elapsed":3899406,"status":200,"step":0,"url":"http://localhost:8000"}]
```

Instead of printing a human-readable result summary, you can have `raven`
output the raw captured data using the `-r / --raw` flag.

You must specify an output format:
* `csv` for command-separated values
* `json` for minimized JSON objects
* `prettyjson` for indented JSON objects

**This flag is passed _before_ the command name**

You **cannot** use both the `--raw <format>` and the `--verbose` flags at the
same time.

### (global) Verbose output
> See detailed information as tests are running by using `verbose` mode


```bash
$ raven -v do 5 get http://localhost:8000
	generating 5 requests
		performing request 0
		performing request 1
		performing request 2
		performing request 3
		performing request 4
	waiting requests to complete
Total requests: 5
Errored requests: 0s
Max elapsed: 6.886712ms
Min elapsed: 4.206239ms
Avg elapsed: 5.538624ms

Status Code counts:
	HTTP 200 - 5
```

You can enable verbose printing by supplying the `-v / --verbose` flag. Additional
information will be printed as the application runs.

**This flag is passed _before_ the command name**

You **cannot** use both the `--raw <format>` and the `--verbose` flags at the
same time.

# License
MIT License

Copyright (c) 2018 Curtis La Graff

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
