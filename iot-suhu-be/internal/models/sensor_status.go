package models

import (
	"IoTT/internal/config"
	"IoTT/internal/database"
	"IoTT/internal/telegram"
	timed "IoTT/internal/time"
	"fmt"
	"log"
	"sync"
	"time"
)

type SensorOperationalStatus struct {
	SensorKey                   string
	SensorType                  string
	AreaID                      int
	SensorNo                    int
	DoorID                      int
	LastSeen                    time.Time
	IsOffline                   bool
	LastOfflineNotificationTime time.Time
	IsDoorOpen                  bool
	DoorOpenStartTime           time.Time
	DoorOpenWarningAlertSent    bool
	DoorOpenDangerAlertSent     bool
}

var (
	sensorStatusRegistry      = make(map[string]*SensorOperationalStatus)
	sensorStatusRegistryMutex = &sync.Mutex{}
)

const OfflineReminderInterval = 1 * time.Hour

const (
	SensorTypeTemp = "temp"
	SensorTypeRH   = "rh"
	SensorTypeProx = "prox"
)

func RegisterOrUpdateSensorStatus(sensorKey, sensorType string, areaID, sensorNo, doorID int, timestamp time.Time, value interface{}) {
	sensorStatusRegistryMutex.Lock()
	defer sensorStatusRegistryMutex.Unlock()

	status, exists := sensorStatusRegistry[sensorKey]
	if !exists {
		status = &SensorOperationalStatus{
			SensorKey:  sensorKey,
			SensorType: sensorType,
			AreaID:     areaID,
			SensorNo:   sensorNo,
			DoorID:     doorID,
			LastSeen:   timestamp,
			IsOffline:  false,
		}
		sensorStatusRegistry[sensorKey] = status

	} else { // notif sensor online
		status.LastSeen = timestamp
		if status.IsOffline {

			onlineMsg := fmt.Sprintf("✅ **Sensor Online**\nSensor %s (%s) kembali mengirimkan data.\nData terakhir pada: %s", getSensorFriendlyName(status), sensorKey, timestamp.In(config.Timezone).Format(time.RFC1123))
			telegram.SendAlert(onlineMsg)
			status.IsOffline = false

		}
	}

	if sensorType == SensorTypeProx {
		proxFloat, ok := value.(float64)
		if !ok {
			log.Printf("Peringatan: Gagal mengonversi nilai prox untuk sensor %s ke float64", sensorKey)
			return
		}
		proxValue := int(proxFloat)

		if proxValue == 0 { //buka
			if !status.IsDoorOpen {
				status.IsDoorOpen = true
				status.DoorOpenStartTime = timestamp
				status.DoorOpenWarningAlertSent = false
				status.DoorOpenDangerAlertSent = false
			}
		} else {
			if status.IsDoorOpen {
				if status.DoorOpenWarningAlertSent || status.DoorOpenDangerAlertSent {
					doorOpenDuration := timestamp.Sub(status.DoorOpenStartTime)
					closedMsg := fmt.Sprintf("🚪✅ **Pintu Ditutup**\nSensor Pintu %s (%s) sekarang tertutup.\nPintu sebelumnya terbuka selama %s.",
						getSensorFriendlyName(status), status.SensorKey, doorOpenDuration.Round(time.Minute))
					telegram.SendAlert(closedMsg)
				}
				status.IsDoorOpen = false               // Reset status pintu
				status.DoorOpenWarningAlertSent = false // Reset flag warning
				status.DoorOpenDangerAlertSent = false  // Reset flag danger
			}
		}
	}
}

// checkOfflineStatus memeriksa apakah sebuah sensor offline dan mengirim notifikasi jika perlu.
func checkOfflineStatus(status *SensorOperationalStatus, now time.Time, offlineThreshold time.Duration) {
	currentActualOfflineDuration := now.Sub(status.LastSeen)

	if currentActualOfflineDuration > offlineThreshold {
		if !status.IsOffline {
			actualOfflineMinutes := int(currentActualOfflineDuration.Minutes())

			status.IsOffline = true
			status.LastOfflineNotificationTime = now

			offlineMsg := fmt.Sprintf("⚠️ **Sensor Offline**\nSensor %s (%s) tidak mengirimkan data selama %d menit.\nTerakhir terlihat: %s\n🪛 Laporkan : [📞 Call . . . ](https://wa.me/+6282221294931)",
				getSensorFriendlyName(status), status.SensorKey, actualOfflineMinutes, status.LastSeen.In(config.Timezone).Format(time.RFC1123))

			telegram.SendAlert(offlineMsg)
		} else {
			if now.Sub(status.LastOfflineNotificationTime) >= OfflineReminderInterval {
				totalOfflineMinutes := int(currentActualOfflineDuration.Minutes())

				reminderMsg := fmt.Sprintf("🕒 **Sensor Masih Offline (Pengingat)**\nSensor %s (%s) masih tidak mengirimkan data.\nTotal durasi offline: %d menit.\nNotifikasi terakhir dikirim %s.\nTerakhir terlihat: %s\n🪛 Laporkan : [📞 Call . . . ](https://facebook.com)",
					getSensorFriendlyName(status), status.SensorKey, totalOfflineMinutes, status.LastOfflineNotificationTime.In(config.Timezone).Format(time.RFC1123), status.LastSeen.In(config.Timezone).Format(time.RFC1123))
				telegram.SendAlert(reminderMsg)
				status.LastOfflineNotificationTime = now
			}
		}
	}
}

