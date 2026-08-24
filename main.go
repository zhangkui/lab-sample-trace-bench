package main

import (
	"github.com/zhangkui/lab-sample-trace-bench/internal/api"
	"github.com/zhangkui/lab-sample-trace-bench/internal/service"
	"github.com/zhangkui/lab-sample-trace-bench/internal/store"
	"log"
	"net/http"
	"os"
)

func main() {
	path := os.Getenv("LAB_SAMPLE_TRACE_BENCH_DB")
	if path == "" {
		path = "data/lab-sample-trace-bench.db"
	}
	repo, err := store.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer repo.Close()
	app := service.NewSampleService(repo)
	addr := os.Getenv("LAB_SAMPLE_TRACE_BENCH_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	log.Printf("lab-sample-trace-bench listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, api.New(app).Handler()))
}
