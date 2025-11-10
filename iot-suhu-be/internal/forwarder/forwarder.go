package forwarder

import (
	"IoTT/internal/models"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// --- Configuration ---
const (
	aggregationInterval = 20 * time.Minute
	sizeLimitBytes      = 10 * 1024 * 1024 // 10 MB
	forwardTopic        = "sensor/data/ingest"
)

// --- State Variables ---
var (
	// Aggregation buffer: groups data by Area ID before it's ready to be sent.
	aggregationBuffer = make(map[int]*models.AreaData)
	bufferMutex       = &sync.Mutex{}
	currentBufferSize int64

	// Forwarding queue: holds marshalled JSON payloads ready to be sent.
	forwardingQueue = make([][]byte, 0)
	queueMutex      = &sync.Mutex{}

	// MQTT Client for the public broker
	forwardClient mqtt.Client
	ticker        *time.Ticker

	// To prevent multiple triggers from running at the same time
	isProcessing = &sync.Mutex{}
)

// Start initializes the forwarder component.
func Start() {
	// --- Configuration for the external EMQX Broker ---
	// It is strongly recommended to move these to environment variables.
	broker := "tcp://emqx.miegacoan.id:1883"
	username := "saiful"
	password := "saiful123"
	clientID := fmt.Sprintf("iot-suhu-be-forwarder-%d", time.Now().UnixNano())

	opts := mqtt.NewClientOptions()
	opts.AddBroker(broker)
	opts.SetClientID(clientID)
	opts.SetUsername(username)
	opts.SetPassword(password)
	opts.SetAutoReconnect(true)
	opts.SetConnectRetry(true)
	opts.SetConnectRetryInterval(1 * time.Minute)

	opts.OnConnect = func(client mqtt.Client) {
		log.Println("✅ (Forwarder) Berhasil terhubung ke Broker EMQX Publik.")
		// When connected, immediately try to process the queue.
		go processForwardingQueue()
	}
	opts.OnConnectionLost = func(client mqtt.Client, err error) {
		log.Printf("⚠️ (Forwarder) Koneksi ke Broker EMQX Publik terputus: %v", err)
	}

	forwardClient = mqtt.NewClient(opts)
	if token := forwardClient.Connect(); token.Wait() && token.Error() != nil {
		log.Printf("❌ (Forwarder) Gagal terhubung ke Broker EMQX Publik saat startup. Data akan diantrekan. Error: %v", token.Error())
	}

	ticker = time.NewTicker(aggregationInterval)

	go func() {
		for range ticker.C {
			log.Println("⏰ (Forwarder) Timer 20 menit tercapai, memicu proses rekap data.")
			triggerForwarding()
		}
	}()

	log.Println("✅ Forwarder worker started. Will aggregate data and forward to public EMQX.")
}

// AddToBufferAndAggregate is called by the local MQTT handler.
func AddToBufferAndAggregate(data []models.AreaData) {
	bufferMutex.Lock()
	for _, areaData := range data {
		// Estimate size increase
		payload, _ := json.Marshal(areaData)
		currentBufferSize += int64(len(payload))

		// Aggregate data
		if existing, ok := aggregationBuffer[areaData.Area]; ok {
			existing.Temp = append(existing.Temp, areaData.Temp...)
			existing.Door = append(existing.Door, areaData.Door...)
		} else {
			// Need to make a copy to avoid pointer issues
			newData := areaData
			aggregationBuffer[areaData.Area] = &newData
		}
	}
	bufferMutex.Unlock()

	// Check if size limit is exceeded
	if currentBufferSize > sizeLimitBytes {
		log.Printf("📦 (Forwarder) Batas ukuran 250MB terlampaui, memicu proses rekap data.")
		go triggerForwarding()
	}
}

// triggerForwarding prepares the aggregated data and moves it to the sending queue.
func triggerForwarding() {
	isProcessing.Lock()
	defer isProcessing.Unlock()

	bufferMutex.Lock()
	if len(aggregationBuffer) == 0 {
		bufferMutex.Unlock()
		log.Println("Info (Forwarder): Pemicu rekap berjalan, namun buffer agregasi kosong.")
		return
	}

	// Create the final payload from the buffer
	finalPayload := make([]models.AreaData, 0, len(aggregationBuffer))
	for _, areaData := range aggregationBuffer {
		// Sort Temp data by sensor number, then by timestamp
		sort.SliceStable(areaData.Temp, func(i, j int) bool {
			if areaData.Temp[i].No != areaData.Temp[j].No {
				return areaData.Temp[i].No < areaData.Temp[j].No
			}
			return areaData.Temp[i].Ts < areaData.Temp[j].Ts
		})

		// De-duplicate door data, keeping the last entry for each door ID
		lastDoorData := make(map[int]models.DoorData)
		for _, door := range areaData.Door {
			lastDoorData[door.DoorID] = door
		}
		areaData.Door = make([]models.DoorData, 0, len(lastDoorData))
		for _, door := range lastDoorData {
			areaData.Door = append(areaData.Door, door)
		}

		finalPayload = append(finalPayload, *areaData)
	}

	// Reset the aggregation buffer
	aggregationBuffer = make(map[int]*models.AreaData)
	currentBufferSize = 0
	bufferMutex.Unlock()

	// Marshal the final aggregated payload
	marshaledPayload, err := json.Marshal(finalPayload)
	if err != nil {
		log.Printf("CRITICAL (Forwarder): Gagal marshal payload agregat. Data untuk siklus ini hilang! Error: %v", err)
		return
	}

	// Add the marshaled payload to the outgoing queue
	queueMutex.Lock()
	forwardingQueue = append(forwardingQueue, marshaledPayload)
	queueMutex.Unlock()

	log.Printf("📥 (Forwarder) Rekap data selesai. %d bytes ditambahkan ke antrean pengiriman.", len(marshaledPayload))

	// Reset the timer and try to send immediately
	ticker.Reset(aggregationInterval)
	go processForwardingQueue()
}

// processForwardingQueue tries to send all data in the queue.
func processForwardingQueue() {
	queueMutex.Lock()
	defer queueMutex.Unlock()

	if len(forwardingQueue) == 0 {
		return // Nothing to do
	}

	if !forwardClient.IsConnected() {
		log.Println("Info (Forwarder): Tidak dapat mengirim antrean, koneksi ke EMQX publik belum siap.")
		return
	}

	log.Printf("📤 (Forwarder) Memulai pengiriman %d paket data dari antrean...", len(forwardingQueue))

	// Send oldest first
	for i, payload := range forwardingQueue {
		token := forwardClient.Publish(forwardTopic, 1, false, payload)
		if token.WaitTimeout(10*time.Second) && token.Error() != nil {
			log.Printf("Error (Forwarder): Gagal mengirim paket %d dari antrean. Menghentikan proses antrean. Error: %v", i+1, token.Error())
			// Stop processing and leave remaining items in the queue for the next attempt
			return
		}
		log.Printf("  -> Paket %d (%d bytes) berhasil dikirim.", i+1, len(payload))
	}

	// If all were sent successfully, clear the queue
	log.Println("✅ (Forwarder) Semua data dalam antrean berhasil dikirim.")
	forwardingQueue = make([][]byte, 0)
}