// checkDoorOpenStatus memeriksa apakah pintu terbuka terlalu lama dan mengirim notifikasi bertingkat.
func checkDoorOpenStatus(status *SensorOperationalStatus, now time.Time, warningThreshold, dangerThreshold time.Duration) {
	if status.SensorType != SensorTypeProx || !status.IsDoorOpen {
		return
	}

	doorOpenDuration := now.Sub(status.DoorOpenStartTime)

	// Level DANGER (prioritas lebih tinggi, cek dulu)
	if doorOpenDuration >= dangerThreshold && !status.DoorOpenDangerAlertSent {
		alertMsg := fmt.Sprintf("🚪🚨 **[DANGER] Pintu Terbuka Terlalu Lama**\nSensor Pintu %s (%s) telah terbuka lebih dari %d menit.\n**Status: DANGER**\nTerbuka sejak: %s\n🪛 Segera lakukan pengecekan: [📞 Call . . . ](https://wa.me/+6282221294931)",
			getSensorFriendlyName(status),
			status.SensorKey,
			config.OFFLINE_DETECTION_CONFIG.DoorOpenDangerMinutes,
			status.DoorOpenStartTime.In(config.Timezone).Format(time.RFC1123))

		telegram.SendAlert(alertMsg)
		status.DoorOpenDangerAlertSent = true
	} else if doorOpenDuration >= warningThreshold && !status.DoorOpenWarningAlertSent {
		// Level WARNING (hanya dijalankan jika kondisi Danger tidak terpenuhi)
		alertMsg := fmt.Sprintf("🚪⚠️ **[WARNING] Pintu Terbuka**\nSensor Pintu %s (%s) telah terbuka lebih dari %d menit.\n**Status: WARNING**\nTerbuka sejak: %s\n🪛 Mohon segera ditutup.",
			getSensorFriendlyName(status),
			status.SensorKey,
			config.OFFLINE_DETECTION_CONFIG.DoorOpenWarningMinutes,
			status.DoorOpenStartTime.In(config.Timezone).Format(time.RFC1123))

		telegram.SendAlert(alertMsg)
		status.DoorOpenWarningAlertSent = true
	}
}

// RunPeriodicChecks menjalankan semua pengecekan rutin untuk setiap sensor.
func RunPeriodicChecks() {
	sensorStatusRegistryMutex.Lock()
	defer sensorStatusRegistryMutex.Unlock()
	now := timed.Now().In(config.Timezone)
	offlineThresholdDuration := time.Duration(config.OFFLINE_DETECTION_CONFIG.MaxIdleMinutes) * time.Minute
	doorOpenWarningThreshold := time.Duration(config.OFFLINE_DETECTION_CONFIG.DoorOpenWarningMinutes) * time.Minute
	doorOpenDangerThreshold := time.Duration(config.OFFLINE_DETECTION_CONFIG.DoorOpenDangerMinutes) * time.Minute

	for _, status := range sensorStatusRegistry {
		// Pengecekan 1: Sensor Offline (Logika ini tetap ada dan tidak berubah)
		checkOfflineStatus(status, now, offlineThresholdDuration)

		// Pengecekan 2: Pintu Terbuka (Logika baru yang ditambahkan)
		checkDoorOpenStatus(status, now, doorOpenWarningThreshold, doorOpenDangerThreshold)
	}
}

func getSensorFriendlyName(status *SensorOperationalStatus) string {

	switch status.SensorType {
	case "temp":
		areaName := database.GetAreaName(status.AreaID)
		return fmt.Sprintf("Suhu di Area %d Titik %d (%s ) ", status.AreaID, status.SensorNo, areaName)
	case "rh":
		areaName := database.GetAreaName(status.AreaID)
		return fmt.Sprintf("Kelembaban di Area %d Titik %d (%s)", status.AreaID, status.SensorNo, areaName)
	case "prox":
		doorName, _, associatedAreaName := database.GetDoorInfo(status.DoorID)
		return fmt.Sprintf("Proximity di Area %s Pintu %d (%s) ", associatedAreaName, status.DoorID, doorName)
	default:
		return status.SensorKey
	}
}

func StartPeriodicCheckWorker() {
	ticker := time.NewTicker(1 * time.Minute)
	go func() {
		for {
			<-ticker.C
			RunPeriodicChecks()
		}
	}()
	log.Println("✅ Worker untuk pengecekan periodik (Offline & Pintu) telah dimulai.")
}

func GetSensorOperationalStatus(sensorKey string) (SensorOperationalStatus, bool) {
	sensorStatusRegistryMutex.Lock()
	defer sensorStatusRegistryMutex.Unlock()

	status, exists := sensorStatusRegistry[sensorKey]
	if !exists {
		return SensorOperationalStatus{}, false
	}

	return *status, true
}
