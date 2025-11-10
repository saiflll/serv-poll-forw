package worker

import (
	"IoTT/internal/database"
	"log"
	"sync"
	"time"
)

var (
	tempBuffer []TempBatchData
	rhBuffer   []RhBatchData
	proxBuffer []ProxBatchData
	bufferMux  sync.Mutex
)

func AddToBuffer(tempData []TempBatchData, rhData []RhBatchData, proxData []ProxBatchData) {
	bufferMux.Lock()
	defer bufferMux.Unlock()

	tempBuffer = append(tempBuffer, tempData...)
	rhBuffer = append(rhBuffer, rhData...)
	proxBuffer = append(proxBuffer, proxData...)
}

func StartPollingWorker() {
	ticker := time.NewTicker(5 * time.Minute)
	go func() {
		for {
			<-ticker.C
			flushBuffer()
		}
	}()
	log.Println("✅ Worker for polling and batch insert has been started.")
}

func flushBuffer() {
	bufferMux.Lock()

	if len(tempBuffer) == 0 && len(rhBuffer) == 0 && len(proxBuffer) == 0 {
		bufferMux.Unlock()
		return
	}

	tempData := tempBuffer
	rhData := rhBuffer
	proxData := proxBuffer

	tempBuffer = nil
	rhBuffer = nil
	proxBuffer = nil

	bufferMux.Unlock()

	tx, err := database.DB.Begin()
	if err != nil {
		log.Printf("Error starting transaction for batch insert: %v", err)
		// Optionally, re-add data to buffer
		AddToBuffer(tempData, rhData, proxData)
		return
	}

	var wg sync.WaitGroup
	errChan := make(chan error, 3)

	if len(tempData) > 0 {
		wg.Add(1)
		go func(d []TempBatchData) {
			defer wg.Done()
			if err := BatchInsertTemp(tx, d); err != nil {
				errChan <- err
			}
		}(tempData)
	}

	if len(rhData) > 0 {
		wg.Add(1)
		go func(d []RhBatchData) {
			defer wg.Done()
			if err := BatchInsertRh(tx, d); err != nil {
				errChan <- err
			}
		}(rhData)
	}

	if len(proxData) > 0 {
		wg.Add(1)
		go func(d []ProxBatchData) {
			defer wg.Done()
			if err := BatchInsertProx(tx, d); err != nil {
				errChan <- err
			}
		}(proxData)
	}

	wg.Wait()
	close(errChan)

	var insertErrors []error
	for err := range errChan {
		insertErrors = append(insertErrors, err)
	}

	if len(insertErrors) > 0 {
		tx.Rollback()
		log.Printf("Error during batch insert, transaction rolled back: %v", insertErrors)
		// Re-add data to buffer after rollback
		AddToBuffer(tempData, rhData, proxData)
		return
	}

	if err := tx.Commit(); err != nil {
		log.Printf("Error committing transaction for batch insert: %v", err)
		// Re-add data to buffer if commit fails
		AddToBuffer(tempData, rhData, proxData)
	} else {
		log.Printf("📦 Batch insert successful for %d temp, %d rh, and %d prox records.", len(tempData), len(rhData), len(proxData))
	}
}
