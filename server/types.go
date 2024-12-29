package server

import (
	"sync"

	"golang.org/x/net/websocket"
)

type Server struct {
	mu        sync.Mutex
	chatRooms map[string]map[string]struct{} // roomID -> (userID -> empty struct)
	users     map[string]UserInfo            // userID -> userInfo
}

type UserInfo struct {
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
