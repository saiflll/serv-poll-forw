package forwarder

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"IoTT/internal/models"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/gofiber/fiber/v2"
)

// --- Configuration ---
const (
	aggregationInterval = 8 * time.Minute
	maxBufferSize       = 50 * 1024 // 50 KB
)

// ForwarderStatus menampung data untuk ditampilkan di dashboard.
type ForwarderStatus struct {
	LastForwardTime    time.Time
	NextForwardTime    time.Time
	BufferSize         int
	BufferItemCount    int
	LastForwardStatus  string
	LastForwardError   string
	ReceivedDataBuffer []models.AreaData
}

var (
	buffer           []models.AreaData
	bufferMutex      = &sync.Mutex{}
	publicMqttClient mqtt.Client
	status           = ForwarderStatus{
		LastForwardStatus: "Belum ada",
	}
	statusMutex = &sync.Mutex{}
	ticker      *time.Ticker
)

// Start initializes the forwarder component.
func Start() {
	setupPublicMQTT()
	ticker = time.NewTicker(aggregationInterval)

	statusMutex.Lock()
	status.NextForwardTime = time.Now().Add(aggregationInterval)
	statusMutex.Unlock()

	go func() {
		for {
			<-ticker.C
			log.Println("⏰ (Forwarder) Timer 8 menit tercapai, memicu proses rekap data.")
			flushBufferIfNecessary(true) // Force flush on timer
		}
	}()

	log.Println("✅ Forwarder worker started. Will aggregate data and forward to public EMQX.")
}

func AddToBufferAndAggregate(data []models.AreaData) {
	bufferMutex.Lock()
	buffer = append(buffer, data...)
	bufferMutex.Unlock()

	// Update status for dashboard display
	statusMutex.Lock()
	status.ReceivedDataBuffer = append(status.ReceivedDataBuffer, data...)
	if len(status.ReceivedDataBuffer) > 100 { // Keep only last 100 items for display
		status.ReceivedDataBuffer = status.ReceivedDataBuffer[len(status.ReceivedDataBuffer)-100:]
	}
	statusMutex.Unlock()

	flushBufferIfNecessary(false) // Check if flush is needed due to size
}

func flushBufferIfNecessary(force bool) {
	bufferMutex.Lock()
	defer bufferMutex.Unlock()

	currentSize := 0
	if len(buffer) > 0 {
		jsonData, _ := json.Marshal(buffer)
		currentSize = len(jsonData)
	}

	// Flush if forced, time is up, or size limit is reached
	if len(buffer) > 0 && (force || time.Now().After(status.NextForwardTime) || currentSize >= maxBufferSize) {
		log.Printf("Flushing buffer. Items: %d, Size: %d, Forced: %v", len(buffer), currentSize, force)
		forwardData(buffer)
		buffer = []models.AreaData{} // Clear buffer

		statusMutex.Lock()
		status.NextForwardTime = time.Now().Add(aggregationInterval)
		status.ReceivedDataBuffer = []models.AreaData{} // Clear display buffer after forward
		ticker.Reset(aggregationInterval)               // Reset timer
		statusMutex.Unlock()
	}

	// Always update buffer size and count for the dashboard
	statusMutex.Lock()
	status.BufferSize = currentSize
	status.BufferItemCount = len(buffer)
	statusMutex.Unlock()
}

func forwardData(dataToForward []models.AreaData) {
	statusMutex.Lock()
	status.LastForwardTime = time.Now()
	statusMutex.Unlock()

	if publicMqttClient == nil || !publicMqttClient.IsConnected() {
		log.Println("Error: Public MQTT client not connected. Skipping forward.")
		statusMutex.Lock()
		status.LastForwardStatus = "Gagal"
		status.LastForwardError = "Client tidak terhubung"
		statusMutex.Unlock()
		return
	}

	topic := os.Getenv("MQTT_PUBLIC_TOPIC")
	if topic == "" {
		topic = "iot/ck3/data/aggregated" // Default topic
	}

	payload, err := json.Marshal(dataToForward)
	if err != nil {
		log.Printf("Error marshalling data for forwarding: %v", err)
		statusMutex.Lock()
		status.LastForwardStatus = "Gagal"
		status.LastForwardError = fmt.Sprintf("JSON Marshal Error: %v", err)
		statusMutex.Unlock()
		return
	}

	token := publicMqttClient.Publish(topic, 1, false, payload)
	if token.WaitTimeout(5*time.Second) && token.Error() != nil {
		log.Printf("Error forwarding data to public MQTT: %v", token.Error())
		statusMutex.Lock()
		status.LastForwardStatus = "Gagal"
		status.LastForwardError = fmt.Sprintf("Publish Error: %v", token.Error())
		statusMutex.Unlock()
	} else {
		log.Printf("Successfully forwarded %d data points to topic %s", len(dataToForward), topic)
		statusMutex.Lock()
		status.LastForwardStatus = "Sukses"
		status.LastForwardError = ""
		statusMutex.Unlock()
	}
}

func setupPublicMQTT() {
	broker := os.Getenv("MQTT_PUBLIC_BROKER_URI")
	if broker == "" {
		log.Println("Warning: MQTT_PUBLIC_BROKER_URI not set. Forwarder will not work.")
		return
	}
	clientID := fmt.Sprintf("servfi-forwarder-%d", time.Now().UnixNano())

	opts := mqtt.NewClientOptions()
	opts.AddBroker(broker)
	opts.SetClientID(clientID)
	opts.OnConnect = func(c mqtt.Client) { log.Println("✅ Forwarder connected to Public MQTT Broker.") }
	opts.OnConnectionLost = func(c mqtt.Client, err error) { log.Printf("⚠️ Forwarder connection to Public MQTT lost: %v", err) }

	publicMqttClient = mqtt.NewClient(opts)
	if token := publicMqttClient.Connect(); token.Wait() && token.Error() != nil {
		log.Printf("❌ Failed to connect forwarder to public MQTT broker: %v", token.Error())
	}
}

// RegisterForwarderHandlers mendaftarkan rute HTTP untuk dashboard.
func RegisterForwarderHandlers(app *fiber.App) {
	// Rute untuk halaman utama dashboard
	app.Get("/forwarder", func(c *fiber.Ctx) error {
		return c.Render("index", fiber.Map{
			"Title": "Forwarder Status",
		})
	})

	// Rute untuk API status
	app.Get("/forwarder/status", func(c *fiber.Ctx) error {
		statusMutex.Lock()
		defer statusMutex.Unlock()
		return c.Status(http.StatusOK).JSON(status)
	})
}
