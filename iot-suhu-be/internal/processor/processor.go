package processor

import (
	"IoTT/internal/config"
	"IoTT/internal/database"
	"IoTT/internal/models"
	"IoTT/internal/telegram"
	timed "IoTT/internal/time"
	"IoTT/internal/worker"
	"fmt"
	"log"
	"time"
)

// ProcessSensorData meng-enkapsulasi logika untuk memproses dan mengevaluasi data sensor.
// Data yang valid akan dikumpulkan dan ditambahkan ke buffer worker untuk diproses secara batch.
func ProcessSensorData(data []models.AreaData) (int, error) {
	processedItemCount := 0
	var combinedError error

	// Slices to collect data for a single batch call to the worker
	var tempsToBatch []worker.TempBatchData
	var rhsToBatch []worker.RhBatchData
	var proxsToBatch []worker.ProxBatchData

	for _, areaData := range data {
		areaID := areaData.Area

		for _, tempData := range areaData.Temp {
			var parsedTS time.Time
			if tempData.Ts == "" || tempData.Ts == " " {
				parsedTS = timed.Now().In(config.Timezone)
			} else {
				var err error
				parsedTS, err = time.Parse(time.RFC3339Nano, tempData.Ts)
				if err != nil {
					log.Printf("invalid timestamp format for area %d: '%s', error: %v", areaID, tempData.Ts, err)
					if combinedError == nil {
						combinedError = fmt.Errorf("invalid timestamp format for area %d: '%s'", areaID, tempData.Ts)
					} else {
						combinedError = fmt.Errorf("%v; invalid timestamp format for area %d: '%s'", combinedError, areaID, tempData.Ts)
					}
				continue
			}
			parsedTS = parsedTS.In(config.Timezone)
		}
		tsString := parsedTS.Format(time.RFC3339Nano)

		// Validasi: Hanya proses sensor suhu yang terdaftar di config
		if !config.IsSensorRegistered("temp", areaID, tempData.No) {
			log.Printf("Peringatan: Menerima data untuk sensor suhu tidak terdaftar (Area: %d, No: %d). Data diabaikan.", areaID, tempData.No)
			continue // Lanjut ke data sensor berikutnya
		}

		// Kumpulkan data suhu untuk di-batch
		tempsToBatch = append(tempsToBatch, worker.TempBatchData{Value: tempData.Temp, AreaID: areaID, No: tempData.No, TS: tsString})

		sensorKeyTemp := fmt.Sprintf("temp-%d-%d", areaID, tempData.No)
		models.RegisterOrUpdateSensorStatus(sensorKeyTemp, "temp", areaID, tempData.No, 0, parsedTS, tempData.Temp)
		processedItemCount++

		// Evaluasi data suhu
		safetyStatus := worker.EvaluateTemp(areaID, tempData.No, tempData.Temp)
		if safetyStatus.IsAlert && safetyStatus.Message != "" {
			telegram.SendAlert("🔥 **Alert Temperature**\n" + safetyStatus.Message)
		}

		if tempData.RH != nil {
			// Validasi: Hanya proses sensor RH yang terdaftar di config
			if config.IsSensorRegistered("rh", areaID, tempData.No) {
				// Kumpulkan data RH untuk di-batch
				rhsToBatch = append(rhsToBatch, worker.RhBatchData{Value: *tempData.RH, AreaID: areaID, No: tempData.No, TS: tsString})

				sensorKeyRH := fmt.Sprintf("rh-%d-%d", areaID, tempData.No)
				models.RegisterOrUpdateSensorStatus(sensorKeyRH, "rh", areaID, tempData.No, 0, parsedTS, *tempData.RH)
				processedItemCount++

				// Evaluasi data kelembaban
				rhSafetyStatus := worker.EvaluateRh(areaID, tempData.No, *tempData.RH)
				if rhSafetyStatus.IsAlert && rhSafetyStatus.Message != "" {
					telegram.SendAlert("💧 **Alert Humidity**\n" + rhSafetyStatus.Message)
				}
			} else {
				log.Printf("Peringatan: Menerima data untuk sensor kelembaban tidak terdaftar (Area: %d, No: %d). Data diabaikan.", areaID, tempData.No)
			}
		}
		}

		for _, doorData := range areaData.Door {
			// Validasi: Hanya proses pintu yang terdaftar di database
			if !database.IsDoorRegistered(doorData.DoorID) {
				log.Printf("Peringatan: Menerima data untuk pintu tidak terdaftar (DoorID: %d). Data diabaikan.", doorData.DoorID)
				continue
			}

			ts := timed.Now().In(config.Timezone)
			if len(areaData.Temp) > 0 && areaData.Temp[0].Ts != "" {
				if parsedTS, err := time.Parse(time.RFC3339Nano, areaData.Temp[0].Ts); err == nil {
					ts = parsedTS.In(config.Timezone)
				}
			}
			tsString := ts.Format(time.RFC3339Nano)

			// Kumpulkan data proximity untuk di-batch
			proxsToBatch = append(proxsToBatch, worker.ProxBatchData{Value: doorData.Value, DoorID: doorData.DoorID, TS: tsString})

			sensorKeyProx := fmt.Sprintf("prox-%d", doorData.DoorID)
			models.RegisterOrUpdateSensorStatus(sensorKeyProx, "prox", areaID, 0, doorData.DoorID, ts, float64(doorData.Value))
			processedItemCount++
		}
	}

	// Setelah semua data diproses, kirimkan batch ke worker
	if len(tempsToBatch) > 0 || len(rhsToBatch) > 0 || len(proxsToBatch) > 0 {
		worker.AddToBuffer(tempsToBatch, rhsToBatch, proxsToBatch)
	}

	return processedItemCount, combinedError
}