package archiver

import (
	"IoTT/internal/database"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"time"
)

const (
	retentionDays = 365 * 2 // 2 years
	backupDir     = "./backups"
)

// Start begins the periodic archiving process.
func Start() {
	// Run once on startup to ensure it runs even if the app restarts often.
	go runArchivingProcess()

	// Then run every 24 hours.
	ticker := time.NewTicker(24 * time.Hour)
	go func() {
		for range ticker.C {
			runArchivingProcess()
		}
	}()
	log.Println("✅ Archiver worker started. Will run every 24 hours.")
}

func runArchivingProcess() {
	log.Println("🚀 Starting daily data archiving process...")

	db := database.GetDB()
	if db == nil {
		log.Println("Error (Archiver): Database connection is not available.")
		return
	}

	// Calculate the timestamp threshold (2 years ago)
	threshold := time.Now().AddDate(-2, 0, 0)
	log.Printf("Info (Archiver): Archiving data older than %s", threshold.Format("2006-01-02"))

	// Ensure backup directory exists
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		log.Printf("Error (Archiver): Could not create backup directory '%s': %v", backupDir, err)
		return
	}

	tablesToArchive := []string{"temp", "rh", "prox"}
	var totalRowsArchived int64 = 0

	for _, table := range tablesToArchive {
		// 1. Backup the data
		rowsArchived, err := backupTable(table, threshold)
		if err != nil {
			log.Printf("Error (Archiver): Failed to backup table '%s'. Halting process for this table. Error: %v", table, err)
			continue // Skip to next table if backup fails
		}

		if rowsArchived == 0 {
			log.Printf("Info (Archiver): No data to archive for table '%s'.", table)
			continue
		}

		// 2. Delete the data AFTER successful backup
		rowsDeleted, err := deleteFromTable(table, threshold)
		if err != nil {
			log.Printf("CRITICAL (Archiver): Failed to delete archived data from table '%s'. Manual cleanup may be required. Error: %v", table, err)
			continue // Do not proceed if deletion fails
		}

		log.Printf("✅ Success (Archiver): Archived %d rows and deleted %d rows from table '%s'.", rowsArchived, rowsDeleted, table)
			totalRowsArchived += rowsArchived
	}

	log.Printf("🏁 Archiving process finished. Total rows archived: %d", totalRowsArchived)
}

func backupTable(tableName string, threshold time.Time) (int64, error) {
	db := database.GetDB()
	query := fmt.Sprintf("SELECT * FROM %s WHERE ts < $1", tableName)

	rows, err := db.Query(query, threshold)
	if err != nil {
		return 0, fmt.Errorf("failed to query old data: %w", err)
	}
	defer rows.Close()

	// Create backup file
	fileName := fmt.Sprintf("%s/backup_%s_%s.csv", backupDir, tableName, time.Now().Format("2006-01-02"))
	file, err := os.Create(fileName)
	if err != nil {
		return 0, fmt.Errorf("failed to create backup file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	columns, err := rows.Columns()
	if err != nil {
		return 0, fmt.Errorf("failed to get columns: %w", err)
	}
	if err := writer.Write(columns); err != nil {
		return 0, fmt.Errorf("failed to write header to csv: %w", err)
	}

	// Write rows
	values := make([]interface{}, len(columns))
	scanArgs := make([]interface{}, len(values))
	for i := range values {
		scanArgs[i] = &values[i]
	}

	var rowCount int64 = 0
	for rows.Next() {
		if err := rows.Scan(scanArgs...); err != nil {
			return 0, fmt.Errorf("failed to scan row: %w", err)
		}

		strValues := make([]string, len(columns))
		for i, val := range values {
			if val == nil {
				strValues[i] = ""
			} else {
				// Handle time.Time specifically for consistent format
				if t, ok := val.(time.Time); ok {
					strValues[i] = t.Format(time.RFC3339Nano)
				} else {
					strValues[i] = fmt.Sprintf("%v", val)
				}
			}
		}

		if err := writer.Write(strValues); err != nil {
			return 0, fmt.Errorf("failed to write row to csv: %w", err)
		}
		rowCount++
	}

	return rowCount, nil
}

func deleteFromTable(tableName string, threshold time.Time) (int64, error) {
	db := database.GetDB()
	query := fmt.Sprintf("DELETE FROM %s WHERE ts < $1", tableName)

	result, err := db.Exec(query, threshold)
	if err != nil {
		return 0, fmt.Errorf("failed to execute delete statement: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get affected rows: %w", err)
	}

	return rowsAffected, nil
}
