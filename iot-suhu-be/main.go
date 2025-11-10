package main

import (
	"IoTT/internal/archiver"
	"IoTT/internal/config"
	"IoTT/internal/database"
	"IoTT/internal/forwarder"
	"IoTT/internal/models"
	"IoTT/internal/mqtt"
	internalrouter "IoTT/internal/router"
	"IoTT/internal/telegram"
	"log"
	"os"
	_ "time/tzdata" // Import untuk menyematkan database zona waktu

	"github.com/gofiber/fiber/v2"
	"github.com/joho/godotenv"
)

func main() {

	if err := godotenv.Load(); err != nil {
		log.Println("Peringatan: Gagal memuat file .env. Menggunakan environment variable sistem.")
	}

	// Inisialisasi zona waktu aplikasi ke Asia/Jakarta (UTC+7)
	config.InitTimezone()

	database.InitDB()
	if database.DB != nil {
		defer database.CloseDB()
	}

	telegram.LoadConfig()
	if err := telegram.InitBot(); err != nil {
		log.Printf("Peringatan: Gagal menginisialisasi bot Telegram: %v. Notifikasi mungkin tidak berfungsi.", err)
	}

	// Jalankan MQTT client di goroutine agar tidak memblokir server HTTP
	go mqtt.StartClient()

	// Memulai worker yang menjalankan pengecekan periodik (sensor offline, pintu terbuka, dll.)-->> sensor_status
	models.StartPeriodicCheckWorker()

	// Memulai worker untuk arsip data lama
	go archiver.Start()

	// Memulai worker untuk forwarder ke EMQX Publik
	go forwarder.Start()

	// Memuat ulang data lookup untuk memastikan semua data hasil seeding tersedia.
	log.Println("🔄 Memuat ulang data lookup (Area & Pintu)...")
	database.LoadLookupData()

	app := fiber.New()

	internalrouter.SetupInternalRouter(app)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8000"
	}
	log.Printf("Start server Fiber: %s", port)

	if err := app.Listen(":" + port); err != nil {
		log.Fatalf("X Gagal menjalankan server Fiber: %v", err)
	}
}
