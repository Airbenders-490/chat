package http

import (
	"chat/domain"
	"chat/utils/errors"
	"chat/utils/httputils"
	"context"
	"encoding/json"
	"fmt"
	"github.com/dgrijalva/jwt-go"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type connection struct {
	ws   *websocket.Conn
	send chan Event
}

type subscription struct {
	conn   *connection
	roomID string
	userID string
}

type Event struct {
	MessageType MessageType `json:"message_type"`
	Message     domain.Message
}

type MessageType int

const (
	Send MessageType = iota
	Edit
	Delete
)

const missingIdError = "Must provide room id"
const invalidRequestBody = "invalid request body"

func NewSendEvent(message domain.Message) Event {
	return Event{
		MessageType: Send,
		Message:     message,
	}
}

func NewEditEvent(message domain.Message) Event {
	return Event{
		MessageType: Edit,
		Message:     message,
	}
}

func NewDeleteEvent(message domain.Message) Event {
	return Event{
		MessageType: Delete,
		Message:     message,
	}
}

// hub is the heart of the chat app. This is what is used to hold "rooms", register and unregister when connecting and
// disconnecting, and broadcast. Whenever a message is sent to broadcast channel, it is delivered to all the connections
// in room
type hub struct {
	rooms      map[string]map[subscription]bool
	broadcast  chan Event
	Register   chan subscription
	unregister chan subscription
}

var (
	singleton hub
	once      sync.Once
	mainHub   = NewHub()
)

func NewHub() hub {
	once.Do(func() {
		singleton = hub{
			broadcast:  make(chan Event),
			Register:   make(chan subscription),
			unregister: make(chan subscription),
			rooms:      make(map[string]map[subscription]bool),
		}
	})

	return singleton
}

const (
	maxMessageSize int64 = 1024
	pongWait             = time.Minute * 5
	pingPeriod           = time.Minute * 4
	writeWait            = time.Minute
)

func (s *subscription) readPump(u domain.MessageUseCase) {
	c := s.conn
	defer func() {
		mainHub.unregister <- *s
		c.ws.Close()
	}()
	c.ws.SetReadLimit(maxMessageSize)
	_ = c.ws.SetReadDeadline(time.Now().Add(pongWait))
	c.ws.SetPongHandler(func(string) error { _ = c.ws.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	for {
		_, msg, err := c.ws.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway) {
				log.Printf("error: %v", err)
			}
			break
		}
		m := domain.Message{RoomID: s.roomID, SentTimestamp: time.Now().UTC(), FromStudentID: s.userID, MessageBody: string(msg)}
		err = u.SaveMessage(context.Background(), &m)
		if err != nil {
			log.Printf("Failed to save message with err %s", err.Error())
		}
		mainHub.broadcast <- NewSendEvent(m)
	}
}

func (s *subscription) writePump() {
	c := s.conn
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.ws.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			if !ok {
				c.write(websocket.CloseMessage, []byte{})
				return
			}
			res, err := json.Marshal(message)
			if err != nil {
				log.Printf("message %s couldn't be sent to %s in room %s.", message.Message, s.userID, s.roomID)
			}
			if err = c.write(websocket.TextMessage, res); err != nil {
				log.Printf("message %s couldn't be sent to %s in room %s.", message.Message, s.userID, s.roomID)
				return
			}
		case <-ticker.C:
			if err := c.write(websocket.PingMessage, []byte{}); err != nil {
				return
			}
		}
	}
}

func (c *connection) write(mt int, payload []byte) error {
	c.ws.SetWriteDeadline(time.Now().Add(writeWait))
	return c.ws.WriteMessage(mt, payload)
}

// ServeWs is the handleFunc for connecting to a room's websocket. A user must be authorized, i.e. already added to
// the room before he can connect to the room. Otherwise, returns 401.
func (h *MessageHandler) ServeWs(w http.ResponseWriter, r *http.Request, roomID string, ctx context.Context) {

	tokenParam, ok := r.URL.Query()["token"]

	if !ok || len(tokenParam[0]) < 1 {
		log.Println("Url Param 'token' is missing")
		return
	}
	tokenString := r.URL.Query()["token"][0]
	token, err := jwt.ParseWithClaims(tokenString, &jwt.StandardClaims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(os.Getenv("SECRET_KEY")), nil
	})
	if err != nil {
		log.Println("NOT logged in")
		return
	}
	standardClaims := token.Claims.(*jwt.StandardClaims)

	authorized := h.u.IsAuthorized(ctx, standardClaims.Issuer, roomID)
	if !authorized {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, "Not authorized to enter the room number "+roomID)
		return
	}
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err.Error())
		return
	}
	c := &connection{send: make(chan Event), ws: ws}
	s := subscription{c, roomID, standardClaims.Issuer}
	if authorized {
		mainHub.Register <- s
	}
	go s.writePump()
	go s.readPump(h.u)
}

