package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"

	"golang.org/x/net/websocket"
)

type Server struct {
	mu        sync.Mutex
	chatRooms map[string]map[string]struct{} // roomID -> (userID -> empty struct)
	users     map[string][]string            // userID -> list of roomIDs
	conns     map[string]*websocket.Conn     // WebSocket connection -> userID // probably reverse this so the key is userID and the value is the ws connection
}
// how does one user have multiple ws connected?

type Message struct {
	Content string `json:"content"`
	Type    string `json:"type"`
}

func NewServer() *Server {
	return &Server{
		chatRooms: make(map[string]map[string]struct{}),
		users:     make(map[string][]string),
		conns:     make(map[string]*websocket.Conn),
	}
}

// "*" makes it a pointer receiver
// makes it so we modify the original connections map, not a copy
func (s *Server) addConnection(ws *websocket.Conn, userId string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.conns[userId] = ws
}

func (s *Server) addUserToRoom(userId, roomId string) {
	// if we don't have the room that was created, add it
	if _, exists := s.chatRooms[roomId]; !exists {
		fmt.Println("room doesn't exist, create new: ", roomId)
		s.chatRooms[roomId] = make(map[string]struct{})
	}

	// if the user id is not in the users list, add it
	if _, exists := s.users[userId]; !exists {
		s.users[userId] = append(s.users[userId], roomId)
	}

	// we already have the room, just add the user to it
	s.chatRooms[roomId][userId] = struct{}{}
}

func (s *Server) joinRoom(roomId, userId string) {
	fmt.Println("room with users", s.chatRooms[roomId])
	if _, exists := s.chatRooms[roomId][userId]; exists {
		fmt.Printf("User: %s already exists in room: %s, no action needed.", userId, roomId)
	} else {
		message := Message{
			Content: fmt.Sprintf("%s joined room %s", userId, roomId),
			Type:    "join",
		}
		jsonBytes, err := json.Marshal(message)
		if err != nil {
			fmt.Println("Error marshaling struct:", err)
			return
		}
		s.broadcastToRoom(jsonBytes, roomId)
		s.addUserToRoom(userId, roomId)
	}
}

func (s *Server) leaveRoom(roomId, userId string) {
	if _, exists := s.chatRooms[roomId][userId]; exists {
		delete(s.chatRooms[roomId], userId)
		message := Message{
			Content: fmt.Sprintf("%s left room %s", userId, roomId),
			Type:    "leave",
		}
		jsonBytes, err := json.Marshal(message)
		if err != nil {
			fmt.Println("Error marshaling struct:", err)
			return
		}
		s.broadcastToRoom(jsonBytes, roomId)
	}
}

func (s *Server) sendMessage(content, roomId string) {
	message := Message{
		Content: content,
		Type:    "message",
	}
	jsonBytes, err := json.Marshal(message)
	if err != nil {
		fmt.Println("Error marshaling struct:", err)
		return
	}
	s.broadcastToRoom(jsonBytes, roomId)
}

func (s *Server) broadcastToRoom(b []byte, roomId string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if listOfUsers, exists := s.chatRooms[roomId]; exists {
		for user := range listOfUsers {
			for id, conn := range s.conns {
				if id == user {
					go func(ws *websocket.Conn) {
						if _, err := ws.Write(b); err != nil {
							fmt.Println("error", err)
						}
					}(conn)
				}
			}
		}
	}
}

type ContentType string
const JoinRoomType ContentType = "join"

func (s *Server) readLoop(ws *websocket.Conn, roomId string) {
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
		

		// var request struct {
		// 	Type    ContentType `json:"type"`
		// 	Content interface{} `json:"content"`
		// }
		// type JoinRoomContent struct {
		// 	RoomIds []string `json:"roomIds"`
		// }
		var request map[string]string
		if err := json.Unmarshal(msg, &request); err != nil {
			fmt.Println("Error unmarshalling message:", err)
			continue
		}
		// switch request.Type {
		switch request["type"] {
		case "join":
			s.joinRoom(request["roomId"], request["userId"])
		case "leave":
			s.leaveRoom(request["roomId"], request["userId"])
		case "message":
			s.sendMessage(request["content"], roomId)
		}
	}
}

func (s *Server) handleWs(ws *websocket.Conn) {
	query := ws.Request().URL.Query()
	userId := query.Get("userId")
	roomId := query.Get("roomId")
	// validation logic for not having userId/roomId
	if userId == "" || roomId == "" {
		// you can only attempt to push a message down (dunno if that will work), or shut down the connection
		// maybe do both
	}
	s.addUserToRoom(userId, roomId)
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
		// remove ws connection
		delete(s.conns, userId)
		// remove user from chat room
		for _, room := range s.users[userId] {
			delete(s.chatRooms[room], userId)
		}
		// remove user from s.users
		delete(s.users, userId)
		s.mu.Unlock()
	}()
	s.readLoop(ws, roomId)
}

func main() {
	server := NewServer()
	http.Handle("/ws/", websocket.Handler(server.handleWs))
	http.ListenAndServe(":3000", nil)
}
