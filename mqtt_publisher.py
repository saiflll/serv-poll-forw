# Skrip Python untuk mengirim data ke broker MQTT untuk aplikasi IoT Suhu.
#
# Skrip ini mensimulasikan perangkat sensor yang mengirim data ke layanan backend.
# Ini menghasilkan batch besar data JSON (sekitar 50 MB masing-masing) dan mengirimkannya
# hingga total data yang dikirim mencapai 250 MB.
#
# Sebelum menjalankan, pastikan Anda telah menginstal pustaka paho-mqtt:
# pip install paho-mqtt
#

import json
import paho.mqtt.client as mqtt
import time
import random
from datetime import datetime, timezone
import os

# --- Konfigurasi Broker MQTT ---
MQTT_BROKER_URI = os.getenv("MQTT_BROKER_URI", "localhost") # Baca dari env, default ke localhost
MQTT_PORT = 1883
MQTT_TOPIC_INGEST = "sensor/data/ingest"
MQTT_USERNAME = "servfi_app"
MQTT_PASSWORD = "S3cr3tP@ssw0rd!"

# --- Pembuatan Payload ---
def create_registered_payload_dict():
    """Membuat kamus payload data hanya menggunakan sensor dan pintu yang terdaftar."""
    
    # Data dari forward/internal/config/config.go
    registered_temp_sensors = {
        1: [1, 2, 3], 2: [1, 2, 3], 3: [1], 4: [1, 2], 5: [1], 6: [1], 7: [1], 8: [1], 9: [1],
        10: [1], 11: [1], 12: [1], 13: [1, 2, 3, 4], 14: [1, 2, 3, 4, 5, 6, 7, 8, 9],
        15: [1], 16: [1, 2], 17: [1, 2, 3, 4, 5, 6], 18: [1], 19: [1, 2]
    }
    registered_rh_sensors = {
        11: [1], 12: [1], 13: [1, 2, 3, 4], 15: [1, 2]
    }
    
    # Data dari forward/internal/seed/seed.go
    registered_doors = {
        1: [11, 13, 15, 17], 2: [21, 23, 25, 22, 24, 26], 3: [31, 32], 5: [51, 52],
        6: [61, 62], 10: [101, 102, 104], 11: [112, 114], 12: [121], 17: [171, 173, 172, 174, 176],
        18: [181, 182]
    }
    
    all_areas = sorted(list(set(registered_temp_sensors.keys()) | set(registered_rh_sensors.keys()) | set(registered_doors.keys())))
    
    area_id = random.choice(all_areas)
    now_utc = datetime.now(timezone.utc).strftime('%Y-%m-%dT%H:%M:%SZ')

    door_data = []
    if area_id in registered_doors:
        # Pilih sejumlah pintu acak untuk area ini
        num_doors_to_send = random.randint(1, len(registered_doors[area_id]))
        selected_door_ids = random.sample(registered_doors[area_id], num_doors_to_send)
        door_data = [{"doorid": door_id, "value": random.randint(0, 1)} for door_id in selected_door_ids]

    temp_data = []
    if area_id in registered_temp_sensors:
        # Pilih sejumlah sensor suhu acak untuk area ini
        num_sensors_to_send = random.randint(1, len(registered_temp_sensors[area_id]))
        selected_sensor_nos = random.sample(registered_temp_sensors[area_id], num_sensors_to_send)
        
        for sensor_no in selected_sensor_nos:
            data_point = {
                "no": sensor_no,
                "ts": now_utc,
                "temp": round(random.uniform(20.0, 35.0), 2),
            }
            # Jika sensor juga merupakan sensor RH, tambahkan data RH
            if area_id in registered_rh_sensors and sensor_no in registered_rh_sensors[area_id]:
                data_point["rh"] = round(random.uniform(40.0, 80.0), 2)
            temp_data.append(data_point)

    return {
        "ck": 3,
        "area": area_id,
        "door": door_data,
        "temp": temp_data
    }

