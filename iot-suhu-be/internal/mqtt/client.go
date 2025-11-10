package mqtt

import (
	"IoTT/internal/forwarder"
	"IoTT/internal/models"
	"IoTT/internal/processor"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

var client mqtt.Client

// SensorDataTopic akan diisi dari environment variable saat startup.
var SensorDataTopic string

var messageHandler mqtt.MessageHandler = func(client mqtt.Client, msg mqtt.Message) {
	log.Printf("📥 Pesan MQTT diterima dari topik: %s", msg.Topic())

	var receivedData []models.AreaData
	// Coba unmarshal sebagai array dulu
	err := json.Unmarshal(msg.Payload(), &receivedData)
	if err != nil {
		// Jika gagal, coba unmarshal sebagai objek tunggal
		log.Printf("Info: Gagal unmarshal sebagai array, mencoba sebagai objek tunggal. Error: %v", err)
		var singleData models.AreaData
		if err2 := json.Unmarshal(msg.Payload(), &singleData); err2 != nil {
			log.Printf("Error: Gagal unmarshal payload JSON dari MQTT (baik sebagai array maupun objek): %v", err2)
			return
		}
		// Jika berhasil, bungkus dalam slice
		receivedData = []models.AreaData{singleData}
	}

	log.Printf("Debug: Data setelah unmarshal: %+v", receivedData)

	if len(receivedData) == 0 {
		log.Println("Peringatan: Menerima payload MQTT kosong.")
		return
	}

	// Kirim data ke forwarder untuk agregasi
	forwarder.AddToBufferAndAggregate(receivedData)

	// Langsung panggil ProcessSensorData tanpa transaksi
	_, err = processor.ProcessSensorData(receivedData)
	if err != nil {
		log.Printf("Error selama pemrosesan data sensor dari MQTT: %v", err)
		// Error di sini kemungkinan besar adalah dari validasi atau parsing,
		// karena penyimpanan data sudah ditangani oleh worker.
	}
}

var connectHandler mqtt.OnConnectHandler = func(client mqtt.Client) {
	log.Println("✅ Berhasil terhubung ke MQTT Broker.")
	// Berlangganan ke topik setelah koneksi berhasil
	token := client.Subscribe(SensorDataTopic, 1, messageHandler)
	token.Wait()
	log.Printf("✔️ Berlangganan ke topik: %s", SensorDataTopic)
}

var connectionLostHandler mqtt.ConnectionLostHandler = func(client mqtt.Client, err error) {
	log.Printf("⚠️ Koneksi ke MQTT Broker terputus: %v", err)
}

func StartClient() {
	brokerURI := os.Getenv("MQTT_BROKER_URI")
	if brokerURI == "" {
		log.Println("Peringatan: MQTT_BROKER_URI tidak diatur. MQTT client tidak akan dimulai.")
		return
	}

	SensorDataTopic = os.Getenv("MQTT_TOPIC_INGEST")
	if SensorDataTopic == "" {
		log.Println("Peringatan: MQTT_TOPIC_INGEST tidak diatur. Menggunakan topik default 'sensor/data/ingest'.")
		SensorDataTopic = "sensor/data/ingest"
	}

	opts := mqtt.NewClientOptions()
	opts.AddBroker(brokerURI)
	opts.SetClientID(fmt.Sprintf("servfi-backend-%d", time.Now().UnixNano()))
	opts.SetDefaultPublishHandler(messageHandler)
	opts.OnConnect = connectHandler
	opts.OnConnectionLost = connectionLostHandler

	// Tambahkan kredensial jika tersedia di environment
	username := os.Getenv("MQTT_USERNAME")
	password := os.Getenv("MQTT_PASSWORD")
	if username != "" {
		opts.SetUsername(username)
		opts.SetPassword(password)
		log.Println("Menggunakan kredensial MQTT.")
	}

	client = mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.Fatalf("❌ Gagal terhubung ke MQTT Broker: %v", token.Error())
	}
}
