package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"golang.org/x/net/websocket"
)

// conns:
// ws1 -> userID1
// ws2 -> userID2
// chatRooms:
// roomId1 -> {userID1, userID3}
// roomId2 -> {userID2, userID1}
// users:
// userID1 -> [roomId1, roomId2]
// userID2 -> [roomId2]

type Server struct {
	chatRooms map[string]map[string]struct{} // roomID -> (userID -> empty struct)
	users     map[string][]string            // userID -> list of roomIDs
	conns     map[*websocket.Conn]string     // WebSocket connection -> userID
}

func NewServer() *Server {
	return &Server{
		chatRooms: make(map[string]map[string]struct{}),
		users:     make(map[string][]string),
		conns:     make(map[*websocket.Conn]string),
	}
}

func (s *Server) handleWs(ws *websocket.Conn, userId, roomId string) {
	s.addUserToRoom(userId, roomId)
	s.conns[ws] = userId
	s.readLoop(ws, roomId)
	fmt.Println("chatRooms", s.chatRooms)
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

	s.chatRooms[roomId][userId] = struct{}{}
	// we already have the room, just add the user to it
	fmt.Println(s.chatRooms)
}

func (s *Server) joinRoom(roomId, userId string) {
	fmt.Println("room with users", s.chatRooms[roomId])
	if _, exists := s.chatRooms[roomId][userId]; exists {
		fmt.Printf("User: %s already exists in room: %s, no action needed.", userId, roomId)
	} else {
		joinMessage := fmt.Sprintf("%s joined room %s", userId, roomId)
		s.broadcastToRooms([]byte(joinMessage), roomId)
		s.addUserToRoom(userId, roomId)
	}
}

func (s *Server) leaveRoom(roomId, userId string) {
	if _, exists := s.chatRooms[roomId][userId]; exists {
		leaveMessage := fmt.Sprintf("%s left room %s", userId, roomId)
		s.broadcastToRooms([]byte(leaveMessage), roomId)
		delete(s.chatRooms[roomId], userId)
	}
}

func (s *Server) readLoop(ws *websocket.Conn, roomId string) {
	buf := make([]byte, 1024)
	for {
		n, err := ws.Read(buf)
		if err != nil {
			if err == io.EOF {
				break
			}
			fmt.Println("read error: ", err)
			continue
		}

		msg := buf[:n]
		var request map[string]string
		if err := json.Unmarshal(msg, &request); err != nil {
			fmt.Println("Error unmarshalling message:", err)
			s.broadcastToRooms(msg, roomId)
			continue
		}
		switch request["action"] {
		case "join":
			s.joinRoom(request["roomId"], request["userId"])
		case "leave":
			s.leaveRoom(request["roomId"], request["userId"])
		default:
			s.broadcastToRooms(msg, roomId)
		}

	}
}

func (s *Server) broadcastToRooms(b []byte, roomId string) {
	if listOfUsers, exists := s.chatRooms[roomId]; exists {
		// fmt.Println("found room", roomId, "has users", listOfUsers)
		for user := range listOfUsers {
			for conn, id := range s.conns {
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

func main() {
	server := NewServer()
	http.Handle("/ws/", websocket.Handler(func(ws *websocket.Conn) {
		query := ws.Request().URL.Query()
		userId := query.Get("userId")
		roomId := query.Get("roomId")
		server.handleWs(ws, userId, roomId)
	}))
	http.ListenAndServe(":3000", nil)
}
