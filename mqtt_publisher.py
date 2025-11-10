# Python script to send data to the MQTT broker for the IoT Suhu application.
#
# This script simulates sensor devices sending data to the backend service.
# It generates large batches of JSON data (approx. 50 MB each) and sends them
# until the total data sent reaches 250 MB.
#
# Before running, make sure you have the paho-mqtt library installed:
# pip install paho-mqtt
#

import json
import paho.mqtt.client as mqtt
import time
import random
from datetime import datetime, timezone

# --- MQTT Broker Configuration ---
MQTT_BROKER_URI = "localhost"
MQTT_PORT = 1883
MQTT_TOPIC_INGEST = "sensor/data/ingest"
MQTT_USERNAME = "servfi_app"
MQTT_PASSWORD = "S3cr3tP@ssw0rd!"

# --- Payload Creation ---
def create_random_payload_dict():
    """Creates a random data payload dictionary for CK 3 and a random area (1-9)."""
    now_utc = datetime.now(timezone.utc).strftime('%Y-%m-%dT%H:%M:%SZ')
    num_door_sensors = random.randint(1, 4)
    num_temp_sensors = random.randint(2, 5)

    door_data = [{"doorid": i, "value": random.randint(0, 1)} for i in range(1, num_door_sensors + 1)]
    temp_data = [
        {
            "no": i,
            "ts": now_utc,
            "temp": round(random.uniform(20.0, 35.0), 2),
            "rh": round(random.uniform(40.0, 80.0), 2)
        } for i in range(1, num_temp_sensors + 1)
    ]

    return {
        "ck": 3,
        "area": random.randint(1, 9),
        "door": door_data,
        "temp": temp_data
    }

def generate_json_batch(target_size_kb):
    """Generates a single JSON string consisting of a list of payloads,
       with a total size close to the target in KB."""
    target_size_bytes = target_size_kb * 1024
    payload_list = []
    current_size = 0

    print(f"Generating a {target_size_kb} KB batch... (this may take a moment)")

    # Estimate the size of a single payload to start with a reasonable number
    # of items, reducing the number of checks.
    # A typical payload is around 500 bytes.
    estimated_items = target_size_bytes // 500
    for _ in range(estimated_items):
        payload_list.append(create_random_payload_dict())

    # Refine the size until it's just over the target
    while True:
        # Use compact encoding (no indentation, no spaces after separators)
        json_string = json.dumps(payload_list, separators=(',', ':'))
        current_size = len(json_string.encode('utf-8'))
        if current_size >= target_size_bytes:
            break
        # Add more items if needed
        payload_list.append(create_random_payload_dict())

    print(f"Batch generated. Actual size: {current_size / 1024:.2f} KB")
    return json_string

# --- MQTT Callbacks ---
def on_connect(client, userdata, flags, rc):
    """Callback for when the client connects to the broker."""
    if rc == 0:
        print("✅ Successfully connected to MQTT Broker!")
    else:
        print(f"❌ Failed to connect, return code {rc}\n")
        exit()

def on_publish(client, userdata, mid):
    """Callback for when a message is published."""
    # This callback is fired after the message is sent.
    pass

# --- Main Execution ---
def main():
    """Main function to connect and publish data in large batches."""
    total_bytes_sent = 0
    total_size_limit_bytes = 6 * 1024 * 1024  # 6 MB
    batch_size_kb = 200  # 200 KB

    client = mqtt.Client()
    client.on_connect = on_connect
    client.on_publish = on_publish
    client.username_pw_set(MQTT_USERNAME, MQTT_PASSWORD)

    print(f"🔌 Connecting to broker at {MQTT_BROKER_URI}:{MQTT_PORT}...")
    try:
        client.connect(MQTT_BROKER_URI, MQTT_PORT, 60)
    except Exception as e:
        print(f"❌ Could not connect to MQTT Broker: {e}")
        print("Please ensure the broker is running and accessible.")
        return

    client.loop_start()

    try:
        while total_bytes_sent < total_size_limit_bytes:
            # Generate a 200 KB JSON batch
            payload_str = generate_json_batch(batch_size_kb)
            payload_bytes = payload_str.encode('utf-8')
            payload_size = len(payload_bytes)

            print(f"🚀 Sending batch of {payload_size / 1024:.2f} KB...")
            result = client.publish(MQTT_TOPIC_INGEST, payload_bytes, qos=1)
            
            # Wait for the publish to complete. For large messages, this might take time.
            result.wait_for_publish(timeout=60) 

            if result.rc == mqtt.MQTT_ERR_SUCCESS:
                total_bytes_sent += payload_size
                progress_mb = total_bytes_sent / (1024 * 1024)
                total_limit_mb = total_size_limit_bytes / (1024 * 1024)
                print(f"✅ Batch sent successfully. Total sent: {progress_mb:.2f} MB / {total_limit_mb:.2f} MB\n")
            else:
                # Note: MQTT brokers often have a message size limit (e.g., 256MB by default in Mosquitto).
                # If you see this error, the batch might be too large for the broker's configuration.
                print(f"\n❌ Failed to publish batch: {mqtt.error_string(result.rc)}.")
                print("This might be due to the broker's message size limit. Stopping.")
                break
            
            # Optional: pause between batches if needed
            # time.sleep(1)

    except KeyboardInterrupt:
        print("\n🛑 Process interrupted by user.")
    finally:
        print("\n")
        client.loop_stop()
        client.disconnect()
        print("🏁 Process finished.")
        progress_mb = total_bytes_sent / (1024 * 1024)
        print(f"Total data sent: {progress_mb:.2f} MB")
        print("🔌 Disconnected from MQTT Broker.")

if __name__ == '__main__':
    main()