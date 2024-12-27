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
	users     map[string]userInfo            // userID -> userInfo
}

type userInfo struct {
	rooms []string
	conn  *websocket.Conn
}

// how does one user have multiple ws connected?
type Message struct {
	Content string `json:"content"`
	Type    string `json:"type"`
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

func (s *Server) joinRoom(userId, roomId string, ws *websocket.Conn) {
	// why is this lock causing so many problems?
	// adding it here or in notifyRoomOfJoin makes it so that only the first user joins the room, and no subsequent room joins (on page load) work
	// s.mu.Lock()
	// defer s.mu.Unlock()
	// if we don't have the room that the user wants to join, add it
	if _, exists := s.chatRooms[roomId]; !exists {
		s.chatRooms[roomId] = make(map[string]struct{})
	}
	s.chatRooms[roomId][userId] = struct{}{}

	if user, exists := s.users[userId]; exists {
		user.rooms = append(user.rooms, roomId)
		// Ensure the connection is set if it's not already or if it's a new connection for this user
		if user.conn == nil {
			user.conn = ws
		}
		s.users[userId] = user
	} else {
		// If the user does not exist, create a new userInfo with this room
		s.users[userId] = userInfo{
			rooms: []string{roomId},
			conn:  ws,
		}
	}
	s.notifyRoomOfJoin(roomId, userId)
}

func (s *Server) notifyRoomOfJoin(roomId, userId string) {
	// why is this lock causing problems?
	// s.mu.Lock()
	// defer s.mu.Unlock()
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
	if roomUsers, exists := s.chatRooms[roomId]; exists {
		for userId := range roomUsers {
			if user, ok := s.users[userId]; ok && user.conn != nil {
				go func(ws *websocket.Conn) {
					if _, err := ws.Write(b); err != nil {
						fmt.Println("error", err)
					}
				}(user.conn)
			}
		}
	}
}

type ContentType string

const JoinRoomType ContentType = "join"
const LeaveRoomType ContentType = "leave"
const MessageType ContentType = "message"

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

		var request struct {
			Type    ContentType `json:"type"`
			Content interface{} `json:"content"`
		}
		// type JoinRoomContent struct {
		// 	RoomIds []string `json:"roomIds"`
		// }
		if err := json.Unmarshal(msg, &request); err != nil {
			fmt.Println("Error unmarshalling message: ", err)
		}
		content, ok := request.Content.(map[string]interface{})

		if ok {
			switch request.Type {
			case MessageType:
				text, textExists := content["text"]
				destination, destinationExists := content["destination"]
				textStr, textIsString := text.(string)
				destinationStr, destinationIsString := destination.(string)

				if !textExists || !destinationExists {
					fmt.Println("Payload missing required parameters for message type")
					return
				}
				if !textIsString || !destinationIsString {
					fmt.Println("text or destination is not a string")
					return
				}
				s.sendMessage(textStr, destinationStr)

			case LeaveRoomType:
				roomId, roomIdExists := content["roomId"]
				roomIdStr, roomIdIsString := roomId.(string)
				userId, userIdExists := content["userId"]
				userIdStr, userIdIsString := userId.(string)

				if !userIdExists || !roomIdExists {
					fmt.Println("Payload missing params")
					return
				}
				if !roomIdIsString || !userIdIsString {
					fmt.Println("Not a string")
					return
				}
				s.leaveRoom(roomIdStr, userIdStr)
			
			case JoinRoomType:
				roomId, roomIdExists := content["roomId"]
				roomIdStr, roomIdIsString := roomId.(string)
				userId, userIdExists := content["userId"]
				userIdStr, userIdIsString := userId.(string)
				if !roomIdIsString || !userIdIsString {
					fmt.Println("Not a string")
					return
				}
				if !userIdExists || !roomIdExists {
					fmt.Println("No user id in payload")
					return
				}
				s.joinRoom(userIdStr, roomIdStr, ws)
			}
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
	server := NewServer()
	http.Handle("/ws/", websocket.Handler(server.handleWs))
	http.ListenAndServe(":3000", nil)
}
