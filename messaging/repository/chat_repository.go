package repository

import (
	"chat/domain"
	"context"
	"github.com/gocql/gocql"
	"time"
)

const (
	insertMessage = `INSERT INTO messages (room_id, from_student_id, message_body, sent_timestamp) VALUES (?, ?, ?, ?)`
	editMessage = `UPDATE messages SET message_body=? WHERE room_id=? AND sent_timestamp=? IF EXISTS;`
	getMessage = `SELECT * FROM messages where room_id=? AND sent_timestamp =?`
	getMessages = `SELECT * FROM messages WHERE room_id=? AND sent_timestamp <=? limit ?`
	deleteMessage = `DELETE FROM messages WHERE room_id=? AND sent_timestamp=? IF EXISTS`
)

type messageRepository struct {
	dbSession *gocql.Session
}

// NewChatRepository is the constructor
func NewChatRepository(session *gocql.Session) domain.MessageRepository {
	return &messageRepository{
		dbSession: session,
	}
}

func (m *messageRepository) SaveMessage(ctx context.Context, message *domain.Message) error {
	err := m.dbSession.Query(insertMessage, message.RoomID, message.FromStudentID, message.MessageBody, message.SentTimestamp).WithContext(ctx).Exec()
	if err != nil {
		return err
	}
	return nil
}

func (m *messageRepository) EditMessage(ctx context.Context, message *domain.Message) error {
	var editedMsg domain.Message
	_, err := m.dbSession.Query(editMessage, message.MessageBody, message.RoomID, message.SentTimestamp).WithContext(ctx).
		ScanCAS(&editedMsg.RoomID, &editedMsg.SentTimestamp, &editedMsg.FromStudentID, &editedMsg.MessageBody)

	if err != nil{
		return err
	}
	return nil
}

func (m *messageRepository) GetMessage(ctx context.Context, roomID string, timeStamp time.Time) (*domain.Message, error) {
	var retrievedMsg domain.Message

	err := m.dbSession.Query(getMessage, roomID, timeStamp).WithContext(ctx).
		Scan(&retrievedMsg.RoomID, &retrievedMsg.SentTimestamp, &retrievedMsg.FromStudentID, &retrievedMsg.MessageBody)
	if err != nil{
		return nil, err
	}

	return &retrievedMsg, err
}

func (m *messageRepository) GetMessages(ctx context.Context, roomID string, timeStamp time.Time, limit int) ([]domain.Message, error) {
	var retrievedMessages []domain.Message
	var scanner gocql.Scanner

	scanner = m.dbSession.Query(getMessages, roomID, timeStamp, limit).WithContext(ctx).Iter().Scanner()

	for scanner.Next(){
		var msg domain.Message
		err := scanner.Scan(&msg.RoomID, &msg.SentTimestamp, &msg.FromStudentID, &msg.MessageBody)

		if err == nil {
			retrievedMessages = append(retrievedMessages, msg)
		}

	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return retrievedMessages, nil

}

func (m *messageRepository) DeleteMessage(ctx context.Context, roomID string, timeStamp time.Time) error {
	err := m.dbSession.Query(deleteMessage, roomID, timeStamp).WithContext(ctx).Exec()

	if err != nil {
		return err
	}

	return nil
}