def generate_json_batch(target_size_kb):
    """Menghasilkan satu string JSON yang terdiri dari daftar payload,
       dengan ukuran total mendekati target dalam KB."""
    target_size_bytes = target_size_kb * 1024
    payload_list = []
    current_size = 0

    print(f"Generating a {target_size_kb} KB batch... (this may take a moment)")

    # Perkirakan ukuran satu payload untuk memulai dengan jumlah item yang wajar
    # item, mengurangi jumlah pemeriksaan.
    # Payload umum sekitar 500 byte.
    estimated_items = target_size_bytes // 500
    for _ in range(estimated_items):
        payload_list.append(create_registered_payload_dict())

    # Sempurnakan ukuran hingga sedikit di atas target
    while True:
        # Gunakan pengkodean ringkas (tanpa lekukan, tanpa spasi setelah pemisah)
        json_string = json.dumps(payload_list, separators=(',', ':'))
        current_size = len(json_string.encode('utf-8'))
        if current_size >= target_size_bytes:
            break
        # Tambahkan lebih banyak item jika perlu
        payload_list.append(create_registered_payload_dict())

    print(f"Batch generated. Actual size: {current_size / 1024:.2f} KB")
    return json_string

# --- Callback MQTT ---
def on_connect(client, userdata, flags, rc):
    """Callback untuk saat klien terhubung ke broker."""
    if rc == 0:
        print("✅ Berhasil terhubung ke MQTT Broker!")
    else:
        print(f"❌ Gagal terhubung, return code {rc}\n")
        exit()

def on_publish(client, userdata, mid):
    """Callback untuk saat pesan dipublikasikan."""
    # Callback ini diaktifkan setelah pesan dikirim.
    pass

# --- Eksekusi Utama ---
def main():
    """Fungsi utama untuk menghubungkan dan mempublikasikan data dalam batch besar."""
    total_bytes_sent = 0
    total_size_limit_bytes = 6 * 1024 * 1024  # 6 MB
    batch_size_kb = 200  # 200 KB

    client = mqtt.Client()
    client.on_connect = on_connect
    client.on_publish = on_publish
    client.username_pw_set(MQTT_USERNAME, MQTT_PASSWORD)

    print(f"🔌 Menghubungkan ke broker di {MQTT_BROKER_URI}:{MQTT_PORT}...")
    try:
        client.connect(MQTT_BROKER_URI, MQTT_PORT, 60)
    except Exception as e:
        print(f"❌ Tidak dapat terhubung ke MQTT Broker: {e}")
        print("Harap pastikan broker berjalan dan dapat diakses.")
        return

    client.loop_start()

    try:
        while total_bytes_sent < total_size_limit_bytes:
            # Hasilkan batch JSON 200 KB
            payload_str = generate_json_batch(batch_size_kb)
            payload_bytes = payload_str.encode('utf-8')
            payload_size = len(payload_bytes)

            print(f"🚀 Mengirim batch {payload_size / 1024:.2f} KB...")
            result = client.publish(MQTT_TOPIC_INGEST, payload_bytes, qos=1)
            
            # Tunggu hingga publikasi selesai. Untuk pesan besar, ini mungkin memakan waktu.
            result.wait_for_publish(timeout=60) 

            if result.rc == mqtt.MQTT_ERR_SUCCESS:
                total_bytes_sent += payload_size
                progress_mb = total_bytes_sent / (1024 * 1024)
                total_limit_mb = total_size_limit_bytes / (1024 * 1024)
                print(f"✅ Batch berhasil dikirim. Total terkirim: {progress_mb:.2f} MB / {total_limit_mb:.2f} MB\n")
            else:
                # Catatan: Broker MQTT seringkali memiliki batas ukuran pesan (mis., 256MB secara default di Mosquitto).
                # Jika Anda melihat kesalahan ini, batch mungkin terlalu besar untuk konfigurasi broker.
                print(f"\n❌ Gagal mempublikasikan batch: {mqtt.error_string(result.rc)}.")
                print("Ini mungkin karena batas ukuran pesan broker. Berhenti.")
                break
            
            # Opsional: jeda antar batch jika perlu
            # time.sleep(1)

    except KeyboardInterrupt:
        print("\n🛑 Proses dihentikan oleh pengguna.")
    finally:
        print("\n")
        client.loop_stop()
        client.disconnect()
        print("🏁 Proses selesai.")
        progress_mb = total_bytes_sent / (1024 * 1024)
        print(f"Total data terkirim: {progress_mb:.2f} MB")
        print("🔌 Terputus dari Broker MQTT.")

if __name__ == '__main__':
    main()