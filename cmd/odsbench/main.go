package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

var requests = [][]byte{
	[]byte(`{"model":"jev-latest","state":"Please help with my refund.","questions":{"urgent":{"type":"noul","instructions":"Does this need immediate attention?"}}}`),
	[]byte(`{"model":"jev-latest","state":{"message":"Payment failed twice","customer":"enterprise"},"questions":{"route":{"type":"choice","instructions":"Which team?","criteria":{"billing":"Payments and invoices","support":"Account help","sales":"Purchases"}},"urgency":{"type":"score","instructions":"How urgent?","criteria":["Can wait","Needs attention","Blocking"]}}}`),
	[]byte(`{"model":"jev-latest","state":"A customer reports that the account password reset link expired. They are unable to sign in and have a time-sensitive meeting. The customer has contacted support twice and is requesting immediate assistance.","questions":{"interrupt":{"type":"noul","instructions":"Should the voice agent interrupt its current speech?","criteria":{"false":"Continue speaking","true":"Stop speaking and listen"}},"route":{"type":"choice","instructions":"Choose the best action","criteria":{"reset":"Send a fresh reset link","billing":"Investigate charges","human":"Escalate to a support agent","ignore":"No action required"}}}}`),
}

type sample struct {
	latency time.Duration
	status  int
	err     error
}

func main() {
	url := flag.String("url", "http://127.0.0.1:8080/v1/systemone", "System One endpoint")
	clients := flag.Int("clients", 64, "concurrent clients")
	duration := flag.Duration("duration", 10*time.Minute, "load duration")
	timeout := flag.Duration("timeout", 30*time.Second, "per request timeout")
	flag.Parse()
	if *clients < 1 || *duration <= 0 {
		fmt.Fprintln(os.Stderr, "invalid load settings")
		os.Exit(2)
	}
	transport := &http.Transport{MaxIdleConns: *clients, MaxIdleConnsPerHost: *clients, MaxConnsPerHost: *clients}
	client := &http.Client{Transport: transport, Timeout: *timeout}
	ctx, cancel := context.WithTimeout(context.Background(), *duration)
	defer cancel()
	start := time.Now()
	results := make(chan sample, 1024)
	var wg sync.WaitGroup
	var sequence atomic.Uint64
	for i := 0; i < *clients; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				n := sequence.Add(1)
				body := requests[int(n)%len(requests)]
				req, err := http.NewRequestWithContext(ctx, http.MethodPost, *url, bytes.NewReader(body))
				if err != nil {
					results <- sample{err: err}
					return
				}
				req.Header.Set("Content-Type", "application/json")
				started := time.Now()
				resp, err := client.Do(req)
				s := sample{latency: time.Since(started), err: err}
				if err == nil {
					s.status = resp.StatusCode
					data, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
					_ = resp.Body.Close()
					if readErr != nil {
						s.err = readErr
					} else if resp.StatusCode == 200 {
						s.err = validate(body, data)
					}
				}
				if ctx.Err() != nil {
					return
				}
				results <- s
			}
		}()
	}
	go func() { wg.Wait(); close(results) }()
	counts := map[int]int{}
	var latencies []time.Duration
	errors := 0
	for result := range results {
		counts[result.status]++
		if result.err != nil {
			errors++
			if errors <= 5 {
				fmt.Fprintln(os.Stderr, "request error:", result.err)
			}
		}
		if result.status == 200 && result.err == nil {
			latencies = append(latencies, result.latency)
		}
	}
	elapsed := time.Since(start)
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	percentile := func(p float64) time.Duration {
		if len(latencies) == 0 {
			return 0
		}
		index := int(p * float64(len(latencies)-1))
		return latencies[index]
	}
	fmt.Printf("elapsed=%s clients=%d total=%d success=%d errors=%d status=%v qps=%.1f p50=%s p95=%s p99=%s\n",
		elapsed.Round(time.Millisecond), *clients, sum(counts), len(latencies), errors, counts,
		float64(len(latencies))/elapsed.Seconds(), percentile(.50), percentile(.95), percentile(.99))
	if errors > 0 || counts[200] != sum(counts) || counts[200] == 0 {
		os.Exit(1)
	}
}

func sum(counts map[int]int) int {
	n := 0
	for _, v := range counts {
		n += v
	}
	return n
}

func validate(request, response []byte) error {
	var asked struct {
		Questions map[string]json.RawMessage `json:"questions"`
	}
	var answered struct {
		Model   string `json:"model"`
		Answers map[string]struct {
			Type string `json:"type"`
		} `json:"answers"`
		Usage struct {
			InputTokens int `json:"input_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(request, &asked); err != nil {
		return err
	}
	if err := json.Unmarshal(response, &answered); err != nil {
		return err
	}
	if answered.Model != "julia-1" || len(answered.Answers) != len(asked.Questions) || answered.Usage.InputTokens < 1 {
		return fmt.Errorf("invalid response: %s", response)
	}
	for key, question := range asked.Questions {
		var kind struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal(question, &kind)
		if got, ok := answered.Answers[key]; !ok || got.Type != kind.Type {
			return fmt.Errorf("answer mismatch for %s", key)
		}
	}
	return nil
}
