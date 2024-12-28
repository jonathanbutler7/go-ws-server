package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	_ "github.com/lib/pq"
	"io"
	"net/http"
	"os"
	"sync"

	"golang.org/x/net/websocket"
)

var goPath = os.Getenv("GOPATH")
var isLikelyLocal = goPath != ""

var db *sql.DB

func initDB() {
	var err error
	db, err = sql.Open("postgres", "user=jonathanbutler dbname=wsauditlog sslmode=disable")
	if err != nil {
		fmt.Println(err)
	}
	// Test connection
	if err := db.Ping(); err != nil {
		fmt.Println(err)
	} else {
		fmt.Println("Successfully connected to db ✅")
	}
}

type Server struct {
	mu        sync.Mutex
	chatRooms map[string]map[string]struct{} // roomID -> (userID -> empty struct)
	users     map[string]userInfo            // userID -> userInfo
}

type userInfo struct {
	rooms []string
	conn  *websocket.Conn
}

type ContentType string

const JoinRoomType ContentType = "join"
const LeaveRoomType ContentType = "leave"
const MessageType ContentType = "message"

type JoinRoomContent struct {
	UserId string `json:"userId"`
	RoomId string `json:"roomId"`
}

type LeaveRoomContent struct {
	UserId string `json:"userId"`
	RoomId string `json:"roomId"`
}

type MessageContent struct {
	Text        string `json:"text"`
	Destination string `json:"destination"`
}

// how does one user have multiple ws connected?
type Message struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}

func NewServer() *Server {
	return &Server{
		chatRooms: make(map[string]map[string]struct{}),
		users:     make(map[string]userInfo),
	}
}

// "*" makes it a pointer receiver
// makes it so we modify the original connections map, not a copy
func (s *Server) addConnection(ws *websocket.Conn, userId string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.users[userId]; exists {
		// If the user already exists, just update the connection, and keep existing rooms
		user := s.users[userId]
		user.conn = ws
		s.users[userId] = user
	} else {
		// If this is a new user, create a new userInfo entry
		s.users[userId] = userInfo{
			conn:  ws,
			rooms: []string{}, // Initialize with empty rooms slice
		}
	}
}

func (s *Server) logAction(userId, action, roomId, message string) {
	if !isLikelyLocal {
		return
	}
	_, err := db.Exec(`
	INSERT INTO audit_logs (
		user_id, action_type, room_id, message
	) 
	VALUES (
	 	$1, $2, $3, $4
	 )`,
		userId, action, roomId, message)

	if err != nil {
		fmt.Printf("Failed to insert join audit log for user %s in room %s: %v", userId, roomId, err)
	}
}

func (s *Server) joinRoom(userId, roomId string, ws *websocket.Conn) {
	// if we don't have the room that the user wants to join, add it
	if _, exists := s.chatRooms[roomId]; !exists {
		s.mu.Lock()
		s.chatRooms[roomId] = make(map[string]struct{})
		s.mu.Unlock()
	}
	s.chatRooms[roomId][userId] = struct{}{}

	if user, exists := s.users[userId]; exists {
		// s.mu.Lock()
		user.rooms = append(user.rooms, roomId)
		// Ensure the connection is set if it's not already or if it's a new connection for this user
		if user.conn == nil {
			user.conn = ws
		}
		s.users[userId] = user
		// s.mu.Unlock()
	} else {
		// If the user does not exist, create a new userInfo with this room
		s.mu.Lock()
		s.users[userId] = userInfo{
			rooms: []string{roomId},
			conn:  ws,
		}
		s.mu.Unlock()
	}
	s.logAction(userId, "join", roomId, "")
	s.notifyRoomOfJoin(roomId, userId)
}

func (s *Server) notifyRoomOfJoin(roomId, userId string) {
	message := Message{
		Content: fmt.Sprintf("%s joined room %s", userId, roomId),
		Type:    "join",
	}
	s.broadcastToRoom(message, roomId)
}

func (s *Server) leaveRoom(roomId, userId string) {
	if _, exists := s.chatRooms[roomId][userId]; exists {
		delete(s.chatRooms[roomId], userId)
		message := Message{
			Content: fmt.Sprintf("%s left room %s", userId, roomId),
			Type:    "leave",
		}
		s.broadcastToRoom(message, roomId)
	}
}

