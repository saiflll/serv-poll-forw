package database

import (
	"IoTT/internal/seed"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/lib/pq"
)

var DB *sql.DB
var (
	AreaNames     map[int]string
	DoorNames     map[int]string
	DoorToAreaMap map[int]int
)

func InitDB() {
	var err error
	retries := 5
	for i := 0; i < retries; i++ {
		connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
			os.Getenv("DB_HOST"),
			os.Getenv("DB_PORT"),
			os.Getenv("DB_USER"),
			os.Getenv("DB_PASSWORD"),
			os.Getenv("DB_NAME"))

		DB, err = sql.Open("postgres", connStr)
		if err != nil {
			log.Fatalf("❌ Error saat mempersiapkan koneksi ke database PostgreSQL: %v", err)
		}

		err = DB.Ping()
		if err == nil {
			log.Println("✅ Berhasil terhubung ke database PostgreSQL.")
			break
		}

		log.Printf("⚠️ Gagal terhubung ke database PostgreSQL, mencoba lagi dalam 5 detik... (%d/%d)", i+1, retries)
		time.Sleep(5 * time.Second)
	}

	if err != nil {
		log.Fatalf("❌ Gagal terhubung ke database PostgreSQL setelah beberapa kali percobaan: %v", err)
	}

	createTables()
	log.Println("⚙️  Memulai proses seeding database...")
	seed.SeedData(DB)
	log.Println("✅ Seeding database selesai.")
	LoadLookupData()
}

func createTables() {
	commands := []string{
		`CREATE TABLE IF NOT EXISTS ck (
            ck_id SERIAL PRIMARY KEY,
            name TEXT NOT NULL UNIQUE
        );`,
		`CREATE TABLE IF NOT EXISTS area (
            area_id SERIAL PRIMARY KEY,
            name TEXT NOT NULL UNIQUE,
            ck_id INTEGER NOT NULL,
            FOREIGN KEY (ck_id) REFERENCES ck (ck_id) ON DELETE CASCADE
        );`,
		`CREATE TABLE IF NOT EXISTS door (
            door_id SERIAL PRIMARY KEY,
            name TEXT NOT NULL,
            area_id INTEGER NOT NULL,
            FOREIGN KEY (area_id) REFERENCES area (area_id) ON DELETE CASCADE
        );`,
		`CREATE TABLE IF NOT EXISTS temp (
            temp_id SERIAL PRIMARY KEY,
            value REAL NOT NULL,
            area_id INTEGER NOT NULL,
            no INTEGER NOT NULL,
            ts TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
            FOREIGN KEY (area_id) REFERENCES area (area_id) ON DELETE CASCADE
        );`,
		`CREATE TABLE IF NOT EXISTS rh (
            rh_id SERIAL PRIMARY KEY,
            value REAL NOT NULL,
            area_id INTEGER NOT NULL,
            no INTEGER NOT NULL,
            ts TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
            FOREIGN KEY (area_id) REFERENCES area (area_id) ON DELETE CASCADE
        );`,
		`CREATE TABLE IF NOT EXISTS prox (
            prox_id SERIAL PRIMARY KEY,
            value INTEGER NOT NULL,
            door_id INTEGER NOT NULL,
            ts TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
            FOREIGN KEY (door_id) REFERENCES door (door_id) ON DELETE CASCADE
        );`,
		`CREATE TABLE IF NOT EXISTS users (
            user_id SERIAL PRIMARY KEY,
            username TEXT NOT NULL UNIQUE,
            password_hash TEXT NOT NULL,
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        );`,
	}

	for _, command := range commands {
		_, err := DB.Exec(command)
		if err != nil {
			log.Fatalf("❌ Error membuat tabel dengan perintah %s: %v", command, err)
		}
	}
	log.Println("✅ Struktur tabel database berhasil diperiksa/dibuat.")
}

func LoadLookupData() {
	AreaNames = make(map[int]string)
	DoorNames = make(map[int]string)
	DoorToAreaMap = make(map[int]int)

	rows, err := DB.Query("SELECT area_id, name FROM area")
	if err != nil {
		log.Printf("Error memuat nama area: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			log.Printf("Error scanning baris area: %v", err)
			continue
		}
		AreaNames[id] = name
	}
	if err = rows.Err(); err != nil {
		log.Printf("Error setelah iterasi baris area: %v", err)
	}

	doorRows, err := DB.Query("SELECT door_id, name, area_id FROM door")
	if err != nil {
		log.Printf("Error memuat nama pintu: %v", err)
		return
	}
	defer doorRows.Close()

	for doorRows.Next() {
		var id, areaID int
		var name string
		if err := doorRows.Scan(&id, &name, &areaID); err != nil {
			log.Printf("Error scanning baris pintu: %v", err)
			continue
		}
		DoorNames[id] = name
		DoorToAreaMap[id] = areaID
	}
	if err = doorRows.Err(); err != nil {
		log.Printf("Error setelah iterasi baris pintu: %v", err)
	}
	log.Printf("✔️ Berhasil memuat %d nama area dan %d nama pintu ke lookup map.", len(AreaNames), len(DoorNames))
}

func GetDB() *sql.DB {
	return DB
}

func GetAreaName(areaID int) string {
	if name, ok := AreaNames[areaID]; ok {
		return name
	}
	return fmt.Sprintf("Area ID %d (Nama tidak ditemukan)", areaID)
}

func GetDoorInfo(doorID int) (doorName string, areaID int, areaName string) {
	var ok bool
	doorName, ok = DoorNames[doorID]
	if !ok {
		doorName = fmt.Sprintf("Pintu ID %d (Nama tidak ditemukan)", doorID)
	}

	areaID, ok = DoorToAreaMap[doorID]
	if !ok {
		areaName = "Area tidak diketahui untuk pintu ini"
		return doorName, 0, areaName
	}

	areaName = GetAreaName(areaID)
	return doorName, areaID, areaName
}

// IsDoorRegistered memeriksa apakah sebuah door ID terdaftar di lookup map.
func IsDoorRegistered(doorID int) bool {
	_, ok := DoorNames[doorID]
	return ok
}

func CloseDB() {
	if DB != nil {
		DB.Close()
	}
}
