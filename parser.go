package main

import (
	"crypto/rand"
	"fmt"
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"log"

	_ "github.com/mattn/go-sqlite3"
)

import "github.com/acmcsufoss/api.acmcsuf.com/internal/api/store/dbmodels"

type Role struct {
	Title string `json:"title"`
	Tier int64 `json:"tier"`
}

type Tier struct {
	ID int `json:"id"`
	Index int `json:"index"`
}

type Member struct {
	FullName string `json:"fullName"`
	Picture string `json:"picture"`
	Positions map[string][]Role `json:"positions"`
	Discord string `json:"discord"` 
	Github string `json:"github"`
}

func toNullString(s string) sql.NullString {
	return sql.NullString{
		String: s,
		Valid: s != "",
	}
}

// Helper to convert a numeric int into a nullable database NullString parameter
func intToNullString(val int) sql.NullString {
	return sql.NullString{
		String: fmt.Sprintf("%d", val),
		Valid:  true,
	}
}

// Helper to convert an int variable into a database safe sql.NullInt64 parameter
func toNullInt64(val int) sql.NullInt64 {
	return sql.NullInt64{
		Int64: int64(val),
		Valid: true,
	}
}


func generateUUID() (string, error) {
	uuid := make([]byte, 16)
	_, err := rand.Read(uuid)
	if err != nil {
		return "", err
	}

	// Set version to 4 (pseudo-random)
	uuid[6] = (uuid[6] & 0x0f) | 0x40
	// Set variant to RFC 4122
	uuid[8] = (uuid[8] & 0x3f) | 0x80

	return fmt.Sprintf("%x-%x-%x-%x-%x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16]), nil
}

func main() {
	ctx := context.Background()

	jsonPath := "officers.json"
	jsonPath2 := "tiers.json"

	fileBytes, err := os.ReadFile(jsonPath)
	if err != nil {
		log.Fatalf("Error to read JSON: %v", err)
	}

	fBytes, err := os.ReadFile(jsonPath2)
	if err != nil {
		log.Fatalf("Error to read JSON: %v", err)
	}

	var tiers map[string]Tier
	if err := json.Unmarshal(fBytes, &tiers); err != nil {
		log.Fatalf("Failed to decode JSON contents: %v", err)
	}

	var members []Member
	if err := json.Unmarshal(fileBytes, &members); err != nil {
		log.Fatalf("Failed to decode JSON contents: %v", err)
	}

	db, err := sql.Open("sqlite3", "local.db")
	if err != nil {
		log.Fatalf("Database connection failed: %v", err)
	}
	defer db.Close()

	queries := dbmodels.New(db)

	// 3. Create a transaction for performance safety and rollback security
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		log.Fatalf("Failed to begin db transaction: %v", err)
	}
	defer tx.Rollback() // Automatically rolls back if an error stops code execution early

	// Bind sqlc queries to work scoped inside this specific transaction context
	qtx := queries.WithTx(tx)

	// 4. Main processor loop
	for _, m := range members {
		// Generate an officer UUID if one doesn't exist natively in the JSON
		officerUUID, err := generateUUID()
		if err != nil {
			log.Fatalf("Failed to generate native cryptographic UUID: %v", err)
		}

		// Prepare params for CreateOfficer method
		officerParams := dbmodels.CreateOfficerParams{
			Uuid:     officerUUID,
			FullName: m.FullName,
			Picture:  toNullString(m.Picture),
			Github:   toNullString(m.Github),
			Discord:  toNullString(m.Discord),
		}

		// Insert Officer record via sqlc 
		_, err = qtx.CreateOfficer(ctx, officerParams)
		if err != nil {
			log.Fatalf("Failed creating officer %s: %v", m.FullName, err)
		}

		// Iterate through the nested Positions map: map[string][]Role
		// semester key is "F25", "S26", etc.
		for semester, roles := range m.Positions {
			for _, r := range roles {
				
				// Prepare params for CreatePosition method
				positionParams := dbmodels.CreatePositionParams{
					OfficerID: officerUUID,
					Semester:  semester,
					Tier:      r.Tier,
					FullName:  m.FullName,
					Title:     toNullString(r.Title),
					Team:      sql.NullString{Valid: false}, // Set true if JSON provides team details
				}

				// Insert Position record via sqlc
				_, err := qtx.CreatePosition(ctx, positionParams)
				if err != nil {
					log.Fatalf("Failed creating position for officer %s: %v", m.FullName, err)
				}
			}
		}
	}

	for tierName, t := range tiers {

		tierParams := dbmodels.CreateTierParams{
			Tier:   int64(t.ID),
			Title:  toNullString(tierName),
			TIndex: toNullInt64(t.Index),
			Team:   sql.NullString{Valid: false},
		}

		_, err := qtx.CreateTier(ctx, tierParams)
		if err != nil {
			log.Fatalf("Failed creating position for tier %s: %v", tierName, err)
		}

	}

	// 5. Safely save all elements to the database file at once
	if err := tx.Commit(); err != nil {
		log.Fatalf("Transaction commit failed: %v", err)
	}


	log.Println("Successfully integrated JSON into SQLite via sqlc!")
	
}