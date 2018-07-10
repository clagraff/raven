![](.github/logo.png)

[![GoDoc](https://godoc.org/github.com/clagraff/raven?status.svg)](https://godoc.org/github.com/clagraff/raven)
[![Go Report Card](http://goreportcard.com/badge/clagraff/raven)](http://goreportcard.com/report/clagraff/raven)

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

# Stress Testing
There are a couple ways to stress test an endpoint. You can:
* perform N concurrent requests _at once_
* ramp up the number of current requests until...
    * ... response times exceed percent threshold of average
    * ... number of non-200 responses exceeds a percent threshold

## `do` — perform requests at once
The `do` command is used to perform multiple requests against an endpoint all at once.

It takes three main arguments:
* `amount`: how many requests to generate
* `method`: the HTTP method to use
* `url`: the actual URL to hit

Here is an example:

```bash
$ raven do 50 get http://localhost
Total Requests:      50
Elapsed Duration:    54.222567ms
Setup duration:      183.107µs

Average Request Duration:  23.841312ms
Min Request Duration:      4.562712ms
Max Request Duration:      53.17683ms

Total Response Size (bytes):    561
Average Response Size (bytes):  11

Status Codes:
	HTTP 200:	50
```

## `stress` — stress an endpoint to hell...
The `stress` command is used to ramp-up concurrent requests to an endpoint until it breaks.
There are two `types` of available stress-testing:
1. `duration`, where the test stops when responses slow
2. `status`, where the test stops when responses result in too many non-200s.

These two types, `duration` and `status`, are defined below:

### Duration — test until response times slow
Ramp up concurrent requests sent until the respponse duration slows down.

```bash
$ raven stress duration get http://localhost:32768/
Step delay:                   500ns
Baseline response time:       4.306627ms
Percent threshold:            10.000000 percent
Max acceptable response time: 4.73729ms

Performing 1 concurrent requests...
Performing 2 concurrent requests...
9.971642ms exceeds 4.73729ms
```

### Status — test until too many non-200 responses
Ramp up concurrent requests sent until responses start returning non-200 HTTP status codes.

```bash
$ raven stress status get http://localhost:32768/
Step delay:                   500ns
Baseline response time:       1.321324947s
Percent threshold:            10.000000 percent

Performing 1 concurrent requests...
    Max acceptable non-200 amount: 1
	...performing iteration 0 of 10
	...performing iteration 1 of 10
	...performing iteration 2 of 10
	...performing iteration 3 of 10
	...performing iteration 4 of 10
	...performing iteration 5 of 10
	...performing iteration 6 of 10
	...performing iteration 7 of 10
	...performing iteration 8 of 10
	...performing iteration 9 of 10

	Average:  1.456330813s
	Min:      692.831102ms
	Max:      4.667752624s
	200s:     10
	Non200s:  0
	
# etc etc etc
```

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
