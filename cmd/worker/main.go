package main

import (
	"log"
	"os"

	"github.com/SysTechSalihY/mini-s3-clone/db"
	"github.com/SysTechSalihY/mini-s3-clone/worker"
	"github.com/hibiken/asynq"
)

func main() {
	// Connect to DB
	if err := db.ConnectDb(); err != nil {
		log.Fatal("DB connection failed:", err)
	}

	// Redis address from env
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	// Create Asynq server
	srv := asynq.NewServer(
		asynq.RedisClientOpt{Addr: redisAddr},
		asynq.Config{
			Concurrency: 10,
		},
	)

	// Create worker instance
	newWorker := &worker.Worker{
		DB: db.DB,
	}

	// ServeMux: pass function references (do NOT call them)
	mux := asynq.NewServeMux()
	mux.HandleFunc("empty_bucket", newWorker.HandleEmptyBucketTask)
	mux.HandleFunc("copy_bucket", newWorker.HandleCopyBucketTask)

	log.Println("Starting Asynq worker...")
	if err := srv.Start(mux); err != nil {
		log.Fatal(err)
	}
}