// StartHubListener starts the functioning of the hub with respect to registering, unregistering and broadcasting
func (h *hub) StartHubListener() {
	for {
		select {
		case s := <-h.Register:
			h.RegisterCase(s)
		case s := <-h.unregister:
			h.UnregisterCase(s)
		case m := <-h.broadcast:
			subscriptions := h.rooms[m.Message.RoomID]
			for s := range subscriptions {
				if m.Message.FromStudentID == s.userID {
					continue
				}
				select {
				case s.conn.send <- m:
				default:
					close(s.conn.send)
					delete(subscriptions, s)
					if len(subscriptions) == 0 {
						delete(h.rooms, m.Message.RoomID)
					}
				}
			}

		}
	}
}

func (h *hub) RegisterCase(s subscription) {
	connections := h.rooms[s.roomID]
	if connections == nil {
		connections = make(map[subscription]bool)
		h.rooms[s.roomID] = connections
	}
	h.rooms[s.roomID][s] = true
}

func (h *hub) UnregisterCase(s subscription) {
	connections := h.rooms[s.roomID]
	if connections != nil {
		if _, ok := connections[s]; ok {
			delete(connections, s)
			close(s.conn.send)
			if len(connections) == 0 {
				delete(h.rooms, s.roomID)
			}
		}
	}
}

// MessageHandler is the standard delivery handler for messaging service
type MessageHandler struct {
	u domain.MessageUseCase
}

// NewMessageHandler instantiates and returns a new MessageHandler
func NewMessageHandler(u domain.MessageUseCase) *MessageHandler {
	return &MessageHandler{u: u}
}

func (h *MessageHandler) LoadMessages(c *gin.Context) {
	room := c.Param("roomID")
	queryLimit := c.Query("limit")
	var limit int

	i, err := strconv.ParseInt(queryLimit, 10, 64)
	if err != nil || i < 1 {
		limit = 10
	} else {
		limit = int(i)
	}

	var message domain.Message
	err = c.ShouldBindJSON(&message)
	if err != nil {
		c.JSON(http.StatusBadRequest, errors.NewBadRequestError(invalidRequestBody))
		return
	}

	ctx := c.Request.Context()

	msgs, err := h.u.GetMessages(ctx, room, message.SentTimestamp, limit)

	if err != nil {
		errors.SetRESTError(err, c)
		return
	}

	c.JSON(http.StatusOK, msgs)
}

func (h *MessageHandler) EditMessage(c *gin.Context) {
	key, _ := c.Get("loggedID")
	loggedID, _ := key.(string)

	ctx := c.Request.Context()

	var message domain.Message
	err := c.ShouldBindJSON(&message)
	if err != nil || message.FromStudentID == "" || message.RoomID == "" {
		c.JSON(http.StatusBadRequest, errors.NewBadRequestError(invalidRequestBody))
		return
	}

	if loggedID != message.FromStudentID {
		c.JSON(http.StatusUnauthorized, errors.NewUnauthorizedError("Can only edit your own messages"))
		return
	}

	editedMessage, err := h.u.EditMessage(ctx, message.RoomID, message.FromStudentID, message.SentTimestamp, message.MessageBody)

	if err != nil {
		errors.SetRESTError(err, c)
		return
	}

	mainHub.broadcast <- NewEditEvent(message)
	c.JSON(http.StatusOK, editedMessage)
}

func (h *MessageHandler) DeleteMessage(c *gin.Context) {
	roomID := c.Param("roomID")
	timestamp := c.Param("timestamp")
	sentTimestamp, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		fmt.Println(err)
		errors.SetRESTError(err, c)
		return
	}
	key, _ := c.Get("loggedID")
	loggedID, _ := key.(string)
	ctx := c.Request.Context()

	message, err := h.u.DeleteMessage(ctx, roomID, sentTimestamp, loggedID)
	if err != nil {
		errors.SetRESTError(err, c)
		return
	}

	mainHub.broadcast <- NewDeleteEvent(*message)
	c.JSON(http.StatusAccepted, httputils.NewResponse("message deleted"))
}

func (h *MessageHandler) JoinRequest(c *gin.Context) {
	room := c.Param("roomID")

	key, _ := c.Get("loggedID")
	loggedID, _ := key.(string)

	ctx := c.Request.Context()

	err := h.u.JoinRequest(ctx, room, loggedID, time.Now().UTC())
	if err != nil {
		errors.SetRESTError(err, c)
		return
	}

	c.JSON(http.StatusOK, httputils.NewResponse("Join Request Sent"))
}

func (h *MessageHandler) RejectJoinRequest(c *gin.Context) {
	roomID := c.Param("roomID")
	userID := c.Param("userID")

	key, _ := c.Get("loggedID")
	loggedID, _ := key.(string)

	ctx := c.Request.Context()

	err := h.u.SendRejection(ctx, roomID, userID, loggedID)

	if err != nil {
		errors.SetRESTError(err, c)
		return
	}

	c.JSON(http.StatusOK, httputils.NewResponse("Decline Join Request Sent"))
}
