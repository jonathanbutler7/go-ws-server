package server

import (
	"database/sql"
	"fmt"
	_ "github.com/lib/pq"
	"os"
)

var goPath = os.Getenv("GOPATH")
var ShouldLog = false

var DB *sql.DB

func InitDB() {
	var err error
	DB, err = sql.Open("postgres", "user=jonathanbutler dbname=wsauditlog sslmode=disable")
	if err != nil {
		fmt.Printf("Failed to initialize database: %v", err)
	}
	if err := DB.Ping(); err != nil {
		fmt.Printf("Database ping failed: %v", err)
	} else {
		fmt.Println("Successfully connected to db ✅")
	}
}

func (s *Server) LogAction(userId, action, roomId, message string) {
	if ShouldLog {
		_, err := DB.Exec(`
			INSERT INTO audit_logs (
				user_id, 
				action_type, 
				room_id, 
				message
			) 
			VALUES (
				$1, 
				$2, 
				$3, 
				$4
			);`,
			userId, action, roomId, message,
		)

		if err != nil {
			fmt.Printf(
				"Failed to insert join audit log for user %s in room %s: %v",
				userId, roomId, err,
			)
		}
	}
}
