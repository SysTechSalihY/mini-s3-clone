package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/SysTechSalihY/mini-s3-clone/db"
	"github.com/SysTechSalihY/mini-s3-clone/worker"
	"github.com/hibiken/asynq"
)

func main() {
	// Connect to DB
	if err := db.ConnectDb(); err != nil {
		log.Fatal("DB connection failed:", err)
	}

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	srv := asynq.NewServer(
		asynq.RedisClientOpt{Addr: redisAddr},
		asynq.Config{
			Concurrency: 10,
		},
	)

	newWorker := &worker.Worker{DB: db.DB}

	mux := asynq.NewServeMux()
	mux.HandleFunc("empty_bucket", newWorker.HandleEmptyBucketTask)
	mux.HandleFunc("copy_bucket", newWorker.HandleCopyBucketTask)

	// Graceful shutdown
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Println("Starting Asynq worker...")
		if err := srv.Start(mux); err != nil {
			log.Fatal(err)
		}
	}()

	<-done
	log.Println("Shutting down worker...")
	srv.Shutdown()
}
