package main

import (
	"log"
	"net/http"
	"os"

	"github.com/zhangkui/lab-sample-trace-bench/internal/handler"
	"github.com/zhangkui/lab-sample-trace-bench/internal/service"
	"github.com/zhangkui/lab-sample-trace-bench/internal/store"
)

func main() {
	path := os.Getenv("LAB_SAMPLE_TRACE_BENCH_DB")
	if path == "" {
		path = "data/lab-sample-trace-bench.db"
	}
	repository, err := store.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer repository.Close()
	app := service.NewLab(repository)
	defer app.Close()
	addr := os.Getenv("LAB_SAMPLE_TRACE_BENCH_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	log.Printf("lab-sample-trace-bench listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, handler.New(app)))
}
