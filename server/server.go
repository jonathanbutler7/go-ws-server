package server

import (
	"encoding/json"
	"fmt"
	"io"

	"golang.org/x/net/websocket"
)

func NewServer() *Server {
	return &Server{
		chatRooms: make(map[string]map[string]struct{}),
		users:     make(map[string]UserInfo),
	}
}

func (s *Server) handleLeave(content json.RawMessage) {
	var leaveContent LeaveRoomContent
	if err := json.Unmarshal(content, &leaveContent); err != nil {
		fmt.Println("Error unmarshalling leave message: ", err)
	}
	success := s.leaveRoom(leaveContent.RoomId, leaveContent.UserId)
	if success {
		content := fmt.Sprintf("%s left room %s", leaveContent.UserId, leaveContent.RoomId)
		s.broadcastMessageToRoom(Message{
			Content: content,
			Type:    "leave",
		}, leaveContent.RoomId)
	}
}

func (s *Server) handleMessage(content json.RawMessage) {
	var message MessageContent
	if err := json.Unmarshal(content, &message); err != nil {
		fmt.Println("Error unmarshalling message: ", err)
	}

	s.broadcastMessageToRoom(Message{
		Content: message.Text,
		Type:    "message",
	}, message.Destination)
}

func (s *Server) handleJoin(content json.RawMessage, ws *websocket.Conn) {
	var joinContent JoinRoomContent
	if err := json.Unmarshal(content, &joinContent); err != nil {
		fmt.Println("Error unmarshalling join message: ", err)
	}
	if success := s.joinRoom(joinContent.UserId, joinContent.RoomId, ws); success {
		s.broadcastMessageToRoom(Message{
			Content: fmt.Sprintf("%s joined room %s", joinContent.UserId, joinContent.RoomId),
			Type:    "join",
		}, joinContent.RoomId)
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
		s.users[userId] = UserInfo{
			conn:  ws,
			rooms: []string{}, // Initialize with empty rooms slice
		}
	}
}

func (s *Server) joinRoom(userId, roomId string, ws *websocket.Conn) bool {
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
		s.users[userId] = UserInfo{
			rooms: []string{roomId},
			conn:  ws,
		}
		s.mu.Unlock()
	}
	return true
}

func (s *Server) leaveRoom(roomId, userId string) bool {
	if _, exists := s.chatRooms[roomId][userId]; exists {
		delete(s.chatRooms[roomId], userId)
		return true
	}
	return false
}

func (s *Server) broadcastMessageToRoom(message Message, roomId string) {
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
			// Capture the necessary variables explicitly
			conn := user.conn // Copy the connection to avoid race conditions
			go func(userId, roomId string, ws *websocket.Conn) {
				s.LogAction(userId, message.Type, roomId, message.Content)
				b, err := json.Marshal(message)
				if err != nil {
					fmt.Println("Error marshalling struct: ", err)
					return
				}
				if _, err := ws.Write(b); err != nil {
					fmt.Println("error writing to socket", err)
				}
			}(userId, roomId, conn)
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
		msgBytes := buf[:n]

		var baseMsg struct {
			Type    ContentType     `json:"type"`
			Content json.RawMessage `json:"content"`
		}
		if err := json.Unmarshal(msgBytes, &baseMsg); err != nil {
			fmt.Println("Error unmarshalling base message: ", err)
		}

		switch baseMsg.Type {
		case MessageType:
			s.handleMessage(baseMsg.Content)
		case LeaveRoomType:
			s.handleLeave(baseMsg.Content)
		case JoinRoomType:
			s.handleJoin(baseMsg.Content, ws)
		}
	}
}

func (s *Server) HandleWs(ws *websocket.Conn) {
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
