package main

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

var (
	globalSpamSemaphore  = make(chan struct{}, HasSpamMaxAsyncRequests)
	globalSpamMutex      sync.Mutex
	semaphoreInitialized bool
)

func RunPipeline(cmds ...cmd) {
	// создаем каналы между командами
	in := make(chan interface{})

	wg := &sync.WaitGroup{}
	var lastOut chan interface{}

	for _, command := range cmds {
		wg.Add(1)
		out := make(chan interface{})

		// запускаем команду в горутине
		go func(cmd cmd, inCh, outCh chan interface{}) {
			defer wg.Done()
			defer close(outCh)
			cmd(inCh, outCh)
		}(command, in, out)

		// выходной канал этой команды становится входным для следующей
		in = out
		lastOut = out
	}

	// ждем завершения последней команды
	// нужно прочитать из последнего канала, чтобы не было дедлока
	go func() {
		for range lastOut {
		}
	}()

	wg.Wait()
}

func SelectUsers(in, out chan interface{}) {
	// 	in - string
	// 	out - User
	var wg = &sync.WaitGroup{}
	var mtx = &sync.RWMutex{}
	processed := make(map[string]bool)
	for item := range in {

		email, ok := item.(string)
		if !ok {
			fmt.Println("Ошибка! Не строка")
			continue
		}

		wg.Add(1)
		go func(em string) {
			defer wg.Done()

			res := GetUser(em)
			mtx.Lock()
			if processed[res.Email] {
				mtx.Unlock()
				return
			}
			processed[res.Email] = true
			mtx.Unlock()

			out <- res
		}(email)
	}

	wg.Wait()
}

func SelectMessages(in, out chan interface{}) {

	var wg sync.WaitGroup
	batch := make([]User, 0, 2)

	processBatch := func(b []User) {
		wg.Add(1)
		go func(curBatch []User) {
			defer wg.Done()
			res, err := GetMessages(curBatch...)
			if err != nil {
				for _, u := range curBatch {
					singleRes, err2 := GetMessages(u)
					if err2 == nil {
						for _, msgID := range singleRes {
							out <- msgID
						}
					}
				}
				return
			}
			for _, msgID := range res {
				out <- msgID
			}
		}(append([]User{}, b...)) // копия батча чтобы не было переиспользование
	}

	for item := range in {
		usr, ok := item.(User)
		if !ok {
			continue
		}

		batch = append(batch, usr)

		if len(batch) == 2 {
			processBatch(batch)
			batch = batch[:0] // очищаем после создания копии
		}
	}

	// Обрабатываем остатки
	if len(batch) > 0 {
		processBatch(batch)
	}

	wg.Wait()
}

func CheckSpam(in, out chan interface{}) {
	// in - MsgID
	// out - MsgData
	wg := &sync.WaitGroup{}
	// Инициализируем глобальный семафор один раз
	globalSpamMutex.Lock()
	if !semaphoreInitialized {
		globalSpamSemaphore = make(chan struct{}, HasSpamMaxAsyncRequests)
		semaphoreInitialized = true
	}
	globalSpamMutex.Unlock()
	for item := range in {
		msg, ok := item.(MsgID)
		if !ok {
			continue
		}

		wg.Add(1)
		go func(msgg MsgID) {
			defer wg.Done()
			globalSpamSemaphore <- struct{}{}
			defer func() { <-globalSpamSemaphore }()

			var isSpam bool
			var err error

			// пробуем несколько раз с экспоненциальной задержкой
			for attempt := 0; attempt < 3; attempt++ {
				isSpam, err = HasSpam(msgg)
				if err == nil {
					break
				}

				// если ошибка too many requests то ждем и пробуем снова
				if err.Error() == "too many requests" {
					// экспоненциальная задержка 50ms, 100ms, 200ms
					backoff := time.Duration(50*(1<<attempt)) * time.Millisecond
					time.Sleep(backoff)
					continue
				}
				break
			}

			if err != nil {
				return
			}

			out <- MsgData{msgg, isSpam}
		}(msg)
	}

	wg.Wait()
}

func Bool2int(b bool) int {
	var i int
	if b {
		i = 1
	} else {
		i = 0
	}
	return i
}

func CombineResults(in, out chan interface{}) {
	// in - MsgData
	// out - string
	var allData []MsgData

	for item := range in {
		msgData, ok := item.(MsgData)
		if !ok {
			fmt.Println("Ошибка! Передан не тип MsgData в CombineResults")
			return
		}
		allData = append(allData, msgData)
	}

	sort.Slice(allData, func(i, j int) bool {
		a := allData[i]
		b := allData[j]

		if Bool2int(a.HasSpam) > Bool2int(b.HasSpam) {
			return true
		} else if Bool2int(a.HasSpam) < Bool2int(b.HasSpam) {
			return false
		}

		return a.ID <= b.ID
	})

	for _, data := range allData {
		out <- fmt.Sprintf("%v %v", data.HasSpam, data.ID)
	}
}