func (s *Server) sendMessage(content, roomId string) {
	message := Message{
		Content: content,
		Type:    "message",
	}
	s.broadcastToRoom(message, roomId)
}

func (s *Server) broadcastToRoom(message Message, roomId string) {
	s.mu.Lock()
	roomUsers, exists := s.chatRooms[roomId]
	s.mu.Unlock()
	if !exists {
		return
	}
	for userId := range roomUsers {
		s.mu.Lock()
		user, foundUser := s.users[userId]
		s.mu.Unlock()
		if foundUser && user.conn != nil {
			go func(ws *websocket.Conn) {
				s.logAction(userId, message.Type, roomId, message.Content)
				b, err := json.Marshal(message)
				if err != nil {
					fmt.Println("Error marshaling struct: ", err)
					return
				}
				if _, err := ws.Write(b); err != nil {
					fmt.Println("error writing to socket", err)
				}
			}(user.conn)
		}
	}
}

func (s *Server) readLoop(ws *websocket.Conn) {
	buf := make([]byte, 1024)
	for {
		n, err := ws.Read(buf)
		if err != nil {
			if err == io.EOF {
				break
			}
			// could be good to write something to the client here
			fmt.Println("read error: ", err)
			continue
		}
		msg := buf[:n]

		var baseMsg struct {
			Type    ContentType     `json:"type"`
			Content json.RawMessage `json:"content"`
		}
		if err := json.Unmarshal(msg, &baseMsg); err != nil {
			fmt.Println("Error unmarshalling base message: ", err)
		}

		switch baseMsg.Type {
		case MessageType:
			var message MessageContent
			if err := json.Unmarshal(baseMsg.Content, &message); err != nil {
				fmt.Println("Error unmarshalling message: ", err)
				continue
			}
			s.sendMessage(message.Text, message.Destination)

		case LeaveRoomType:
			var leaveContent LeaveRoomContent
			if err := json.Unmarshal(baseMsg.Content, &leaveContent); err != nil {
				fmt.Println("Error unmarshalling leave message: ", err)
				continue
			}
			s.leaveRoom(leaveContent.RoomId, leaveContent.UserId)

		case JoinRoomType:
			var joinContent JoinRoomContent
			if err := json.Unmarshal(baseMsg.Content, &joinContent); err != nil {
				fmt.Println("Error unmarshalling join message: ", err)
				continue
			}
			s.joinRoom(joinContent.UserId, joinContent.RoomId, ws)
		}

	}
}

func (s *Server) handleWs(ws *websocket.Conn) {
	query := ws.Request().URL.Query()
	userId := query.Get("userId")

	// validation logic for not having userId
	if userId == "" {
		// you can only attempt to push a message down (dunno if that will work), or shut down the connection
		// maybe do both
		ws.Write([]byte("Invalid user or room ID"))
		ws.Close()
		return
	}
	// go isn't async, it's concurrent (switching between 2 threads quickly)
	// go will panic if multiple go routines accessing the map at the same time
	// s.conns[ws] = userId
	// adding the lock/unlock makes this race-condition-safe
	s.addConnection(ws, userId)
	// defer is like a finally in try/catch
	// defer func will still run even if there's a panic
	// if read loop is above the defer func, it stops bubbling up
	// useful if you have a panic that you're not sure why it's happening. would allow you to inspect closely
	defer func() {
		// will always run even if read loop freaks out
		s.mu.Lock()
		defer s.mu.Unlock()
		// remove ws connection and user
		delete(s.users, userId)
		// remove user from chat room
		// check if user exists before attempting to access their rooms
		if user, ok := s.users[userId]; ok {
			for _, room := range user.rooms {
				delete(s.chatRooms[room], userId)
			}
			delete(s.users, userId)
		}
	}()
	s.readLoop(ws)
}

func main() {
	if isLikelyLocal {
		initDB()
	}
	server := NewServer()
	http.Handle("/ws/", websocket.Handler(server.handleWs))
	http.ListenAndServe(":3000", nil)
}
