package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/brianvoe/gofakeit/v7"
)

const (
	GENERATE_DELAY_TIME = time.Millisecond * 500
)

var (
	ErrWorkerAlreadyExist = errors.New("worker already exist")
	ErrWorkerNotExist     = errors.New("worker does not exist")
)

type worker struct {
	id         int
	workerDone chan struct{}
}

func (w *worker) run(ctx context.Context, in <-chan string, workerOutput io.Writer) {
loop:
	for {
		select {
		case <-ctx.Done():
			fmt.Printf("Worker [id %d]: cancelled\n", w.id)
			break loop
		case msg := <-in:
			fmt.Fprintf(workerOutput, "Worker [id %d]: %s\n", w.id, msg)
			// можно раскомментировать, чтобы эмулировать тяжелую задачу
			// time.Sleep(5 * time.Second)
		}
	}
	w.workerDone <- struct{}{}
	fmt.Printf("Worker [id %d]: stopped\n", w.id)
}

type workerWithCancel struct {
	worker *worker
	cancel context.CancelFunc
}

type WorkerPool struct {
	// По заданию надо реализовать примитивный workerpool, поэтому используем контекст для отмены
	workersQty int
	workers    map[int]workerWithCancel
}

func NewWorkerPool() *WorkerPool {
	return &WorkerPool{
		workers: make(map[int]workerWithCancel),
	}
}

func (p *WorkerPool) Add(in <-chan string, workerOutput io.Writer) {
	worker := &worker{
		id:         p.workersQty,
		workerDone: make(chan struct{}, 1),
	}

	ctx, cancel := context.WithCancel(context.Background())
	p.workers[p.workersQty] = workerWithCancel{
		worker: worker,
		cancel: cancel,
	}

	go worker.run(ctx, in, workerOutput)

	fmt.Printf("Workerpool added worker: %d\n", p.workersQty)
	p.workersQty++
}

func (p *WorkerPool) Delete(id int) error {
	// Проверяем, существует ли воркер с данным id
	cancel, ok := p.workers[id]
	if !ok {
		return ErrWorkerNotExist
	}

	cancel.cancel()
	<-cancel.worker.workerDone
	delete(p.workers, id)
	fmt.Printf("Workerpool deleted worker: %d\n", id)

	return nil
}

func (p *WorkerPool) Finish() {
	wg := &sync.WaitGroup{}
	for k := range p.workers {
		wg.Add(1)
		go func() {
			err := p.Delete(k)
			if err == nil {
				wg.Done()
			}
		}()
	}
	wg.Wait()
}

func main() {
	file, err := os.Create("log.txt")
	if err != nil {
		panic("can't create log file")
	}

	workerInput := make(chan string)
	ctxGenerate, generateCancel := context.WithCancel(context.Background())

	go func() {
	generate:
		for {
			select {
			case <-ctxGenerate.Done():
				break generate
			default:
				workerInput <- gofakeit.MinecraftAnimal()
			}
			time.Sleep(GENERATE_DELAY_TIME)
		}
	}()

	workerPool := NewWorkerPool()

	scanner := bufio.NewScanner(os.Stdin)

	fmt.Println("Доступные команды:")
	fmt.Println("Add - Добавить воркера")
	fmt.Println("Delete - Удалить воркера")
	fmt.Println("Exit - Завершить воркерпул")
	fmt.Println("Введите команду: ")

scan:
	for scanner.Scan() {
		switch strings.TrimSpace(strings.ToLower(scanner.Text())) {
		case "add":
			workerPool.Add(workerInput, file)
		case "delete":
			id := 0
			fmt.Println("Введите id воркера:")
			fmt.Scanf("%d", &id)
			err := workerPool.Delete(id)
			if err != nil {
				fmt.Println(fmt.Errorf("delete worker from workerpool: %w", err))
			}
		case "exit":
			generateCancel()
			workerPool.Finish()
			break scan
		default:
			fmt.Println("Неизвестная команда")
		}
		fmt.Println("Введите команду: ")
	}

	close(workerInput)
}
