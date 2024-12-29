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
